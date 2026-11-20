package toolbox

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

var extractNumericSplitPattern = regexp.MustCompile(`\.(\d{3,})$`)

func unnamedExtractSplit(path string) bool {
	return extractNumericSplitPattern.MatchString(filepath.Base(path)) && !extractSplitPattern.MatchString(filepath.Base(path))
}

// Inspect only the first eight bytes of an extension-less split archive. Do
// not ask 7-Zip to probe every ordinary file or concatenate large volumes.
func extractSplitType(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	var header [8]byte
	n, err := io.ReadFull(f, header[:])
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return "", err
	}
	b := header[:n]
	switch {
	case bytes.HasPrefix(b, []byte{'7', 'z', 0xbc, 0xaf, 0x27, 0x1c}):
		return "7z", nil
	case bytes.HasPrefix(b, []byte{'P', 'K', 3, 4}), bytes.HasPrefix(b, []byte{'P', 'K', 5, 6}), bytes.HasPrefix(b, []byte{'P', 'K', 7, 8}):
		return "zip", nil
	case bytes.HasPrefix(b, []byte{'R', 'a', 'r', '!', 0x1a, 7, 0}), bytes.HasPrefix(b, []byte{'R', 'a', 'r', '!', 0x1a, 7, 1, 0}):
		return "rar", nil
	}
	return "", nil
}

func extractTypeOptions(source string) ([]string, error) {
	if !unnamedExtractSplit(source) {
		return nil, nil
	}
	format, err := extractSplitType(source)
	if err != nil {
		return nil, err
	}
	if format == "" {
		return nil, errors.New("无法识别此数字分卷的压缩格式，仅支持 7z、ZIP、RAR")
	}
	// Without the explicit inner type, 7-Zip may merely join a.001/a.002 into
	// one huge file called 'a' instead of listing/extracting the archive inside.
	return []string{"-t" + format + ".split"}, nil
}
