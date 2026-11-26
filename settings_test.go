package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestArchiveStartingNumbersMigrationAndPersistence(t *testing.T) {
	s := &server{dataDir: t.TempDir()}
	c := toolbox.DefaultConfig()
	c.Tool, c.ArchiveStart = "archive", 29
	if _, err := s.saveWorkspace(c); err != nil {
		t.Fatal(err)
	}
	p, err := s.loadPreferences()
	if err != nil {
		t.Fatal(err)
	}
	c = p.Workspaces["archive"]
	if c.ArchiveDirStart != 29 || c.ArchiveFileStart != 29 {
		t.Fatal(c)
	}
	c.ArchiveDirStart, c.ArchiveFileStart = 99, 1
	if _, err := s.saveWorkspace(c); err != nil {
		t.Fatal(err)
	}
	restarted := &server{dataDir: s.dataDir}
	p, err = restarted.loadPreferences()
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Workspaces["archive"]; got.ArchiveDirStart != 99 || got.ArchiveFileStart != 1 {
		t.Fatal(got)
	}
}

func TestArchivePasswordIsNeverPersisted(t *testing.T) {
	s := &server{dataDir: t.TempDir()}
	c := toolbox.DefaultConfig()
	c.Tool = "archive"
	c.ArchivePassword = "Private-Test-Secret!"
	c.ArchiveVolume = "500m"
	if _, e := s.saveWorkspace(c); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(filepath.Join(s.dataDir, "settings.json"))
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(b), c.ArchivePassword) {
		t.Fatal("password persisted")
	}
	p, e := s.loadPreferences()
	if e != nil {
		t.Fatal(e)
	}
	if p.Workspaces["archive"].ArchivePassword != "" || p.Workspaces["archive"].ArchiveVolume != "500m" {
		t.Fatal("settings wrong")
	}
	// Also strip stale passwords from an older or manually edited settings file.
	p.Workspaces["archive"] = c
	b, e = json.Marshal(p)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(s.dataDir, "settings.json"), b, 0600); e != nil {
		t.Fatal(e)
	}
	p, e = s.loadPreferences()
	if e != nil || p.Workspaces["archive"].ArchivePassword != "" {
		t.Fatal("stale password returned", e)
	}
	if c.ArchivePassword != "Private-Test-Secret!" {
		t.Fatal("live task password modified")
	}
}

func TestArchiveAdvancedDisclosurePersistence(t *testing.T) {
	s := &server{token: "test-session", dataDir: t.TempDir()}
	p, err := s.loadPreferences()
	if err != nil || p.ArchiveAdvancedOpen {
		t.Fatal("old settings should keep advanced options collapsed", p, err)
	}
	for _, open := range []bool{true, false} {
		w := requestAPI(t, s, "/api/ui-settings", map[string]any{"ArchiveAdvancedOpen": open})
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		// Parameter and other UI saves must preserve this independent preference.
		c := toolbox.DefaultConfig()
		c.Tool = "archive"
		c.ArchiveStart = 29
		if _, err = s.saveWorkspace(c); err != nil {
			t.Fatal(err)
		}
		w = requestAPI(t, s, "/api/ui-settings", map[string]any{"SidebarCollapsed": true, "TaskSound": false})
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		restarted := &server{dataDir: s.dataDir}
		p, err = restarted.loadPreferences()
		if err != nil || p.ArchiveAdvancedOpen != open || !p.SidebarCollapsed || p.TaskSound == nil || *p.TaskSound || p.Workspaces["archive"].ArchiveStart != 29 {
			t.Fatal("UI preference or workspace settings lost after restart", p, err)
		}
	}
}

func TestTemporaryExtractPasswordIsNeverPersisted(t *testing.T) {
	s := &server{dataDir: t.TempDir()}
	c := toolbox.DefaultConfig()
	c.Tool = "extract"
	c.ExtractPassword = "Extract-Secret-Only!"
	if _, e := s.saveWorkspace(c); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(filepath.Join(s.dataDir, "settings.json"))
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(b), c.ExtractPassword) {
		t.Fatal("temporary extraction password persisted")
	}
	p, e := s.loadPreferences()
	if e != nil || p.Workspaces["extract"].ExtractPassword != "" || p.Workspaces["extract"].ExtractConflict != "rename" {
		t.Fatal("extraction settings wrong", e)
	}
}

func TestExtractDeleteModeMigratesToRecycle(t *testing.T) {
	s := &server{dataDir: t.TempDir()}
	old := preferences{Version: 1, LastTool: "extract", Workspaces: map[string]toolbox.Config{"extract": {Tool: "extract", ExtractDeleteSource: true}}}
	b, e := json.Marshal(old)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(s.dataDir, "settings.json"), b, 0600); e != nil {
		t.Fatal(e)
	}
	p, e := s.loadPreferences()
	if e != nil {
		t.Fatal(e)
	}
	c := p.Workspaces["extract"]
	if !c.ExtractDeleteSource || c.ExtractDeleteMode != "recycle" {
		t.Fatal("legacy cleanup did not adopt recoverable default")
	}
	c.ExtractDeleteMode = "permanent"
	if _, e = s.saveWorkspace(c); e != nil {
		t.Fatal(e)
	}
	p, e = s.loadPreferences()
	if e != nil || p.Workspaces["extract"].ExtractDeleteMode != "permanent" {
		t.Fatal("explicit cleanup preference not preserved", e)
	}
}

func TestRetiredCompressionSettingsAreMigrated(t *testing.T) {
	for _, tc := range []struct {
		name          string
		formats, want []string
	}{
		{"mixed", []string{".heic", "PNG", ".jpg", ".gif", ".png"}, []string{".jpg", ".png"}},
		{"only-retired", []string{".avif", ".svg"}, []string{}},
		{"no-selection", []string{}, []string{}},
		{"legacy-default", nil, []string{".jpg", ".jpeg", ".jpe", ".jfif"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &server{dataDir: t.TempDir()}
			original := preferences{Version: 1, LastTool: "image", Workspaces: map[string]toolbox.Config{
				"image": {Tool: "image", Root: "E:/photos", Output: "E:/compressed", ImageFormats: tc.formats, Quality: 75, PngMode: "webp-png", PngQuality: 92},
				"video": {Tool: "video", Root: "E:/videos", CRF: 28},
			}}
			b, e := json.Marshal(original)
			if e != nil {
				t.Fatal(e)
			}
			if e = os.WriteFile(filepath.Join(s.dataDir, "settings.json"), b, 0600); e != nil {
				t.Fatal(e)
			}
			got, e := s.loadPreferences()
			if e != nil {
				t.Fatal(e)
			}
			image := got.Workspaces["image"]
			if !reflect.DeepEqual(image.ImageFormats, tc.want) {
				t.Fatal(image.ImageFormats, tc.want)
			}
			expected := original.Workspaces["image"]
			expected.ImageFormats = tc.want
			expected.Workers = 1
			expectedVideo := original.Workspaces["video"]
			expectedVideo.VideoMode, expectedVideo.VideoAcceleration = "compatible", "auto"
			expectedVideo.Workers = 1
			expectedVideo.VideoHDR = "preserve"
			expectedVideo.VideoContainer = "original"
			if !reflect.DeepEqual(image, expected) || !reflect.DeepEqual(got.Workspaces["video"], expectedVideo) {
				t.Fatal("other settings changed", got)
			}
			if _, e = s.saveWorkspace(image); e != nil {
				t.Fatal(e)
			}
			b, e = os.ReadFile(filepath.Join(s.dataDir, "settings.json"))
			if e != nil {
				t.Fatal(e)
			}
			var saved preferences
			if e = json.Unmarshal(b, &saved); e != nil || !reflect.DeepEqual(saved.Workspaces["image"].ImageFormats, tc.want) {
				t.Fatal(saved, e)
			}
		})
	}
}

func TestWorkspaceSettingsSurviveRestart(t *testing.T) {
	dir := t.TempDir()
	s := &server{dataDir: dir}
	image := toolbox.Config{Tool: "image", Root: "C:/photos", Output: "C:/small-photos", Quality: 75, Magick: "C:/image/magick.exe", FFmpeg: "unused", ImageFormats: []string{".png"}, PngMode: "webp-png", PngQuality: 92}
	video := toolbox.Config{Tool: "video", Root: "D:/videos", Output: "D:/small-videos", CRF: 28, VideoMode: "archive", VideoAcceleration: "intel", FFmpeg: "D:/video/ffmpeg.exe", FFprobe: "D:/video/ffprobe.exe"}
	image.Magick = filepath.Join(dir, "programs", "magick")
	video.FFmpeg = filepath.Join(dir, "programs", "ffmpeg")
	video.FFprobe = filepath.Join(dir, "programs", "ffprobe")
	for _, c := range []toolbox.Config{image, video, image} {
		if _, e := s.saveWorkspace(c); e != nil {
			t.Fatal(e)
		}
	}
	restarted := &server{dataDir: dir}
	p, e := restarted.loadPreferences()
	if e != nil {
		t.Fatal(e)
	}
	if p.LastTool != "image" || len(p.Workspaces) != 2 {
		t.Fatal(p)
	}
	if c := p.Workspaces["image"]; c.Output != image.Output || c.Quality != 75 || c.Magick != image.Magick || c.FFmpeg != "" {
		t.Fatal(c)
	}
	if formats := p.Workspaces["image"].ImageFormats; len(formats) != 1 || formats[0] != ".png" {
		t.Fatal(formats)
	}
	if c := p.Workspaces["image"]; c.PngMode != "webp-png" || c.PngQuality != 92 {
		t.Fatal(c)
	}
	if c := p.Workspaces["video"]; c.Output != video.Output || c.CRF != 28 || c.VideoMode != "archive" || c.VideoAcceleration != "intel" || c.FFmpeg != video.FFmpeg || c.Magick != "" {
		t.Fatal(c)
	}
}

func TestThumbnailSettingsSurviveRestart(t *testing.T) {
	s := &server{dataDir: t.TempDir()}
	c := toolbox.DefaultConfig()
	c.Tool, c.Root, c.Output = "thumbnail", "E:/videos", "E:/sheets"
	c.ThumbnailColumns, c.ThumbnailRows, c.ThumbnailWidth, c.ThumbnailGap = 4, 3, 1280, 7
	c.ThumbnailFormat, c.ThumbnailBackground, c.ThumbnailInfo = "png", "#224466", false
	if _, e := s.saveWorkspace(c); e != nil {
		t.Fatal(e)
	}
	restarted := &server{dataDir: s.dataDir}
	p, e := restarted.loadPreferences()
	if e != nil || p.LastTool != "thumbnail" || !reflect.DeepEqual(p.Workspaces["thumbnail"], toolbox.ScopeConfig(c)) {
		t.Fatal(p, e)
	}
}

func TestDamagedSettingsAreNotOverwritten(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	os.WriteFile(path, []byte("broken-json"), 0600)
	s := &server{dataDir: dir}
	if _, e := s.saveWorkspace(toolbox.Config{Tool: "image"}); e == nil {
		t.Fatal("corruption not reported")
	}
	b, _ := os.ReadFile(path)
	if string(b) != "broken-json" {
		t.Fatal("overwrote existing settings")
	}
}

func requestAPI(t *testing.T, s *server, path string, data any) *httptest.ResponseRecorder {
	t.Helper()
	b, e := json.Marshal(data)
	if e != nil {
		t.Fatal(e)
	}
	req := httptest.NewRequest("POST", path, bytes.NewReader(b))
	req.Header.Set("X-Toolbox-Token", s.token)
	w := httptest.NewRecorder()
	s.guard(s.api)(w, req)
	return w
}

func TestDependencyAPIIsScopedAndProtected(t *testing.T) {
	s := &server{token: "test-session", dataDir: t.TempDir()}
	w := requestAPI(t, s, "/api/dependencies", map[string]any{"Config": toolbox.Config{Tool: "rename"}, "Verify": true})
	if w.Code != 200 || w.Body.String() != "[]\n" {
		t.Fatal(w.Code, w.Body.String())
	}
	w = requestAPI(t, s, "/api/dependencies", map[string]any{"Config": toolbox.Config{Tool: "video"}})
	var deps []toolbox.Dependency
	if e := json.Unmarshal(w.Body.Bytes(), &deps); e != nil {
		t.Fatal(e)
	}
	if len(deps) != 2 || deps[0].Key != "FFmpeg" || deps[1].Key != "FFprobe" {
		t.Fatal(deps)
	}
	if deps[0].Available || deps[1].Available {
		t.Fatal("empty paths marked ready")
	}
	w = httptest.NewRecorder()
	s.guard(s.api)(w, httptest.NewRequest("POST", "/api/settings", bytes.NewBufferString("{}")))
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}

func TestProgramPickerOnlyListsRelevantFiles(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"program.exe", "font.ttf", "photo.jpg"} {
		os.WriteFile(filepath.Join(dir, n), []byte("test"), 0600)
	}
	os.Chmod(filepath.Join(dir, "program.exe"), 0700)
	s := &server{token: "test-session"}
	w := requestAPI(t, s, "/api/browse", map[string]any{"Path": dir, "Kind": "program"})
	var listing struct{ Files []string }
	if e := json.Unmarshal(w.Body.Bytes(), &listing); e != nil {
		t.Fatal(e)
	}
	if len(listing.Files) != 1 || filepath.Base(listing.Files[0]) != "program.exe" {
		t.Fatal(listing)
	}
	w = requestAPI(t, s, "/api/browse", map[string]any{"Path": dir, "Kind": "file"})
	if e := json.Unmarshal(w.Body.Bytes(), &listing); e != nil || len(listing.Files) != 3 {
		t.Fatal("extra-file picker omitted files", e, listing)
	}
}

func TestArchiveExecutionRecordOmitsPasswordIntegration(t *testing.T) {
	if os.Getenv("TOOLBOX_INTEGRATION") == "" {
		t.Skip("set TOOLBOX_INTEGRATION=1")
	}
	c := toolbox.DefaultConfig()
	c.Tool = "archive"
	c.Root = t.TempDir()
	c.SevenZip, _ = filepath.Abs("tools/sevenzip/7za.exe")
	c.ArchivePassword = "Record-Secret!"
	c.ArchiveSplit = "none"
	if e := os.WriteFile(filepath.Join(c.Root, "test.txt"), []byte("resource"), 0600); e != nil {
		t.Fatal(e)
	}
	p, e := toolbox.BuildPlan(context.Background(), c, func(string) {})
	if e != nil {
		t.Fatal(e)
	}
	s := &server{token: "test-session", dataDir: t.TempDir(), plan: p, state: state{Status: "ready", PlanID: "archive-plan", Tool: "archive", Total: len(p.Items), Items: p.Items}}
	w := requestAPI(t, s, "/api/execute", map[string]any{"PlanID": "archive-plan"})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		files, _ := filepath.Glob(filepath.Join(s.dataDir, "task-history", "*.json"))
		if len(files) > 0 {
			b, e := os.ReadFile(files[0])
			var record taskRecord
			if e == nil && json.Unmarshal(b, &record) == nil && record.Status == "finished" {
				if strings.Contains(string(b), c.ArchivePassword) || record.Config.ArchivePassword != "" {
					t.Fatal("record leaked password")
				}
				if record.Result.Success != 1 {
					t.Fatal(record)
				}
				if p.Config.ArchivePassword != c.ArchivePassword {
					t.Fatal("record redaction changed live task")
				}
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("execution record not written")
}

func TestBuiltinPreviewAPI(t *testing.T) {
	for _, tc := range []struct {
		tool, folder, find string
		want               int
	}{
		{"format", "album", "", 1}, {"rename", "album", "old", 1},
		{"format", "album [1P-1B]", "", 0}, {"rename", "album", "no-match", 0},
	} {
		t.Run(tc.tool+"-"+tc.folder+tc.find, func(t *testing.T) {
			root := t.TempDir()
			folder := filepath.Join(root, tc.folder)
			if e := os.Mkdir(folder, 0755); e != nil {
				t.Fatal(e)
			}
			file := filepath.Join(folder, "old.jpg")
			if e := os.WriteFile(file, []byte("x"), 0600); e != nil {
				t.Fatal(e)
			}
			c := toolbox.DefaultConfig()
			c.Tool = tc.tool
			c.Root = root
			c.Recursive = tc.tool == "rename"
			c.Find = tc.find
			c.Replace = "new"
			s := &server{token: "test-session", dataDir: t.TempDir()}
			w := requestAPI(t, s, "/api/plan", c)
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
			deadline := time.Now().Add(3 * time.Second)
			for {
				s.mu.Lock()
				snapshot := s.state
				s.mu.Unlock()
				if snapshot.Status != "scanning" {
					if snapshot.Status != "ready" || snapshot.Total != tc.want || snapshot.Actionable != tc.want {
						t.Fatal(snapshot)
					}
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("preview did not finish")
				}
				time.Sleep(5 * time.Millisecond)
			}
			if _, e := os.Stat(file); e != nil {
				t.Fatal("preview changed input", e)
			}
		})
	}
}

func TestNumberingAPIAndSavedSettings(t *testing.T) {
	root := t.TempDir()
	for _, n := range []string{"作者 - NO.098 已有资源", "新增资源"} {
		if e := os.Mkdir(filepath.Join(root, n), 0755); e != nil {
			t.Fatal(e)
		}
	}
	s := &server{token: "test-session", dataDir: t.TempDir()}
	w := requestAPI(t, s, "/api/number-info", toolbox.Config{Root: root})
	var info toolbox.NumberInfo
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if e := json.Unmarshal(w.Body.Bytes(), &info); e != nil {
		t.Fatal(e)
	}
	if info.Name != "作者" || info.NextNumber != 99 || info.Pending != 1 {
		t.Fatal(info)
	}
	c := toolbox.DefaultConfig()
	c.Tool = "number"
	c.Root = root
	c.NumberName = info.Name
	c.StartNumber = 99
	c.NumberStyle = "underscore"
	c.NumberKeepOriginal = false
	if _, e := s.saveWorkspace(c); e != nil {
		t.Fatal(e)
	}
	restarted := &server{dataDir: s.dataDir}
	p, e := restarted.loadPreferences()
	if e != nil {
		t.Fatal(e)
	}
	restored := p.Workspaces["number"]
	if restored.StartNumber != 99 || restored.NumberStyle != "underscore" || restored.NumberKeepOriginal || restored.Recursive || restored.Magick != "" {
		t.Fatal(restored)
	}
	w = requestAPI(t, s, "/api/plan", restored)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		s.mu.Lock()
		state := s.state
		s.mu.Unlock()
		if state.Status != "scanning" {
			if state.Status != "ready" || state.Total != 1 || state.Actionable != 1 || filepath.Base(state.Items[0].Target) != "作者_NO.099" {
				t.Fatal(state)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("numbering preview timed out")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestCustomNumberingWithoutNOAcrossAPIAndRestart(t *testing.T) {
	root := t.TempDir()
	for _, n := range []string{"作者 098 旧资源", "新增资源"} {
		if e := os.Mkdir(filepath.Join(root, n), 0755); e != nil {
			t.Fatal(e)
		}
	}
	s := &server{token: "test-session", dataDir: t.TempDir()}
	c := toolbox.DefaultConfig()
	c.Tool = "number"
	c.Root = root
	c.NumberName = "作者"
	c.NumberStyle = "custom"
	c.NumberTemplate = "{name} {number}"
	w := requestAPI(t, s, "/api/number-info", c)
	var info toolbox.NumberInfo
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if e := json.Unmarshal(w.Body.Bytes(), &info); e != nil {
		t.Fatal(e)
	}
	if info.NextNumber != 99 || info.Style != "custom" || info.Pending != 1 {
		t.Fatal(info)
	}
	c.StartNumber = info.NextNumber
	if _, e := s.saveWorkspace(c); e != nil {
		t.Fatal(e)
	}
	restarted := &server{dataDir: s.dataDir}
	prefs, e := restarted.loadPreferences()
	if e != nil {
		t.Fatal(e)
	}
	restored := prefs.Workspaces["number"]
	if restored.NumberTemplate != "{name} {number}" || restored.NumberStyle != "custom" {
		t.Fatal(restored)
	}
	p, e := toolbox.BuildPlan(context.Background(), restored, func(string) {})
	if e != nil || len(p.Items) != 1 || filepath.Base(p.Items[0].Target) != "作者 099 新增资源" {
		t.Fatal(p, e)
	}
}

func TestExampleRenameSettingsSurviveRestart(t *testing.T) {
	root := t.TempDir()
	if e := os.Mkdir(filepath.Join(root, "test 第002期 资源"), 0755); e != nil {
		t.Fatal(e)
	}
	c := toolbox.DefaultConfig()
	c.Root = root
	c.Tool = "rename"
	c.Files = false
	c.RenameMode = "example"
	c.Find = "test 第001期"
	c.Replace = "新名称_{编号}"
	s := &server{dataDir: t.TempDir()}
	if _, e := s.saveWorkspace(c); e != nil {
		t.Fatal(e)
	}
	restarted := &server{dataDir: s.dataDir}
	prefs, e := restarted.loadPreferences()
	if e != nil {
		t.Fatal(e)
	}
	restored := prefs.Workspaces["rename"]
	if restored.RenameMode != "example" || !restored.ExampleNumber || !restored.ExampleAtStart {
		t.Fatal(restored)
	}
	p, e := toolbox.BuildPlan(context.Background(), restored, func(string) {})
	if e != nil || len(p.Items) != 1 || filepath.Base(p.Items[0].Target) != "新名称_002 资源" {
		t.Fatal(p, e)
	}
}

func TestFormatCleanupSettingsSurviveRestart(t *testing.T) {
	c := toolbox.DefaultConfig()
	c.Tool, c.FormatCleanOther, c.FormatDeleteMode = "format", true, "permanent"
	s := &server{dataDir: t.TempDir()}
	if _, e := s.saveWorkspace(c); e != nil {
		t.Fatal(e)
	}
	restarted := &server{dataDir: s.dataDir}
	prefs, e := restarted.loadPreferences()
	if e != nil {
		t.Fatal(e)
	}
	got := prefs.Workspaces["format"]
	if !got.FormatCleanOther || got.FormatDeleteMode != "permanent" {
		t.Fatal(got)
	}
	c.Tool = "rename"
	if _, e := s.saveWorkspace(c); e != nil {
		t.Fatal(e)
	}
	prefs, e = restarted.loadPreferences()
	if e != nil || prefs.Workspaces["rename"].FormatCleanOther || prefs.Workspaces["rename"].FormatDeleteMode != "" {
		t.Fatal(prefs, e)
	}
}
