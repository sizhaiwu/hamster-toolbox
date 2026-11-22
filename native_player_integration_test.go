package main

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Opt-in check with bundled mpv and a synthetic clip; never opens user files.
func TestNativePlayerMeasuredWindowSize(t *testing.T) {
	if os.Getenv("TOOLBOX_NATIVE_SIZE_QA") != "1" {
		t.Skip("optional mpv integration check")
	}
	program, _ := filepath.Abs("tools/mpv/mpv.exe")
	clip, _ := filepath.Abs("samples/assets/motion-demo.mp4")
	for _, preset := range []struct {
		size          string
		width, height int
	}{{"compact", 800, 600}, {"medium", 1100, 825}} {
		for _, rotate := range []bool{false, true} {
			endpoint, dir, err := nativePlayerEndpoint(randomID())
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			args := nativePlayerArgs(clip, endpoint, 0, preset.size)
			extra := []string{"--window-minimized=no", "--ontop=no", "--pause=yes", "--mute=yes"}
			if rotate {
				extra = append(extra, "--video-rotate=90")
			}
			args = append(append(args[:len(args)-2], extra...), "--", clip)
			cmd := exec.CommandContext(ctx, program, args...)
			prepareNativePlayerCommand(cmd)
			cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
			if err := cmd.Start(); err != nil {
				cancel()
				t.Fatal(err)
			}
			var conn net.Conn
			func() {
				defer func() {
					cancel()
					_ = cmd.Wait()
					if dir != "" {
						_ = os.Remove(endpoint)
						_ = os.Remove(dir)
					}
				}()
				for ctx.Err() == nil {
					conn, err = dialNativePlayer(ctx, endpoint)
					if err == nil {
						break
					}
					time.Sleep(80 * time.Millisecond)
				}
				if conn == nil {
					t.Error("mpv IPC unavailable")
					return
				}
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
				decoder, encoder := json.NewDecoder(conn), json.NewEncoder(conn)
				for ctx.Err() == nil {
					if err = encoder.Encode(map[string]any{"command": []any{"get_property", "osd-dimensions"}, "request_id": 1}); err != nil {
						t.Error(err)
						return
					}
					for {
						var response struct {
							RequestID int `json:"request_id"`
							Data      struct{ W, H int }
						}
						if err = decoder.Decode(&response); err != nil {
							t.Error(err)
							return
						}
						if response.RequestID != 1 {
							continue
						}
						ratio := 16.0 / 9.0
						if rotate {
							ratio = 9.0 / 16.0
						}
						if response.Data.W > 0 && response.Data.H > 0 && math.Abs(float64(response.Data.W)/float64(response.Data.H)-ratio) < 0.03 {
							t.Logf("size=%s rotated=%v: actual mpv window %dx%d", preset.size, rotate, response.Data.W, response.Data.H)
							if response.Data.W > preset.width || response.Data.H > preset.height {
								t.Error("window exceeded initial size")
							}
							return
						}
						break
					}
					time.Sleep(80 * time.Millisecond)
				}
				t.Error("mpv did not report window dimensions")
			}()
		}
	}
}
