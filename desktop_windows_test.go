//go:build windows

package main

import (
	"bufio"
	"context"
	"debug/pe"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestDesktopGUIAndSingleInstanceIntegration(t *testing.T) {
	exe := os.Getenv("TOOLBOX_DESKTOP_TEST_EXE")
	if exe == "" {
		t.Skip("requires a freshly built GUI test executable")
	}
	binary, err := pe.Open(exe)
	if err != nil {
		t.Fatal(err)
	}
	header, ok := binary.OptionalHeader.(*pe.OptionalHeader64)
	if !ok || header.Subsystem != 2 {
		t.Fatal("desktop executable has a console subsystem")
	}
	binary.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	data := t.TempDir()
	first := exec.CommandContext(ctx, exe, "-tray", "-no-open", "-data", data)
	stdout, err := first.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = first.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { first.Process.Kill(); first.Wait() }()
	addresses := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "http://127.0.0.1:") {
				addresses <- line
				return
			}
		}
	}()
	var address string
	select {
	case address = <-addresses:
	case <-ctx.Done():
		t.Fatal("desktop startup did not emit a local URL")
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := http.Client{Timeout: 3 * time.Second, Jar: jar}
	var response *http.Response
	for tries := 0; tries < 20; tries++ {
		response, err = client.Get(address)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal(response.StatusCode)
	}
	// Exercise the real static allowlists; DOM mocks do not fetch script tags.
	ownerURL, e := url.Parse(address)
	if e != nil {
		t.Fatal(e)
	}
	ownerURL.Path = "/files/"
	ownerPage, e := client.Get(ownerURL.String())
	if e != nil {
		t.Fatal(e)
	}
	ownerPage.Body.Close()
	if ownerPage.StatusCode != 200 {
		t.Fatal("owner Explorer bootstrap failed", ownerPage.StatusCode)
	}
	for _, path := range []string{"/version-info.js", "/files/version-info.js"} {
		u, e := url.Parse(address)
		if e != nil {
			t.Fatal(e)
		}
		u.Path = path
		u.RawQuery = ""
		asset, e := client.Get(u.String())
		if e != nil {
			t.Fatal(e)
		}
		body, e := io.ReadAll(io.LimitReader(asset.Body, 128<<10))
		asset.Body.Close()
		if e != nil || asset.StatusCode != 200 || !strings.Contains(string(body), "window.toolboxVersion") || !strings.Contains(asset.Header.Get("Content-Type"), "javascript") {
			t.Fatal("version script not served", path, asset.StatusCode, e)
		}
	}
	second := exec.CommandContext(ctx, exe, "-no-open", "-data", data)
	if err = second.Run(); err != nil {
		t.Fatal("duplicate launcher failed", err)
	}
	response, err = client.Get(address)
	if err != nil {
		t.Fatal("original instance stopped", err)
	}
	response.Body.Close()
}
