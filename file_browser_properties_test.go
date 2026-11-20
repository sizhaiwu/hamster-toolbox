package main

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

func TestResourceDirectorySizeAuthorizationAndScope(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "album")
	os.MkdirAll(filepath.Join(folder, "sub"), 0700)
	os.WriteFile(filepath.Join(folder, "one.txt"), []byte("123"), 0600)
	os.WriteFile(filepath.Join(folder, "sub", "two.txt"), []byte("4567"), 0600)
	b, _ := newFileBrowser(t.TempDir(), "owner")
	scope, err := toolbox.NewBrowserScope(root)
	if err != nil {
		t.Fatal(err)
	}
	b.scopes["demo"] = scope
	call := func(path, version string, local bool, ctx context.Context) *httptest.ResponseRecorder {
		data, _ := json.Marshal(map[string]string{"Path": path, "Version": version})
		r := httptest.NewRequest("POST", "/api/properties-size?root=demo", bytes.NewReader(data)).WithContext(ctx)
		r.Header.Set("X-Hamster-Browser", "1")
		w := httptest.NewRecorder()
		b.dispatch(w, r, local, "api/properties-size")
		return w
	}
	ref := shortcutTestReference(t, b, folder)
	w := call("album", ref, false, context.Background())
	var result toolbox.DirectorySize
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Size != 7 || result.Files != 2 || result.Directories != 1 || strings.Contains(w.Body.String(), root) {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = call("../outside", "", false, context.Background()); w.Code == 200 {
		t.Fatal("shared size escaped root")
	}
	b.directorySizeRunning = true
	if w = call(folder, "", true, context.Background()); w.Code == 200 {
		t.Fatal("concurrent directory scan accepted")
	}
	b.directorySizeRunning = false
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	call(folder, "", true, ctx)
	if b.directorySizeRunning {
		t.Fatal("cancelled size scan kept global slot")
	}
	if w = call(folder, "invalid", true, context.Background()); w.Code == 200 {
		t.Fatal("stale reference accepted")
	}
	r := httptest.NewRequest("POST", "/files/api/properties-size", bytes.NewBufferString(`{"Path":"/"}`))
	r.Header.Set("X-Hamster-Browser", "1")
	w = httptest.NewRecorder()
	b.local(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatal("size bypassed owner auth")
	}
}

func TestResourcePropertiesScopeAndIdentity(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "sample & [01].txt")
	if err := os.WriteFile(file, []byte("demo"), 0600); err != nil {
		t.Fatal(err)
	}
	b, _ := newFileBrowser(t.TempDir(), "owner")
	ref := shortcutTestReference(t, b, file)
	get := func(path, version string, local bool) *httptest.ResponseRecorder {
		data, _ := json.Marshal(map[string]string{"Path": path, "Version": version})
		r := httptest.NewRequest("POST", "/api/properties?root=demo", bytes.NewReader(data))
		r.Header.Set("X-Hamster-Browser", "1")
		w := httptest.NewRecorder()
		b.dispatch(w, r, local, "api/properties")
		return w
	}
	w := get(file, ref, true)
	var result struct {
		Name, Path string
		Size       int64
		Dir        bool
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Size != 4 || result.Name != filepath.Base(file) || result.Dir {
		t.Fatal("incorrect file properties", w.Code, w.Body.String())
	}
	scope, err := toolbox.NewBrowserScope(root)
	if err != nil {
		t.Fatal(err)
	}
	b.scopes["demo"] = scope
	w = get(filepath.Base(file), ref, false)
	if w.Code != 200 || strings.Contains(w.Body.String(), root) || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Path != filepath.Base(file) {
		t.Fatal("shared metadata exposes host path", w.Code, w.Body.String())
	}
	w = get("", "", false)
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Name != "共享根目录" || !result.Dir || result.Path != "" {
		t.Fatal("shared root properties are not scoped", w.Code, w.Body.String())
	}
	if w = get("../outside", "", false); w.Code != 400 {
		t.Fatal("properties escaped shared root")
	}
	if err = os.WriteFile(file, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if w = get(file, ref, true); w.Code != 400 {
		t.Fatal("stale properties reference accepted")
	}
	r := httptest.NewRequest("POST", "/files/api/properties", bytes.NewBufferString(`{"Path":"/"}`))
	r.Header.Set("X-Hamster-Browser", "1")
	w = httptest.NewRecorder()
	b.local(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatal("properties bypassed local authentication")
	}
}

func TestResourceImagePropertiesReadActualEncoding(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "renamed.jpg")
	var original bytes.Buffer
	if err := png.Encode(&original, image.NewRGBA(image.Rect(0, 0, 17, 9))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, original.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	b, _ := newFileBrowser(t.TempDir(), "owner")
	scope, err := toolbox.NewBrowserScope(root)
	if err != nil {
		t.Fatal(err)
	}
	b.scopes["demo"] = scope
	data, _ := json.Marshal(map[string]string{"Path": "renamed.jpg"})
	r := httptest.NewRequest("POST", "/api/properties?root=demo", bytes.NewReader(data))
	r.Header.Set("X-Hamster-Browser", "1")
	w := httptest.NewRecorder()
	b.dispatch(w, r, false, "api/properties")
	var result struct {
		Width, Height           int
		ImageFormat, MediaError string
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Width != 17 || result.Height != 9 || result.ImageFormat != "png" || result.MediaError != "" || strings.Contains(w.Body.String(), root) {
		t.Fatal(w.Code, w.Body.String())
	}
	after, err := os.ReadFile(file)
	if err != nil || !bytes.Equal(after, original.Bytes()) {
		t.Fatal("properties changed original", err)
	}
}

func TestResourceVideoPropertiesIntegration(t *testing.T) {
	if os.Getenv("TOOLBOX_INTEGRATION") != "1" {
		t.Skip("requires FFprobe")
	}
	program, err := filepath.Abs(filepath.Join("tools", "ffmpeg", "bin", "ffprobe.exe"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(program); err != nil {
		program = toolbox.DefaultConfig().FFprobe
	}
	dataDir, dir := t.TempDir(), t.TempDir()
	settings, _ := json.Marshal(preferences{Version: 1, Workspaces: map[string]toolbox.Config{"video": {FFprobe: program}}})
	if err = os.WriteFile(filepath.Join(dataDir, "settings.json"), settings, 0600); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(filepath.Join("samples", "assets", "motion-demo.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "sample.mp4")
	if err = os.WriteFile(file, original, 0600); err != nil {
		t.Fatal(err)
	}
	b, _ := newFileBrowser(dataDir, "owner")
	data, _ := json.Marshal(map[string]string{"Path": file})
	r := httptest.NewRequest("POST", "/api/properties", bytes.NewReader(data))
	r.Header.Set("X-Hamster-Browser", "1")
	w := httptest.NewRecorder()
	b.dispatch(w, r, true, "api/properties")
	var result struct {
		Width, Height          int
		Duration               float64
		VideoCodec, MediaError string
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Width < 1 || result.Height < 1 || result.Duration < 1 || result.VideoCodec == "" || result.MediaError != "" {
		t.Fatal(w.Code, w.Body.String())
	}
	after, err := os.ReadFile(file)
	if err != nil || !bytes.Equal(after, original) {
		t.Fatal("video properties changed original", err)
	}
}
