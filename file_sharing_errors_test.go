package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestSharingListenErrorReportsActualCause(t *testing.T) {
	for _, tc := range []struct {
		cause error
		text  string
	}{
		{syscall.EADDRINUSE, "对外连接占用"},
		{syscall.EACCES, "系统保留端口"},
		{syscall.EPERM, "权限策略"},
		{errors.New("synthetic network error"), "系统网络限制"},
	} {
		cause := &net.OpError{Op: "listen", Net: "tcp4", Err: os.NewSyscallError("bind", tc.cause)}
		err := sharingListenError(8788, cause, "原设置未变")
		if !strings.Contains(err.Error(), tc.text) || !strings.Contains(err.Error(), "8788") || !strings.Contains(err.Error(), "原设置未变") || !errors.Is(err, tc.cause) {
			t.Fatal(err)
		}
	}
}

func TestSharingDefaultPortPreservesSavedPort(t *testing.T) {
	b, err := newFileBrowser(t.TempDir(), "owner")
	if err != nil || b.config.Port != 18788 {
		t.Fatalf("unexpected fresh default: %d, %v", b.config.Port, err)
	}
	for _, port := range []int{8788, 18788, 28788} {
		dir := t.TempDir()
		data, err := json.Marshal(sharingConfig{Port: port, Roots: []sharedFolder{}})
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, "sharing.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
		restored, err := newFileBrowser(dir, "owner")
		if err != nil || restored.config.Port != port {
			t.Fatalf("saved port %d changed: %v", port, err)
		}
	}
}

func TestSharingBindFailureKeepsSavedSettings(t *testing.T) {
	b, err := newFileBrowser(t.TempDir(), "owner")
	if err != nil {
		t.Fatal(err)
	}
	saveTestSharing(t, b, false, "Synthetic-Share-Password!", []sharedFolder{{Name: "demo", Path: t.TempDir()}})
	before, err := os.ReadFile(filepath.Join(b.dataDir, "sharing.json"))
	if err != nil {
		t.Fatal(err)
	}
	b.listen = func(string, string) (net.Listener, error) {
		return nil, &net.OpError{Op: "listen", Net: "tcp4", Err: os.NewSyscallError("bind", syscall.EADDRINUSE)}
	}
	body, _ := json.Marshal(map[string]any{"Enabled": true, "Port": 8788, "Roots": b.config.Roots})
	err = b.saveSharing(httptest.NewRequest("POST", "/", bytes.NewReader(body)))
	if err == nil || !strings.Contains(err.Error(), "对外连接占用") {
		t.Fatal(err)
	}
	after, e := os.ReadFile(filepath.Join(b.dataDir, "sharing.json"))
	if e != nil || !bytes.Equal(before, after) || b.config.Enabled || b.server != nil {
		t.Fatal("bind failure changed settings", e)
	}
}
