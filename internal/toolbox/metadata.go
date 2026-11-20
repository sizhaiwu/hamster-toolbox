package toolbox

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
)

const metadataVideos = ".mp4 .mov .m4v .qt .mkv .webm .avi .ts .mts .m2ts .m2t .mpg .mpeg .mpe .wmv .asf .flv"
const metadataExifVideos = ".mp4 .mov .m4v .qt"

type metadataStream struct {
	Type                    string `json:"codec_type"`
	Codec                   string `json:"codec_name"`
	Width, Height, Channels int
	SampleRate              string `json:"sample_rate"`
	Tags                    map[string]string
}
type metadataMedia struct {
	Streams  []metadataStream
	Format   struct{ Duration string }
	Chapters []struct{ Tags map[string]string }
}

func probeMetadataMedia(ctx context.Context, exe, path string) (metadataMedia, error) {
	var m metadataMedia
	b, e := run(ctx, exe, "-v", "error", "-show_streams", "-show_format", "-show_chapters", "-of", "json", path)
	if e != nil {
		return m, e
	}
	e = json.Unmarshal(b, &m)
	return m, e
}

func cleanMetadata(ctx context.Context, c Config, source, target string) error {
	if !has(source, metadataVideos) || has(source, metadataExifVideos) {
		var before metadataMedia
		var e error
		video := has(source, metadataVideos)
		if video {
			before, e = probeMetadataMedia(ctx, c.FFprobe, source)
			if e != nil {
				return e
			}
		}
		if e := publishContext(ctx, source, target); e != nil {
			return e
		}
		if _, e = run(ctx, c.Exiftool, "-all=", "-overwrite_original", target); e != nil {
			return e
		}
		if video {
			after, e := probeMetadataMedia(ctx, c.FFprobe, target)
			if e != nil {
				return e
			}
			return validateMetadataCopy(before, after)
		}
		return nil
	}
	before, e := probeMetadataMedia(ctx, c.FFprobe, source)
	if e != nil {
		return e
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-xerror", "-n", "-i", source, "-map", "0", "-copy_unknown", "-c", "copy", "-map_metadata", "-1", "-map_metadata:s", "-1", "-map_chapters", "0", "-fflags", "+bitexact", "-metadata", "encoder=", "-metadata", "service_name=", "-metadata", "service_provider="}
	for i, stream := range before.Streams {
		// Restore only presentation tags. Descriptive titles, comments and authors
		// are deliberately not copied back with the streams.
		for _, key := range []string{"language", "rotate", "stereo_mode", "alpha_mode", "filename", "mimetype"} {
			if value := stream.Tags[key]; value != "" {
				args = append(args, "-metadata:s:"+strconv.Itoa(i), key+"="+value)
			}
		}
	}
	for i := range before.Chapters {
		args = append(args, "-map_metadata:c:"+strconv.Itoa(i), "-1")
	}
	args = append(args, target)
	if _, e = run(ctx, c.FFmpeg, args...); e != nil {
		return fmt.Errorf("原容器无法完成流复制清理，未转码且保留原件：%w", e)
	}
	after, e := probeMetadataMedia(ctx, c.FFprobe, target)
	if e != nil {
		return e
	}
	return validateMetadataCopy(before, after)
}

func validateMetadataCopy(before, after metadataMedia) error {
	if len(before.Streams) != len(after.Streams) || len(before.Chapters) != len(after.Chapters) {
		return fmt.Errorf("清理后轨道或章节数量变化，保留原件")
	}
	for i, a := range before.Streams {
		b := after.Streams[i]
		if a.Type != b.Type || a.Codec != b.Codec || a.Width != b.Width || a.Height != b.Height || a.Channels != b.Channels || a.SampleRate != b.SampleRate {
			return fmt.Errorf("清理后轨道参数变化，保留原件")
		}
	}
	d1, _ := strconv.ParseFloat(before.Format.Duration, 64)
	d2, _ := strconv.ParseFloat(after.Format.Duration, 64)
	if d1 > 0 && (d2 <= 0 || math.Abs(d1-d2) > math.Max(1, d1*0.005)) {
		return fmt.Errorf("清理后时长不符合预期，保留原件")
	}
	return nil
}
