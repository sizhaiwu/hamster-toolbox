package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

const releasePageURL = "https://github.com/sizhaiwu/hamster-toolbox/releases"
const latestReleaseAPI = "https://api.github.com/repos/sizhaiwu/hamster-toolbox/releases/latest"

type versionUpdate struct {
	Current, Latest, State, Message, URL string
	Available, Development, Cached       bool
	CheckedAt                            time.Time
}
type releaseSnapshot struct {
	tag, state       string
	checked, expires time.Time
}
type releaseChecker struct {
	mu       sync.Mutex
	client   *http.Client
	endpoint string
	cached   releaseSnapshot
	flight   chan struct{}
}

var publicReleaseChecker = &releaseChecker{
	client:   &http.Client{Timeout: 6 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	endpoint: latestReleaseAPI,
}

// Three numeric components are compared numerically without overflowing ints.
// Only stable releases are offered; a prerelease sorts below its final version.
var productVersionPattern = regexp.MustCompile(`^[vV]?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z]+(?:[.-][0-9A-Za-z]+)*))?(?:\+[0-9A-Za-z]+(?:[.-][0-9A-Za-z]+)*)?$`)

func compareReleaseVersion(current, latest string) (int, bool) {
	a, b := productVersionPattern.FindStringSubmatch(current), productVersionPattern.FindStringSubmatch(latest)
	if a == nil || b == nil || b[4] != "" {
		return 0, false
	}
	for i := 1; i <= 3; i++ {
		if len(a[i]) < len(b[i]) {
			return -1, true
		}
		if len(a[i]) > len(b[i]) {
			return 1, true
		}
		if a[i] < b[i] {
			return -1, true
		}
		if a[i] > b[i] {
			return 1, true
		}
	}
	if a[4] != "" {
		return -1, true
	}
	return 0, true
}
func describeVersionUpdate(current string, raw releaseSnapshot, cached bool) versionUpdate {
	v := versionUpdate{Current: current, State: raw.state, URL: releasePageURL, CheckedAt: raw.checked, Cached: cached}
	m := productVersionPattern.FindStringSubmatch(current)
	v.Development = m == nil || m[4] != ""
	if raw.state == "unpublished" {
		v.Message = "仓库尚未发布正式版本。"
		return v
	}
	if raw.state != "ok" {
		v.Message = "暂时无法检查更新，请稍后重试，或打开 GitHub 发布页。"
		return v
	}
	v.Latest = raw.tag
	cmp, known := compareReleaseVersion(current, raw.tag)
	switch {
	case !known:
		v.State = "development"
		v.Message = "当前为未标注版本的开发构建，无法自动比较；可查看最新正式版。"
	case cmp < 0:
		v.State = "available"
		v.Available = true
		v.Message = "发现新的正式版本，请打开发布页下载并手动更新。"
	case v.Development:
		v.State = "development"
		v.Message = "你正在使用开发版；公开正式版较旧，无需降级。"
	case cmp > 0:
		v.State = "ahead"
		v.Message = "当前版本高于公开正式版，无需降级。"
	default:
		v.State = "current"
		v.Message = "已是最新正式版。"
	}
	return v
}
func (c *releaseChecker) fetch() releaseSnapshot {
	now := time.Now().UTC()
	result := releaseSnapshot{state: "unavailable", checked: now, expires: now.Add(time.Minute)}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint, nil)
	if err != nil {
		return result
	}
	r.Header.Set("Accept", "application/vnd.github+json")
	r.Header.Set("User-Agent", "HamsterToolbox-UpdateCheck")
	response, err := c.client.Do(r)
	if err != nil {
		return result
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		result.state = "unpublished"
		result.expires = now.Add(time.Hour)
		return result
	}
	if response.StatusCode != http.StatusOK {
		return result
	}
	var release struct {
		Tag               string `json:"tag_name"`
		Draft, Prerelease bool
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	if decoder.Decode(&release) != nil {
		return result
	}
	if err = decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return result
	}
	m := productVersionPattern.FindStringSubmatch(release.Tag)
	if release.Draft || release.Prerelease || m == nil || m[4] != "" || len(release.Tag) > 80 {
		return result
	}
	result.tag = release.Tag
	result.state = "ok"
	result.expires = now.Add(time.Hour)
	return result
}
func (c *releaseChecker) check(ctx context.Context, current string, force bool) (versionUpdate, error) {
	for {
		c.mu.Lock()
		if c.flight != nil {
			flight := c.flight
			c.mu.Unlock()
			select {
			case <-flight:
				force = false
				continue
			case <-ctx.Done():
				return versionUpdate{}, ctx.Err()
			}
		}
		now := time.Now()
		if !c.cached.checked.IsZero() && now.Before(c.cached.expires) && (!force || now.Sub(c.cached.checked) < 30*time.Second) {
			raw := c.cached
			c.mu.Unlock()
			return describeVersionUpdate(current, raw, true), nil
		}
		flight := make(chan struct{})
		c.flight = flight
		c.mu.Unlock()
		// One bounded request per process; page cancellation doesn't invalidate
		// other windows sharing the same result. Nothing from local files is sent.
		go func() {
			raw := c.fetch()
			c.mu.Lock()
			c.cached = raw
			c.flight = nil
			close(flight)
			c.mu.Unlock()
		}()
		select {
		case <-flight:
			force = false
			continue
		case <-ctx.Done():
			return versionUpdate{}, ctx.Err()
		}
	}
}
func (s *server) versionUpdateAPI(w http.ResponseWriter, r *http.Request) {
	var input struct{ Force bool }
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(&input) != nil {
		fail(w, errors.New("版本检查请求无效"))
		return
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		fail(w, errors.New("版本检查请求无效"))
		return
	}
	checker := s.updates
	if checker == nil {
		checker = publicReleaseChecker
	}
	state, err := checker.check(r.Context(), strings.TrimSpace(appVersion), input.Force)
	if err != nil {
		fail(w, err)
		return
	}
	send(w, state)
}
