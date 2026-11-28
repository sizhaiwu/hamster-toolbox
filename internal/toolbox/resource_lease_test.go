package toolbox

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func resourceTestCache(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old := resourceCacheDir
	resourceCacheDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { resourceCacheDir = old })
	return dir
}
func TestResourceLeaseScopedCache(t *testing.T) {
	profile := resourceTestCache(t)
	scoped := t.TempDir()
	root := t.TempDir()
	ctx := context.WithValue(context.Background(), resourceCacheKey{}, scoped)
	release, err := AcquireResourcePaths(ctx, nil, []string{root}, nil)
	if err != nil {
		t.Fatal(err)
	}
	release()
	entries, err := os.ReadDir(profile)
	if err != nil || len(entries) != 0 {
		t.Fatalf("scoped lease wrote profile cache: %v, %v", entries, err)
	}
	claims, err := os.ReadFile(filepath.Join(scoped, "hamster-toolbox", "resource-leases-v1", "claims.json"))
	if err != nil || strings.TrimSpace(string(claims)) != "[]" {
		t.Fatalf("scoped lease was not released: %s, %v", claims, err)
	}
}
func TestResourceLeaseConflictCancellationAndRelease(t *testing.T) {
	resourceTestCache(t)
	root := t.TempDir()
	paths := []string{root}
	release, e := AcquireResourcePaths(context.Background(), nil, paths, nil)
	if e != nil {
		t.Fatal(e)
	}
	if paths[0] != root {
		t.Fatal("mutated caller paths")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	messages := 0
	if r, e := AcquireResourcePaths(ctx, []string{filepath.Join(root, "nested")}, nil, func(string) { messages++ }); e == nil {
		r()
		t.Fatal("conflict permitted")
	}
	if messages != 1 {
		t.Fatal("waiting logs", messages)
	}
	unrelated, e := AcquireResourcePaths(context.Background(), nil, []string{t.TempDir()}, nil)
	if e != nil {
		t.Fatal(e)
	}
	unrelated()
	release()
	next, e := AcquireResourcePaths(context.Background(), nil, paths, nil)
	if e != nil {
		t.Fatal(e)
	}
	next()
	cancelled, stop := context.WithCancel(context.Background())
	stop()
	if r, e := AcquireResourcePaths(cancelled, nil, paths, nil); e == nil {
		r()
		t.Fatal("cancelled acquired")
	}
}
func TestResourceLeaseMediaIsolation(t *testing.T) {
	root := t.TempDir()
	image := claimForConfig(Config{Tool: "image", Root: root})
	video := claimForConfig(Config{Tool: "video", Root: root})
	format := claimForConfig(Config{Tool: "format", Root: root})
	if claimConflict(image, video) {
		t.Fatal("independent media outputs blocked")
	}
	if !claimConflict(image, format) || !claimConflict(video, format) {
		t.Fatal("rename conflict missed")
	}
	shared := video
	shared.Write = image.Write
	if !claimConflict(image, shared) {
		t.Fatal("shared output not protected")
	}
}
func TestResourceLeaseProcessHelper(t *testing.T) {
	if os.Getenv("HAMSTER_TEST_LEASE_HELPER") != "1" {
		return
	}
	cache := os.Getenv("HAMSTER_TEST_LEASE_CACHE")
	resourceCacheDir = func() (string, error) { return cache, nil }
	release, e := AcquireResourcePaths(context.Background(), nil, []string{os.Getenv("HAMSTER_TEST_LEASE_ROOT")}, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer release()
	if e = os.WriteFile(os.Getenv("HAMSTER_TEST_LEASE_READY"), []byte("ready"), 0600); e != nil {
		t.Fatal(e)
	}
	if os.Getenv("HAMSTER_TEST_LEASE_EXIT") == "1" {
		return
	}
	time.Sleep(20 * time.Second)
}
func TestResourceLeaseCrossProcessAndCrash(t *testing.T) {
	cache := resourceTestCache(t)
	root := t.TempDir()
	ready := filepath.Join(t.TempDir(), "ready")
	release, token, e := AcquireResourcePathsWithToken(context.Background(), nil, []string{root}, nil)
	if e != nil {
		t.Fatal(e)
	}
	helper := func(parent string, exit bool) *exec.Cmd {
		cmd := exec.Command(os.Args[0], "-test.run=^TestResourceLeaseProcessHelper$")
		cmd.Env = append(os.Environ(), "HAMSTER_TEST_LEASE_HELPER=1", "HAMSTER_TEST_LEASE_CACHE="+cache, "HAMSTER_TEST_LEASE_ROOT="+root, "HAMSTER_TEST_LEASE_READY="+ready, "HAMSTER_RESOURCE_PARENT="+parent)
		if exit {
			cmd.Env = append(cmd.Env, "HAMSTER_TEST_LEASE_EXIT=1")
		}
		return cmd
	}
	// A declared parent script can invoke the CLI without waiting on its own reservation.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	child := helper(token, true)
	child.Cancel = nil
	if e = child.Start(); e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	select {
	case e = <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-ctx.Done():
		child.Process.Kill()
		<-done
		t.Fatal("parent-child lease deadlock")
	}
	release()
	os.Remove(ready)
	holder := helper("", false)
	if e = holder.Start(); e != nil {
		t.Fatal(e)
	}
	defer func() { holder.Process.Kill(); holder.Wait() }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, e = os.Stat(ready); e == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("holder not ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	blocked, cancelBlocked := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancelBlocked()
	if r, e := AcquireResourcePaths(blocked, []string{root}, nil, nil); e == nil {
		r()
		t.Fatal("different process bypassed lease")
	}
	holder.Process.Kill()
	holder.Wait()
	next, cancelNext := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelNext()
	r, e := AcquireResourcePaths(next, nil, []string{root}, nil)
	if e != nil {
		t.Fatal("stale crash lease blocked", e)
	}
	r()
	b, _ := os.ReadFile(filepath.Join(cache, "hamster-toolbox", "resource-leases-v1", "claims.json"))
	if strings.Contains(string(b), "Script") || strings.Contains(string(b), "Password") {
		t.Fatal("unexpected config in lock file")
	}
}
