package toolbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeImageArgsAndPasswordSafety(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source.png")
	file(t, src, []byte("fixture"))
	c := DefaultConfig()
	c.Tool, c.Root, c.Output, c.PngMode = "image", root, filepath.Join(root, "output"), "webp-png"
	it := Item{Source: src, Target: filepath.Join(c.Output, "source.png")}
	commands, e := NativeCommands(context.Background(), c, it)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(commands[0].PowerShell, "WEBP:") || !strings.Contains(commands[0].PowerShell, "Test-Path -LiteralPath") {
		t.Fatal(commands)
	}
	c.Tool, c.ArchiveMode, c.ArchivePassword = "archive", "pure", "private-password"
	it.Target = filepath.Join(c.Output, "F_001.7z.001")
	commands, e = NativeCommands(context.Background(), c, it)
	if e != nil {
		t.Fatal(e)
	}
	code := commands[0].PowerShell
	if strings.Contains(code, c.ArchivePassword) || !strings.Contains(code, "TOOLBOX_ARCHIVE_PASSWORD") {
		t.Fatal("password leaked", code)
	}
	if strings.Index(code, "('-p'+") > strings.Index(code, "'--'") {
		t.Fatal("password switch placed after option delimiter", code)
	}
	c.ArchiveMode = "extras"
	if _, e = NativeCommands(context.Background(), c, it); e == nil {
		t.Fatal("multi-step archive exported as incomplete native command")
	}
	if _, e = os.Stat(src); e != nil {
		t.Fatal("command generation changed source", e)
	}
}
