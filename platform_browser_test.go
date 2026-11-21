package main

import (
	"os/exec"
	"reflect"
	"testing"
)

func TestBrowserCommandPlatformFallback(t *testing.T) {
	for _, c := range []struct {
		platform  string
		available map[string]string
		want      []string
	}{
		{"linux", map[string]string{"xdg-open": "/usr/bin/xdg-open", "gio": "/usr/bin/gio"}, []string{"/usr/bin/xdg-open", "http://127.0.0.1:8787/"}},
		{"linux", map[string]string{"gio": "/usr/bin/gio"}, []string{"/usr/bin/gio", "open", "http://127.0.0.1:8787/"}},
		{"darwin", nil, []string{"open", "http://127.0.0.1:8787/"}},
		{"windows", nil, []string{"rundll32", "url.dll,FileProtocolHandler", "http://127.0.0.1:8787/"}},
	} {
		t.Run(c.platform+"-"+c.want[0], func(t *testing.T) {
			cmd := browserCommandFor(c.platform, "http://127.0.0.1:8787/", func(name string) (string, error) {
				if path, ok := c.available[name]; ok {
					return path, nil
				}
				return "", exec.ErrNotFound
			})
			if !reflect.DeepEqual(cmd.Args, c.want) {
				t.Fatal(cmd.Args)
			}
		})
	}
}
