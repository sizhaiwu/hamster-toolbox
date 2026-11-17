package toolbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// Some Windows FreeType builds cannot open a Unicode absolute font path.
// An ASCII font filename relative to a Unicode working directory works because
// Windows sets that working directory with its Unicode process API.
func runWithFont(ctx context.Context, exe string, args ...string) ([]byte, error) {
	for i := 0; i+1 < len(args); i++ {
		if args[i] != "-font" {
			continue
		}
		font, e := filepath.Abs(filepath.FromSlash(args[i+1]))
		if e != nil {
			return nil, e
		}
		dir, name := filepath.Dir(font), filepath.Base(font)
		ascii := true
		for _, r := range name {
			if r > 127 {
				ascii = false
				break
			}
		}
		if !ascii {
			dir, e = os.MkdirTemp("", "hamster-font-")
			if e != nil {
				return nil, e
			}
			defer os.RemoveAll(dir)
			name = "font" + strings.ToLower(filepath.Ext(font))
			if e = publishContext(ctx, font, filepath.Join(dir, name)); e != nil {
				return nil, e
			}
		}
		copied := append([]string{}, args...)
		copied[i+1] = name
		return runInDirectory(ctx, dir, exe, copied...)
	}
	return run(ctx, exe, args...)
}
