package main

import (
	"strings"
	"testing"
)

func TestPrivateNotesPreviewSourceAnchorsAndFixedBridge(t *testing.T) {
	html, err := notesMarkdown("# heading\n\nparagraph\n\n```js\nconst value = 42;\n```\n\n<script>danger()</script>")
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{`data-note-line="0"`, `data-note-line="2"`, `data-note-line="5"`, "script-src 'sha256-" + notesPreviewScriptHash() + "'", "<script>" + notesPreviewScript + "</script>"} {
		if !strings.Contains(html, text) {
			t.Fatalf("missing %s", text)
		}
	}
	if strings.Count(html, "<script>") != 1 || strings.Contains(html, "<script>danger()") {
		t.Fatal("document injected a script")
	}
	if strings.Contains(html, "allow-same-origin") {
		t.Fatal("preview gained origin privileges")
	}
}
