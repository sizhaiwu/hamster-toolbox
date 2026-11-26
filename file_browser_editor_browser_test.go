package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileEditorBrowserAcceptance(t *testing.T) {
	output := os.Getenv("TOOLBOX_EDITOR_QA")
	if output == "" {
		t.Skip("optional synthetic editor browser fixture")
	}
	s := &server{dataDir: t.TempDir(), token: "synthetic-editor-owner"}
	b, err := newFileBrowser(s.dataDir, s.token)
	if err != nil {
		t.Fatal(err)
	}
	s.browser = b
	root := t.TempDir()
	file := filepath.Join(root, "editor-demo.json")
	content := "{\r\n  \"name\": \"Editor demo\",\r\n  \"enabled\": true,\r\n  \"count\": 42,\r\n  \"description\": \"中文代码编辑示例\"\r\n}\r\n"
	if err = os.WriteFile(file, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(filepath.Join(root, "src"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "src", "app.js"), []byte("// Synthetic code sample\nexport function greet(name) {\n  const message = `Hello ${name}`;\n  return {message, enabled: true, count: 42};\n}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "README.md"), []byte("# Demo project\n\nSynthetic files for code editor testing.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "test.go"), []byte("package main\n\nfunc main() {\n  println(\"1\")\n}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "large.ts"), []byte(strings.Repeat("const sampleValue = 42;\n", 70000)), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "large-preview.txt"), []byte(strings.Repeat("Synthetic text preview row.\n", 30000)), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "src", "app.ts"), []byte("export const sampleValue: number = 42;\nexport function greet(name: string): string {\n  return `Hello ${name}`;\n}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "recording.ts"), syntheticTransportStream(188, 0), 0600); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(s.httpHandler())
	defer srv.Close()
	data, _ := json.Marshal(map[string]string{"URL": srv.URL + "/files/?session=" + s.token, "Root": root, "File": file, "StopFile": output + ".stop"})
	if err = os.WriteFile(output, data, 0600); err != nil {
		t.Fatal(err)
	}
	for end := time.Now().Add(8 * time.Minute); time.Now().Before(end); time.Sleep(100 * time.Millisecond) {
		if _, err = os.Stat(output + ".stop"); err == nil {
			return
		}
	}
}
