package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// Optional isolated browser fixture: no saved user notes or settings are opened.
func TestMobileNotesBrowserAcceptance(t *testing.T) {
	output := os.Getenv("TOOLBOX_MOBILE_NOTES_QA")
	if output == "" {
		t.Skip("optional mobile notes browser fixture")
	}
	s, session := notesNew(t)
	b, err := newFileBrowser(s.dataDir, s.token)
	if err != nil {
		t.Fatal(err)
	}
	s.browser = b
	defer s.notes.lock()
	notesSave(t, s, session, privateNote{Title: "手机滚动测试", Markdown: "# 手机编辑与滚动\n\n" + strings.Repeat("这是合成测试内容，用于检查手机上下滚动、输入光标和键盘遮挡。\n\n", 80)})
	srv := httptest.NewServer(s.httpHandler())
	defer srv.Close()
	data, _ := json.Marshal(map[string]string{"URL": srv.URL + "/notes/?session=" + s.token, "Password": testNotesPassword, "StopFile": output + ".stop"})
	if err := os.WriteFile(output, data, 0600); err != nil {
		t.Fatal(err)
	}
	for end := time.Now().Add(5 * time.Minute); time.Now().Before(end); time.Sleep(100 * time.Millisecond) {
		if _, err := os.Stat(output + ".stop"); err == nil {
			return
		}
	}
	t.Fatal("mobile browser fixture timed out")
}
