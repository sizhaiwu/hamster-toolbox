//go:build darwin

package toolbox

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDarwinStandardSystemAliases(t *testing.T) {
	for _, alias := range []string{"/var", "/tmp", "/etc"} {
		t.Run(alias, func(t *testing.T) {
			if _, err := os.Readlink(alias); err != nil {
				t.Skip("standard system alias unavailable")
			}
			want, err := filepath.EvalSymlinks(alias)
			if err != nil {
				t.Fatal(err)
			}
			if want != "/private"+alias {
				t.Skip("system alias has a nonstandard target")
			}
			got, err := resolveOrdinaryPath(alias, true)
			if err != nil || got != want {
				t.Fatalf("standard alias: got %q, %v; want %q", got, err, want)
			}
			if _, err = safePath(alias); err == nil {
				t.Fatal("system alias bypassed write protection")
			}
		})
	}
}
