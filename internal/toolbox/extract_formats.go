package toolbox

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
)

// Archive extensions only: do not treat ordinary media, Office documents or
// executable files as batch extraction inputs just because 7-Zip can open them.
const extractArchiveExtensions = ".7z .zip .zipx .rar .tar .gz .gzip .bz2 .bzip2 .xz .zst .zstd .lzma .lzma86 .z .cab .arj .lzh .lha .cpio .ar .deb .udeb .rpm .xar .pkg .xip .iso .udf .wim .esd .dmg .squashfs .cramfs .jar .apk .xpi .epub .ipa .appx"

func disguisedExtractArchive(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	for _, prefix := range []string{".7z", ".zip", ".rar"} {
		if ext != prefix && strings.HasPrefix(ext, prefix) && !has(path, extractArchiveExtensions) {
			return true
		}
	}
	return false
}

func compressedTAR(path string) (string, string) {
	name := filepath.Base(path)
	switch strings.ToLower(filepath.Ext(name)) {
	case ".gz", ".gzip", ".tgz", ".tpz", ".bz2", ".bzip2", ".tbz2", ".tbz", ".xz", ".txz", ".zst", ".zstd", ".tzst", ".lzma", ".tlz", ".z", ".taz":
	default:
		return "", ""
	}
	lowerName := strings.ToLower(name)
	for _, group := range []struct{ format, suffixes string }{
		{"gzip", ".tar.gz .tar.gzip .tgz .tpz"},
		{"bzip2", ".tar.bz2 .tar.bzip2 .tbz2 .tbz"},
		{"xz", ".tar.xz .txz"},
		{"zstd", ".tar.zst .tar.zstd .tzst"},
		{"lzma", ".tar.lzma .tlz"},
		{"Z", ".tar.z .taz"},
	} {
		for _, suffix := range strings.Fields(group.suffixes) {
			if strings.HasSuffix(lowerName, suffix) {
				return name[:len(name)-len(suffix)], group.format
			}
		}
	}
	return "", ""
}

// Stream the outer compression directly into the TAR reader. Neither scan nor
// execution writes an intermediate TAR, and both processes share cancellation.
// Listing and extraction use the same pipeline, retaining member safety checks.
func runCompressedTAR(ctx context.Context, c Config, source, format string, notify func(int), args ...string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(args) < 3 || args[len(args)-2] != "--" || args[len(args)-1] != source {
		return nil, errors.New("复合 TAR 解压参数无效")
	}
	innerArgs := append([]string(nil), args[:len(args)-2]...)
	innerArgs = append(innerArgs, "-si", "-ttar")
	outerArgs := []string{"x", "-so", "-t" + format, "-y", "-p" + c.ExtractPassword}
	if notify != nil {
		outerArgs = append(outerArgs, "-bsp2")
	} else {
		outerArgs = append(outerArgs, "-bd")
	}
	outerArgs = append(outerArgs, "--", source)
	outer := exec.CommandContext(ctx, c.SevenZip, outerArgs...)
	inner := exec.CommandContext(ctx, c.SevenZip, innerArgs...)
	hideWindow(outer)
	hideWindow(inner)
	outer.Stdin = strings.NewReader("")
	var outerError, innerError, progressOutput videoErrorBuffer
	var output bytes.Buffer
	outer.Stderr = &outerError
	inner.Stderr = &innerError
	inner.Stdout = &output
	if notify != nil {
		inner.Stdout = &progressOutput
		outer.Stderr = io.MultiWriter(&outerError, &sevenZipProgressWriter{notify: notify})
	}
	pipe, err := outer.StdoutPipe()
	if err != nil {
		return nil, err
	}
	defer pipe.Close()
	inner.Stdin = pipe
	if err = outer.Start(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("无法启动 7-Zip，请检查程序路径和组件")
	}
	innerErr := inner.Run()
	pipe.Close() // An early inner failure must not leave the producer blocked.
	outerErr := outer.Wait()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if innerErr != nil || outerErr != nil {
		return nil, classifyExtractError(c, source, outerError.String()+"\n"+innerError.String()+"\n"+output.String()+"\n"+progressOutput.String())
	}
	return output.Bytes(), nil
}
