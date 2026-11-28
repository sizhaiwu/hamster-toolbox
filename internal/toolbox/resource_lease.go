package toolbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type resourceClaim struct {
	ID, Tool, Metadata string
	Read, Write        []string
}

func leasePath(p string) string {
	if p == "" {
		return ""
	}
	a, e := CanonicalResourcePath(p)
	if e != nil {
		a, _ = filepath.Abs(p)
	}
	if runtime.GOOS == "windows" {
		a = strings.ToLower(a)
	}
	return filepath.Clean(a)
}
func leaseOverlap(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	for _, pair := range [][2]string{{a, b}, {b, a}} {
		r, e := filepath.Rel(pair[0], pair[1])
		if e == nil && (r == "." || r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator))) {
			return true
		}
	}
	return false
}
func leaseKinds(c resourceClaim, write bool) int {
	switch c.Tool {
	case "video":
		return 2
	case "image":
		return 1
	case "thumbnail":
		if write {
			return 1
		}
		return 2
	case "exif":
		if c.Metadata == "images" {
			return 1
		}
		if c.Metadata == "videos" {
			return 2
		}
		return 3
	case "pdf", "archive":
		if write {
			return 4
		}
	}
	return 7
}
func leaseReadConflict(reader, writer resourceClaim, r, w string) bool {
	if !leaseOverlap(r, w) || leaseKinds(reader, false)&leaseKinds(writer, true) == 0 {
		return false
	}
	switch reader.Tool {
	case "image", "video", "exif", "thumbnail":
		rel, e := filepath.Rel(r, w)
		if e == nil {
			first := strings.Split(rel, string(filepath.Separator))[0]
			if first == "toolbox-output" || first == "0utput_7z" || first == "视频预览图" || first == "解压资源" {
				return false
			}
		}
	}
	return true
}
func claimConflict(a, b resourceClaim) bool {
	for _, w := range a.Write {
		for _, v := range b.Write {
			if leaseOverlap(w, v) {
				return true
			}
		}
		for _, r := range b.Read {
			if leaseReadConflict(b, a, r, w) {
				return true
			}
		}
	}
	for _, r := range a.Read {
		for _, w := range b.Write {
			if leaseReadConflict(a, b, r, w) {
				return true
			}
		}
	}
	return false
}
func claimForConfig(c Config) resourceClaim {
	claim := resourceClaim{Tool: c.Tool, Metadata: c.MetadataTypes, Read: []string{c.Root}}
	switch c.Tool {
	case "format", "rename", "number", "describe":
		claim.Write = []string{c.Root}
	default:
		out := c.Output
		if out == "" {
			out = filepath.Join(c.Root, "toolbox-output", c.Tool)
			switch c.Tool {
			case "thumbnail":
				out = filepath.Join(c.Root, "视频预览图")
			case "archive":
				out = filepath.Join(c.Root, "0utput_7z")
			case "extract":
				out = filepath.Join(c.Root, "解压资源")
			case "pdf":
				out = filepath.Join(c.Root, "toolbox-output", "目录索引.pdf")
			}
		} else if !filepath.IsAbs(out) {
			out = filepath.Join(c.Root, out)
		}
		claim.Write = []string{out}
		if c.Tool == "extract" && c.ExtractDeleteSource {
			claim.Write = append(claim.Write, c.Root)
		}
	}
	return claim
}
func AcquireResourcePaths(ctx context.Context, read, write []string, log func(string)) (func(), error) {
	return acquireResourceClaim(ctx, resourceClaim{Tool: "script", Read: read, Write: write}, log)
}

type resourceTokenKey struct{}
type resourceParentKey struct{}
type resourceCacheKey struct{}

func AcquireResourcePathsWithToken(ctx context.Context, read, write []string, log func(string)) (func(), string, error) {
	token := ""
	release, e := acquireResourceClaim(context.WithValue(ctx, resourceTokenKey{}, &token), resourceClaim{Tool: "script", Read: read, Write: write}, log)
	return release, token, e
}

var resourceCacheDir = os.UserCacheDir

func acquireResourceClaim(ctx context.Context, incoming resourceClaim, log func(string)) (func(), error) {
	incoming.Read = append([]string(nil), incoming.Read...)
	incoming.Write = append([]string(nil), incoming.Write...)
	for i, p := range incoming.Read {
		incoming.Read[i] = leasePath(p)
	}
	for i, p := range incoming.Write {
		incoming.Write[i] = leasePath(p)
	}
	cache, _ := ctx.Value(resourceCacheKey{}).(string)
	if cache == "" {
		var e error
		cache, e = resourceCacheDir()
		if e != nil {
			return nil, e
		}
	}
	base := filepath.Join(cache, "hamster-toolbox", "resource-leases-v1")
	if e := os.MkdirAll(base, 0700); e != nil {
		return nil, e
	}
	announced := false
	gate := func(ctx context.Context) (*os.File, error) {
		for {
			f, e := os.OpenFile(filepath.Join(base, "gate.lock"), os.O_CREATE|os.O_RDWR, 0600)
			if e != nil {
				return nil, e
			}
			if e = lockResourceFile(f); e == nil {
				return f, nil
			}
			f.Close()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
	readClaims := func() ([]resourceClaim, error) {
		b, e := os.ReadFile(filepath.Join(base, "claims.json"))
		if errors.Is(e, os.ErrNotExist) {
			return []resourceClaim{}, nil
		}
		if e != nil {
			return nil, e
		}
		var claims []resourceClaim
		if json.Unmarshal(b, &claims) != nil || len(claims) > 1000 {
			return nil, errors.New("资源互斥记录无效，请关闭工具箱后检查缓存目录")
		}
		return claims, nil
	}
	writeClaims := func(claims []resourceClaim) error {
		b, _ := json.Marshal(claims)
		f, e := os.CreateTemp(base, ".claims-*")
		if e != nil {
			return e
		}
		defer os.Remove(f.Name())
		_, e = f.Write(b)
		ce := f.Close()
		if e == nil {
			e = ce
		}
		if e != nil {
			return e
		}
		return os.Rename(f.Name(), filepath.Join(base, "claims.json"))
	}
	validID := func(id string) bool {
		return filepath.Base(id) == id && strings.HasPrefix(id, "active-") && strings.HasSuffix(id, ".lease")
	}
	for {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		g, e := gate(ctx)
		if e != nil {
			return nil, e
		}
		claims, e := readClaims()
		if e != nil {
			g.Close()
			return nil, e
		}
		active := []resourceClaim{}
		conflict := false
		for _, c := range claims {
			if !validID(c.ID) {
				g.Close()
				return nil, errors.New("资源互斥记录 ID 无效")
			}
			probe, err := os.OpenFile(filepath.Join(base, c.ID), os.O_RDWR, 0600)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				g.Close()
				return nil, err
			}
			err = lockResourceFile(probe)
			probe.Close()
			if err == nil {
				_ = os.Remove(filepath.Join(base, c.ID))
				continue
			}
			active = append(active, c)
			parent, _ := ctx.Value(resourceParentKey{}).(string)
			if parent == "" {
				parent = os.Getenv("HAMSTER_RESOURCE_PARENT")
			}
			if c.ID != parent && claimConflict(incoming, c) {
				conflict = true
			}
		}
		if !conflict {
			live, e := os.CreateTemp(base, "active-*.lease")
			if e == nil {
				e = lockResourceFile(live)
			}
			if e != nil {
				if live != nil {
					live.Close()
					os.Remove(live.Name())
				}
				g.Close()
				return nil, e
			}
			incoming.ID = filepath.Base(live.Name())
			active = append(active, incoming)
			e = writeClaims(active)
			g.Close()
			if e != nil {
				live.Close()
				os.Remove(live.Name())
				return nil, e
			}
			if token, ok := ctx.Value(resourceTokenKey{}).(*string); ok {
				*token = incoming.ID
			}
			return func() {
				defer func() { live.Close(); os.Remove(live.Name()) }()
				deadline, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				g, e := gate(deadline)
				if e != nil {
					return
				}
				defer g.Close()
				claims, e := readClaims()
				if e != nil {
					return
				}
				keep := []resourceClaim{}
				for _, c := range claims {
					if c.ID != incoming.ID {
						keep = append(keep, c)
					}
				}
				_ = writeClaims(keep)
			}, nil
		}
		g.Close()
		if !announced && log != nil {
			log("正在等待其他任务释放同一资源目录…")
			announced = true
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("等待目录互斥已取消：%w", ctx.Err())
		case <-time.After(250 * time.Millisecond):
		}
	}
}
