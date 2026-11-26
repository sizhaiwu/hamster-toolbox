package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/sizhaiwu/hamster-toolbox/internal/toolbox"
)

func browserAmbiguousSource(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".ts" || ext == ".mts"
}

// Only ambiguous TypeScript/transport-stream suffixes need content inspection.
// ReadAt leaves playback/text readers untouched and bounds directory-list I/O;
// listing a code project never launches FFprobe or a decoder.
func browserFileKind(f *os.File, st os.FileInfo) string {
	kind := browserKind(st.Name())
	if !st.Mode().IsRegular() || !browserAmbiguousSource(st.Name()) {
		return kind
	}
	var sample [16 << 10]byte
	n, err := f.ReadAt(sample[:], 0)
	if err != nil && err != io.EOF {
		return "file"
	}
	data := sample[:n]
	if browserSourceSample(data) {
		return "text"
	}
	// Standard TS, timestamp-prefixed M2TS, and parity-protected TS packets.
	// Allow a small leading header or damaged initial packet, but require three
	// consecutive sync bytes with valid adaptation/payload flags.
	for _, packetSize := range []int{188, 192, 204} {
		for start := 0; start < 4096 && start+2*packetSize+4 <= len(data); start++ {
			valid := true
			for packet := 0; packet < 3; packet++ {
				at := start + packet*packetSize
				if data[at] != 0x47 || data[at+3]&0x30 == 0 {
					valid = false
					break
				}
			}
			if valid {
				return "video"
			}
		}
	}
	return "file"
}

func browserSourceSample(data []byte) bool {
	if bytes.HasPrefix(data, []byte{0xff, 0xfe}) || bytes.HasPrefix(data, []byte{0xfe, 0xff}) {
		// A source file may use BOM-marked UTF-16. The editor validates the full
		// encoding before allowing edits; zero bytes here must not imply video.
		return true
	}
	for _, value := range data {
		if value < 32 && value != '\t' && value != '\n' && value != '\r' && value != '\f' || value == 127 {
			return false
		}
	}
	return true
}

func browserPathKind(path string, st os.FileInfo) string {
	if !st.Mode().IsRegular() || !browserAmbiguousSource(st.Name()) {
		return browserKind(st.Name())
	}
	f, err := toolbox.BrowseOpen(path)
	if err != nil {
		return "file"
	}
	defer f.Close()
	return browserFileKind(f, st)
}
