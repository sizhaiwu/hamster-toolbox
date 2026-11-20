package toolbox

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image/jpeg"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

type thumbnailBuffer struct {
	bytes.Buffer
	limit int
}

const browserMediaFormats = "mov,matroska,webm,avi,mpegts,mpeg,mpegvideo,flv,asf,ogg,mxf,rm,nut,mp3,aac,wav,flac"

func (b *thumbnailBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if b.Len() < b.limit {
		b.Buffer.Write(p[:min(len(p), b.limit-b.Len())])
	}
	return n, nil
}

// Range requests serve the already-open file, not a path FFmpeg can race or
// interpret as a URL. Each request gets its own offset; the token and listener
// exist only during this thumbnail. No full video copy or external server.
func browserVideoFrame(ctx context.Context, program string, input *os.File) ([]byte, error) {
	data, err := browserVideoFrameFD(ctx, program, input)
	if !browserFDUnavailable(err) {
		return data, err
	}
	return browserVideoFrameHTTP(ctx, program, input)
}

func browserVideoFrameFD(ctx context.Context, program string, input *os.File) ([]byte, error) {
	var last error
	for _, seek := range []string{"1", "0"} {
		if _, err := input.Seek(0, 0); err != nil {
			return nil, err
		}
		args := append(browserSafeInputArgs(), "-ss", seek, "-i", "fd:", "-map", "0:v:0", "-an", "-sn", "-dn", "-frames:v", "1", "-vf", "scale=640:480:force_original_aspect_ratio=decrease", "-threads", "1", "-f", "image2pipe", "-vcodec", "mjpeg", "pipe:1")
		cmd := exec.CommandContext(ctx, program, args...)
		hideWindow(cmd)
		cmd.Stdin = input
		out, stderr := &thumbnailBuffer{limit: 2 << 20}, &thumbnailBuffer{limit: 4096}
		cmd.Stdout, cmd.Stderr = out, stderr
		err := cmd.Run()
		if err == nil {
			if cfg, e := jpeg.DecodeConfig(bytes.NewReader(out.Bytes())); e == nil && cfg.Width <= 640 && cfg.Height <= 480 {
				return out.Bytes(), nil
			}
		}
		last = fmt.Errorf("无法生成视频缩略图：%v %s", err, stderr.String())
		if browserFDUnavailable(last) || ctx.Err() != nil {
			return nil, last
		}
	}
	return nil, last
}

func browserFDUnavailable(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "protocol not found") || strings.Contains(message, "unknown protocol")
}

func browserSafeInputArgs() []string {
	return []string{"-v", "error", "-nostdin", "-max_alloc", "268435456", "-threads", "2", "-protocol_whitelist", "fd,pipe", "-format_whitelist", browserMediaFormats}
}

func browserVideoFrameHTTP(ctx context.Context, program string, input *os.File) ([]byte, error) {
	st, e := input.Stat()
	if e != nil || !st.Mode().IsRegular() {
		return nil, errors.New("视频文件不可读")
	}
	listener, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		return nil, e
	}
	token := make([]byte, 24)
	if _, e = rand.Read(token); e != nil {
		listener.Close()
		return nil, e
	}
	path := "/" + hex.EncodeToString(token)
	srv := &http.Server{ReadHeaderTimeout: 3 * time.Second, IdleTimeout: 3 * time.Second, ErrorLog: log.New(io.Discard, "", 0), Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path || (r.Method != "GET" && r.Method != "HEAD") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeContent(w, r, st.Name(), st.ModTime(), io.NewSectionReader(input, 0, st.Size()))
	})}
	go srv.Serve(listener)
	defer srv.Close()
	address := "http://" + listener.Addr().String() + path
	var last error
	for _, seek := range []string{"1", "0"} {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		args := []string{"-v", "error", "-nostdin", "-threads", "1", "-protocol_whitelist", "http,tcp,pipe", "-format_whitelist", "mov,matroska,webm,avi,mpegts,mpeg,mpegvideo,flv,asf,ogg,mxf,rm,nut", "-http_proxy", "", "-ss", seek, "-i", address, "-map", "0:v:0", "-an", "-sn", "-dn", "-frames:v", "1", "-vf", "scale=640:480:force_original_aspect_ratio=decrease", "-threads", "1", "-f", "image2pipe", "-vcodec", "mjpeg", "pipe:1"}
		cmd := exec.CommandContext(ctx, program, args...)
		hideWindow(cmd)
		out, stderr := &thumbnailBuffer{limit: 2 << 20}, &thumbnailBuffer{limit: 4096}
		cmd.Stdout = out
		cmd.Stderr = stderr
		e := cmd.Run()
		if e == nil && out.Len() > 0 {
			if _, e = jpeg.DecodeConfig(bytes.NewReader(out.Bytes())); e == nil {
				return out.Bytes(), nil
			}
		}
		last = fmt.Errorf("无法生成视频缩略图：%v %s", e, strings.TrimSpace(stderr.String()))
	}
	return nil, last
}
