package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// Opt-in browser fixture using only a temporary vault and public sample image.
func TestPrivateNotesImageBrowserAcceptance(t *testing.T) {
	output := os.Getenv("TOOLBOX_NOTES_IMAGE_QA")
	if output == "" {
		t.Skip("optional attachment image browser fixture")
	}
	s, session := notesNew(t)
	defer s.notes.lock()
	b, err := newFileBrowser(s.dataDir, s.token)
	if err != nil {
		t.Fatal(err)
	}
	s.browser = b
	n := notesSave(t, s, session, privateNote{Title: "图片附件演示", Markdown: "# 图片附件演示\n\n这是一篇合成笔记。图片只作为附件保存，没有插入正文；点击底部的查看图片即可打开。"})
	png, err := os.ReadFile("samples/assets/transparent-card.png")
	if err != nil {
		t.Fatal(err)
	}
	notesJSON(t, notesCall(t, s, session, "attachment-add", notesRequest{Note: n, Name: "image.png", Content: png}))
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
	t.Fatal("attachment image browser fixture timed out")
}
