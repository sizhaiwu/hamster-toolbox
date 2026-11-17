package toolbox

import (
	"fmt"
	"sort"
	"strings"
)

// Compression uses a narrower allowlist than image statistics and metadata tools.
const ImageCompressionFormats = ".jpg .jpeg .jpe .jfif .png .webp"

func defaultImageFormats() []string { return []string{".jpg", ".jpeg", ".jpe", ".jfif"} }

func imageFormatExtension(raw string) string {
	ext := strings.ToLower(strings.TrimSpace(raw))
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return ext
}

func isImageCompressionFormat(ext string) bool {
	for _, supported := range strings.Fields(ImageCompressionFormats) {
		if ext == supported {
			return true
		}
	}
	return false
}

// RestoreImageFormats removes retired options from saved settings without
// selecting new formats. An empty selection requires the user to choose again.
func RestoreImageFormats(formats []string) []string {
	if formats == nil {
		return defaultImageFormats()
	}
	result := make([]string, 0, len(formats))
	seen := map[string]bool{}
	for _, raw := range formats {
		ext := imageFormatExtension(raw)
		if isImageCompressionFormat(ext) && !seen[ext] {
			seen[ext] = true
			result = append(result, ext)
		}
	}
	sort.Strings(result)
	return result
}

func normalizeImageFormats(formats []string) ([]string, error) {
	// Older settings have no selection. Use the conservative JPEG-only preset.
	if formats == nil {
		formats = defaultImageFormats()
	}
	if len(formats) == 0 {
		return nil, fmt.Errorf("请至少选择一种要压缩的图片格式")
	}
	for _, raw := range formats {
		if !isImageCompressionFormat(imageFormatExtension(raw)) {
			return nil, fmt.Errorf("暂不支持压缩此格式：%s；请选择 JPEG、PNG 或 WebP", raw)
		}
	}
	return RestoreImageFormats(formats), nil
}
