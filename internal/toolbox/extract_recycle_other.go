//go:build !windows

package toolbox

func recycleExtractFiles(paths []string) (recycleResult, error) {
	result := recycleResult{}
	for _, path := range paths {
		if e := browserRecycle([]string{path}); e != nil {
			return result, e
		}
		result.Count++
		result.Paths = append(result.Paths, path)
	}
	return result, nil
}
