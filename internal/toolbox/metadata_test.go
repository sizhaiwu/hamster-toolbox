package toolbox

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestMetadataFileSelection(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"image.jpg", "video.mp4", "video.mov", "video.m4v", "unsupported.mkv", "text.txt"} {
		file(t, filepath.Join(root, name), []byte("test"))
	}
	c := DefaultConfig()
	c.Tool, c.Root = "exif", root
	c.Exiftool = filepath.Join(t.TempDir(), "exiftool.exe")
	file(t, c.Exiftool, []byte("test"))
	c.FFmpeg, c.FFprobe = c.Exiftool, c.Exiftool
	for _, tc := range []struct {
		mode             string
		total, conflicts int
	}{{"", 1, 0}, {"images", 1, 0}, {"videos", 4, 0}, {"all", 5, 0}} {
		c.MetadataTypes = tc.mode
		p, e := BuildPlan(context.Background(), c, quiet)
		if e != nil || len(p.Items) != tc.total {
			t.Fatal(tc, p, e)
		}
		conflicts := 0
		for _, it := range p.Items {
			if it.Conflict != "" {
				conflicts++
			}
		}
		if conflicts != tc.conflicts {
			t.Fatal(tc, p.Items)
		}
	}
	c.MetadataTypes = "invalid"
	if _, e := BuildPlan(context.Background(), c, quiet); e == nil {
		t.Fatal("invalid selection allowed")
	}
}

func TestRemuxMetadataFormatsIntegration(t *testing.T) {
	if os.Getenv("TOOLBOX_INTEGRATION") != "1" {
		t.Skip("set TOOLBOX_INTEGRATION=1")
	}
	ctx := context.Background()
	base := integrationConfig(t)
	for _, tc := range []struct{ extension, video, audio string }{{"ts", "libx264", "aac"}, {"mts", "libx264", "aac"}, {"m2ts", "libx264", "aac"}, {"avi", "mpeg4", "pcm_s16le"}, {"mkv", "libx264", "aac"}, {"webm", "libvpx", "libopus"}, {"mpg", "mpeg2video", "mp2"}, {"wmv", "wmv2", "wmav2"}, {"flv", "flv", "libmp3lame"}} {
		t.Run(tc.extension, func(t *testing.T) {
			c := base
			c.Tool, c.Root, c.MetadataTypes = "exif", t.TempDir(), "videos"
			source := filepath.Join(c.Root, "metadata."+tc.extension)
			args := []string{"-v", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-t", "1", "-c:v", tc.video, "-c:a", tc.audio, "-metadata", "title=Sample promotion", "-metadata", "comment=Sample promotion", "-metadata", "artist=Sample promotion", "-metadata", "service_name=Sample promotion", "-metadata", "service_provider=Sample promotion", "-metadata:s:v:0", "title=Sample promotion", source}
			if _, e := run(ctx, c.FFmpeg, args...); e != nil {
				t.Fatal(e)
			}
			if tc.extension == "mkv" {
				staged := filepath.Join(t.TempDir(), "multi.mkv")
				chapters := filepath.Join(t.TempDir(), "chapters.txt")
				subtitles := filepath.Join(t.TempDir(), "captions.srt")
				attachment := filepath.Join(t.TempDir(), "notes.txt")
				file(t, attachment, []byte("Sample attachment"))
				file(t, subtitles, []byte("1\n00:00:00,000 --> 00:00:00,900\nSample subtitle\n"))
				file(t, chapters, []byte(";FFMETADATA1\n[CHAPTER]\nTIMEBASE=1/1000\nSTART=0\nEND=900\ntitle=Sample promotion\n"))
				if _, e := run(ctx, c.FFmpeg, "-v", "error", "-i", source, "-i", chapters, "-i", subtitles, "-map", "0", "-map", "0:a:0", "-map", "2:s:0", "-map_chapters", "1", "-c", "copy", "-metadata:s:a:0", "language=eng", "-metadata:s:a:1", "language=jpn", "-attach", attachment, "-metadata:s:t:0", "filename=notes.txt", "-metadata:s:t:0", "mimetype=text/plain", staged); e != nil {
					t.Fatal(e)
				}
				b, e := os.ReadFile(staged)
				if e != nil {
					t.Fatal(e)
				}
				file(t, source, b)
			}
			original, e := os.ReadFile(source)
			if e != nil {
				t.Fatal(e)
			}
			p, e := BuildPlan(ctx, c, quiet)
			if e != nil || len(p.Items) != 1 {
				t.Fatal(p, e)
			}
			logs := []string{}
			r := Execute(ctx, p, func(s string) { logs = append(logs, s) }, progress)
			if r.Success != 1 || r.Failed != 0 {
				t.Fatal(r, logs)
			}
			target := p.Items[0].Target
			meta, e := run(ctx, c.FFprobe, "-v", "error", "-show_format", "-show_streams", "-show_programs", "-show_chapters", "-of", "json", target)
			if e != nil || strings.Contains(string(meta), "Sample promotion") {
				t.Fatal("promotional tag remains", string(meta), e)
			}
			var hashes [][]string
			for _, path := range []string{source, target} {
				decoded, e := run(ctx, c.FFmpeg, "-v", "error", "-xerror", "-i", path, "-map", "0:v?", "-map", "0:a?", "-f", "framemd5", "-")
				if e != nil {
					t.Fatal(e)
				}
				frames := []string{}
				for _, line := range strings.Split(string(decoded), "\n") {
					if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
						continue
					}
					fields := strings.Split(line, ",")
					frames = append(frames, strings.TrimSpace(fields[0])+":"+strings.TrimSpace(fields[len(fields)-1]))
				}
				hashes = append(hashes, frames)
			}
			// Packet interleaving may change after remux; compare each stream's decoded
			// sequence independently, without container timestamps.
			group := func(lines []string) map[string][]string {
				m := map[string][]string{}
				for _, line := range lines {
					key, value, _ := strings.Cut(line, ":")
					m[key] = append(m[key], value)
				}
				return m
			}
			if len(hashes[0]) == 0 || !reflect.DeepEqual(group(hashes[0]), group(hashes[1])) {
				t.Fatal("decoded audio/video changed", len(hashes[0]), len(hashes[1]))
			}
			if tc.extension == "mkv" {
				m, e := probeMetadataMedia(ctx, c.FFprobe, target)
				if e != nil || len(m.Chapters) != 1 || len(m.Streams) != 5 || m.Streams[3].Type != "subtitle" || m.Streams[4].Type != "attachment" || m.Streams[4].Tags["filename"] != "notes.txt" || m.Streams[4].Tags["mimetype"] != "text/plain" || m.Streams[1].Tags["language"] != "eng" || m.Streams[2].Tags["language"] != "jpn" {
					t.Fatal("chapter or language lost", m, e)
				}
			}
			after, e := os.ReadFile(source)
			if e != nil || !reflect.DeepEqual(original, after) {
				t.Fatal("source changed", e)
			}
		})
	}
}

func TestVideoMetadataCleanupIntegration(t *testing.T) {
	if os.Getenv("TOOLBOX_INTEGRATION") != "1" {
		t.Skip("set TOOLBOX_INTEGRATION=1")
	}
	ctx := context.Background()
	c := integrationConfig(t)
	c.Tool, c.Root, c.MetadataTypes = "exif", t.TempDir(), "videos"
	original := filepath.Join(t.TempDir(), "input.mp4")
	if _, e := run(ctx, c.FFmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-t", "1", "-c:v", "libx264", "-c:a", "aac", original); e != nil {
		t.Fatal(e)
	}
	src := filepath.Join(c.Root, "中文 预览.mp4")
	if _, e := run(ctx, c.FFmpeg, "-v", "error", "-i", original, "-c", "copy", "-metadata", "title=Sample title", "-metadata", "comment=Sample comment", "-metadata", "artist=Sample author", "-metadata", "description=ad.example", "-metadata:s:v:0", "rotate=90", src); e != nil {
		t.Fatal(e)
	}
	sourceBytes, e := os.ReadFile(src)
	if e != nil {
		t.Fatal(e)
	}
	p, e := BuildPlan(ctx, c, quiet)
	if e != nil || len(p.Items) != 1 {
		t.Fatal(p, e)
	}
	result := Execute(ctx, p, quiet, progress)
	if result.Success != 1 || result.Failed != 0 {
		t.Fatal(result)
	}
	target := p.Items[0].Target
	b, e := run(ctx, c.FFprobe, "-v", "error", "-show_entries", "format_tags=title,comment,artist,description", "-of", "json", target)
	if e != nil {
		t.Fatal(e)
	}
	var metadata struct {
		Format struct{ Tags map[string]string }
	}
	if e = json.Unmarshal(b, &metadata); e != nil || len(metadata.Format.Tags) != 0 {
		t.Fatal(string(b), e)
	}
	var payloads, structures [][]byte
	for _, path := range []string{src, target} {
		packets, e := run(ctx, c.FFprobe, "-v", "error", "-show_entries", "packet=stream_index,data_hash", "-show_data_hash", "sha256", "-of", "json", path)
		if e != nil {
			t.Fatal(e)
		}
		structure, e := run(ctx, c.FFprobe, "-v", "error", "-show_entries", "format=duration:stream=codec_name,width,height,pix_fmt,avg_frame_rate,sample_aspect_ratio,color_space,color_transfer,color_primaries:stream_side_data=rotation", "-of", "json", path)
		if e != nil {
			t.Fatal(e)
		}
		payloads = append(payloads, packets)
		structures = append(structures, structure)
	}
	if !reflect.DeepEqual(payloads[0], payloads[1]) {
		t.Fatal("encoded audio/video packets changed")
	}
	if !reflect.DeepEqual(structures[0], structures[1]) {
		t.Fatal("playback structure changed", string(structures[0]), string(structures[1]))
	}
	after, e := os.ReadFile(src)
	if e != nil || !reflect.DeepEqual(sourceBytes, after) {
		t.Fatal("source was modified", e)
	}
	result = Execute(ctx, p, quiet, progress)
	if result.Failed != 1 || result.Success != 0 {
		t.Fatal("existing output not protected", result)
	}
}
