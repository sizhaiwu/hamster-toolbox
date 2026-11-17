package toolbox

import (
	"bytes"
	"context"
	"crypto/sha256"
	"debug/pe"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

func executableDirectory() string {
	exe, _ := os.Executable()
	return filepath.Dir(exe)
}

func regularFile(path string) bool {
	s, e := os.Stat(path)
	return e == nil && s.Mode().IsRegular()
}

func bundledDependency(base, key string) string {
	var candidates []string
	switch key {
	case "FFmpeg", "FFprobe":
		name := strings.ToLower(key) + ".exe"
		candidates = []string{filepath.Join("ffmpeg", "bin", name), filepath.Join("ffmpeg", name), name}
	case "Magick":
		candidates = []string{filepath.Join("imagemagick", "magick.exe"), filepath.Join("imagemagick", "bin", "magick.exe"), "magick.exe"}
	case "Exiftool":
		candidates = []string{filepath.Join("exiftool", "exiftool.exe"), "exiftool.exe"}
	case "Font":
		for _, ext := range []string{".otf", ".ttf", ".ttc"} {
			candidates = append(candidates, filepath.Join("fonts", "NotoSansCJKsc-Regular"+ext))
		}
	}
	for _, rel := range candidates {
		path := filepath.Join(base, "tools", rel)
		if regularFile(path) {
			return path
		}
	}
	return ""
}

func dependencyField(c *Config, key string) *string {
	switch key {
	case "FFmpeg":
		return &c.FFmpeg
	case "FFprobe":
		return &c.FFprobe
	case "Magick":
		return &c.Magick
	case "Exiftool":
		return &c.Exiftool
	case "Font":
		return &c.Font
	}
	return nil
}

func portableSuffix(path string) string {
	s := strings.ReplaceAll(path, "\\", "/")
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "tools/") {
		return filepath.FromSlash(s)
	}
	if i := strings.LastIndex(lower, "/tools/"); i >= 0 {
		return filepath.FromSlash(s[i+1:])
	}
	return ""
}

func dependencyPathsAt(c Config, base string, restore bool) Config {
	for _, d := range RequiredDependencies(c) {
		field := dependencyField(&c, d.Key)
		if field == nil {
			continue
		}
		original := *field
		if restore {
			// Rebind old absolute bundle paths even if the old copy still exists.
			if rel := portableSuffix(original); rel != "" {
				candidate := filepath.Join(base, rel)
				if within(filepath.Join(base, "tools"), candidate) && regularFile(candidate) {
					*field = candidate
					continue
				}
			}
		}
		if original != "" && !filepath.IsAbs(original) && (d.Key == "Font" || strings.ContainsAny(original, "/\\")) {
			*field = filepath.Join(base, filepath.FromSlash(strings.ReplaceAll(original, "\\", "/")))
		}
		if restore {
			current := d
			current.Path = *field
			if _, e := dependencyLocation(current); e != nil {
				if found := bundledDependency(base, d.Key); found != "" {
					*field = found
				}
			}
		}
	}
	return c
}

// ResolveDependencyPaths makes relative dependency paths independent of the
// caller's working directory. Explicit invalid selections still report errors.
func ResolveDependencyPaths(c Config) Config {
	return dependencyPathsAt(c, executableDirectory(), false)
}

// RestoreDependencyPaths repairs persisted paths after a portable folder moves.
// Valid custom programs outside the bundle retain priority.
func RestoreDependencyPaths(c Config) Config {
	return dependencyPathsAt(c, executableDirectory(), true)
}

func portableConfigAt(c Config, base string) Config {
	for _, d := range RequiredDependencies(c) {
		field := dependencyField(&c, d.Key)
		if field == nil || !filepath.IsAbs(*field) || !within(filepath.Join(base, "tools"), *field) {
			continue
		}
		if rel, e := filepath.Rel(base, *field); e == nil {
			*field = filepath.ToSlash(rel)
		}
	}
	return c
}

// PortableConfig stores only bundled dependencies as relative paths. User input
// and output directories, and external custom programs, are not relocated.
func PortableConfig(c Config) Config { return portableConfigAt(c, executableDirectory()) }

// CheckPortableBundle deliberately ignores PATH, registry and developer paths.
func CheckPortableBundle(ctx context.Context) []Dependency {
	base := executableDirectory()
	c := Config{Tool: "thumbnail", FFmpeg: bundledDependency(base, "FFmpeg"), FFprobe: bundledDependency(base, "FFprobe"), Magick: bundledDependency(base, "Magick"), Font: bundledDependency(base, "Font")}
	deps := CheckDependencies(ctx, c, true)
	deps = append(deps, CheckDependencies(ctx, Config{Tool: "exif", Exiftool: bundledDependency(base, "Exiftool")}, true)...)
	runtimePath := filepath.Join(base, "tools", "imagemagick", "vcomp140.dll")
	runtime := Dependency{Key: "OpenMP", Name: "ImageMagick OpenMP runtime", Purpose: "随包图片组件运行库", Path: runtimePath, Message: "缺少 x64 运行库，请使用完整版"}
	if f, e := pe.Open(runtimePath); e == nil {
		runtime.Available = f.Machine == pe.IMAGE_FILE_MACHINE_AMD64
		f.Close()
		if runtime.Available {
			runtime.Verified = true
			runtime.Message = "Bundled x64 OpenMP runtime"
		}
	}
	deps = append(deps, runtime)
	return deps
}

func PortableBundleWarning(expected string) string {
	return portableBundleWarningAt(executableDirectory(), expected)
}

func portableBundleWarningAt(base, expected string) string {
	if expected == "" {
		return ""
	}
	manifest, e := os.ReadFile(filepath.Join(base, "tools-manifest.json"))
	if e != nil {
		return "此程序需要配套的完整版目录，请完整解压发布包，保留 tools 文件夹。"
	}
	sum := sha256.Sum256(manifest)
	if !strings.EqualFold(hex.EncodeToString(sum[:]), expected) {
		return "组件清单与程序版本不匹配，请使用此版本的完整版。"
	}
	var marker struct{ ManifestSHA256 string }
	b, e := os.ReadFile(filepath.Join(base, "tools", "bundle-files.json"))
	if e != nil || json.Unmarshal(bytes.TrimPrefix(b, []byte{0xef, 0xbb, 0xbf}), &marker) != nil || !strings.EqualFold(marker.ManifestSHA256, expected) {
		return "内置组件缺失或版本不匹配，请下载此版本的完整版并完整解压。"
	}
	return ""
}
