package toolbox

import (
	"regexp"
	"strings"
)

var directoryStatsBlock = regexp.MustCompile(`\[[^\[\]]+\]`)
var directoryMediaStats = regexp.MustCompile(`(?i)^\d+\s*[PVA](?:[\s,+，-]*\d+\s*[PVA])*(?:[\s,+，-]*\d+(?:\.\d+)?\s*(?:B|[KMGTP](?:I?B)?))?$`)
var directorySizeStats = regexp.MustCompile(`(?i)^\d+(?:\.\d+)?\s*(?:B|[KMGTP]I?B)$`)
var directoryResolutionTag = regexp.MustCompile(`(?i)^(?:480|576|720|1080|1440|2160|4320|8640)\s*P$`)
var directoryExtraSpaces = regexp.MustCompile(`[ \t]{2,}`)

func stripDirectoryStatsSuffix(name string) string {
	changed := false
	base := directoryStatsBlock.ReplaceAllStringFunc(name, func(block string) string {
		value := strings.TrimSpace(block[1 : len(block)-1])
		if directoryResolutionTag.MatchString(value) {
			return block
		}
		if directoryMediaStats.MatchString(value) || directorySizeStats.MatchString(value) {
			changed = true
			return ""
		}
		return block
	})
	if changed {
		base = directoryExtraSpaces.ReplaceAllString(base, " ")
	}
	return strings.TrimSpace(base)
}

// Format statistics recognize media independently from conversion support.
// A video need not be supported by a compressor to be preserved during cleanup.
func formatMediaKind(path string) int {
	switch {
	case has(path, Images+" .pdf .jp2 .j2k .jxl .jxr .tga .dng .cr2 .cr3 .nef .arw .rw2 .orf .raf .pef .exr .hdr"):
		return 0
	case has(path, Videos+" .mts .m2ts .m2t .vob .asf .3gp .3g2 .mxf .m2v .mpv .ogv .rm .rmvb .f4v .divx .h264 .264 .h265 .265 .hevc"):
		return 1
	case has(path, Audios+" .m3u .m3u8 .aif .aiff .ape .alac .amr .ac3 .dts .mka .mp2 .weba .cue .m4b .oga .au .snd .wv .tta .dsf .dff"):
		return 2
	default:
		return -1
	}
}

func isFormatCleanup(it Item) bool { return it.Action == "recycle" || it.Action == "delete" }
