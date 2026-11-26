package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

func quickExtractFixture(t *testing.T) (*server, *fileBrowser, string, string) {
	t.Helper()
	s := &server{dataDir: t.TempDir(), token: "synthetic-inline-extract-owner"}
	b, err := newFileBrowser(s.dataDir, s.token)
	if err != nil {
		t.Fatal(err)
	}
	s.browser = b
	root := t.TempDir()
	source := filepath.Join(root, "demo.zip")
	f, err := os.Create(source)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	member, err := w.Create("hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	member.Write([]byte("new synthetic content"))
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
	os.WriteFile(filepath.Join(root, "unrelated.zip"), []byte("not an archive; must not be scanned"), 0600)
	program, _ := filepath.Abs("tools/sevenzip/7z.exe")
	if _, err = os.Stat(program); err != nil {
		t.Fatal(err)
	}
	if _, err = s.saveWorkspace(toolbox.Config{Tool: "extract", SevenZip: program}); err != nil {
		t.Fatal(err)
	}
	return s, b, root, source
}

func TestExplorerInlineExtractionWorkflow(t *testing.T) {
	if os.Getenv("TOOLBOX_INTEGRATION") == "" {
		t.Skip("optional real 7-Zip integration")
	}
	s, b, root, source := quickExtractFixture(t)
	defer s.taskWait.Wait()
	handler := s.httpHandler()
	call := func(route string, body any, auth bool) *httptest.ResponseRecorder {
		data, _ := json.Marshal(body)
		r := httptest.NewRequest("POST", route, bytes.NewReader(data))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Toolbox-Tool", "extract")
		if auth {
			r.Header.Set("X-Toolbox-Token", s.token)
			r.Header.Set("X-Hamster-Browser", "1")
			r.AddCookie(&http.Cookie{Name: "hamster_files_" + b.cookieTag, Value: b.localCookie})
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if w := call("/api/plan", map[string]string{"Tool": "extract", "Root": root}, false); w.Code != 403 {
		t.Fatal("anonymous extraction allowed", w.Code)
	}
	listing := call("/files/api/list", map[string]string{"Path": root}, true)
	var files struct{ Entries []browserEntry }
	if err := json.Unmarshal(listing.Body.Bytes(), &files); err != nil {
		t.Fatal(err)
	}
	version := ""
	for _, item := range files.Entries {
		if item.Path == source {
			version = item.Version
		}
	}
	if version == "" {
		t.Fatal("archive reference missing", listing.Body.String())
	}
	if w := call("/files/api/properties", map[string]string{"Path": source, "Version": version}, true); w.Code != 200 {
		t.Fatal("selected file check", w.Body.String())
	}
	program, _ := filepath.Abs("tools/sevenzip/7z.exe")
	c := toolbox.Config{Tool: "extract", Root: root, ExtractSource: source, SevenZip: program, ExtractConflict: "rename", ExtractLayout: "separate", ExtractDeleteSource: false}
	original, _ := os.ReadFile(source)
	w := call("/api/plan", c, true)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var st state
	wait := func(phase string) {
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			w := call("/api/state", map[string]bool{"Summary": true}, true)
			if json.Unmarshal(w.Body.Bytes(), &st) != nil {
				t.Fatal(w.Body.String())
			}
			if st.Status == phase {
				return
			}
			if st.Status == "error" {
				t.Fatal(st.Error)
			}
			time.Sleep(30 * time.Millisecond)
		}
		t.Fatal("inline extraction timeout", phase, st.Status)
	}
	wait("ready")
	if st.Actionable != 1 || st.Conflicts != 0 || st.Config.ExtractSource != source {
		t.Fatal("scanned unrelated archive", st)
	}
	preview := call("/api/preview", map[string]any{"PlanID": st.PlanID, "Limit": 100}, true)
	var page previewPage
	if json.Unmarshal(preview.Body.Bytes(), &page) != nil || len(page.Items) != 1 {
		t.Fatal(preview.Body.String())
	}
	target := page.Items[0].Target
	os.MkdirAll(target, 0700)
	os.WriteFile(filepath.Join(target, "hello.txt"), []byte("existing content"), 0600)
	if w := call("/api/execute", map[string]string{"PlanID": "stale"}, true); w.Code != 400 {
		t.Fatal("stale plan executed", w.Code)
	}
	if w := call("/api/execute", map[string]string{"PlanID": st.PlanID}, true); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	wait("finished")
	if st.Result.Success != 1 || st.Result.Failed != 0 || st.HistoryID == "" {
		t.Fatal("execution/history failed", st)
	}
	if after, _ := os.ReadFile(source); !bytes.Equal(after, original) {
		t.Fatal("source archive changed")
	}
	if old, _ := os.ReadFile(filepath.Join(target, "hello.txt")); string(old) != "existing content" {
		t.Fatal("existing file overwritten")
	}
	entries, _ := os.ReadDir(target)
	if len(entries) != 2 {
		t.Fatal("renamed extraction missing", entries)
	}
	if text, _ := os.ReadFile(filepath.Join(root, "unrelated.zip")); !strings.Contains(string(text), "must not be scanned") {
		t.Fatal("unrelated archive changed")
	}
}

func TestQuickExtractBrowserAcceptance(t *testing.T) {
	output := os.Getenv("TOOLBOX_QUICK_EXTRACT_QA")
	if output == "" {
		t.Skip("optional synthetic browser fixture")
	}
	s, _, root, _ := quickExtractFixture(t)
	srv := httptest.NewServer(s.httpHandler())
	defer srv.Close()
	defer s.taskWait.Wait()
	data, _ := json.Marshal(map[string]string{"URL": srv.URL + "/files/?session=" + s.token, "Root": root, "StopFile": output + ".stop"})
	if err := os.WriteFile(output, data, 0600); err != nil {
		t.Fatal(err)
	}
	for end := time.Now().Add(8 * time.Minute); time.Now().Before(end); time.Sleep(100 * time.Millisecond) {
		if _, err := os.Stat(output + ".stop"); err == nil {
			return
		}
	}
}
