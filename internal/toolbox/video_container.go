package toolbox

import (
	"fmt"
	"path/filepath"
	"strings"
)

func videoMuxer(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp4", ".m4v":
		return "mp4"
	case ".mov", ".qt":
		return "mov"
	case ".mkv":
		return "matroska"
	case ".ts":
		return "mpegts"
	}
	return ""
}

func videoOutputPath(c Config, path string) (string, error) {
	if c.VideoContainer == "mp4" {
		return strings.TrimSuffix(path, filepath.Ext(path)) + ".mp4", nil
	}
	if videoMuxer(path) == "" {
		return "", fmt.Errorf("保留原格式暂不支持 %s 的当前压缩编码，请选择“统一 MP4”；原件保留", filepath.Ext(path))
	}
	return path, nil
}

func videoContainerCodecError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "could not find tag for codec hevc") ||
		strings.Contains(s, "hevc") && strings.Contains(s, "not currently supported in container")
}
