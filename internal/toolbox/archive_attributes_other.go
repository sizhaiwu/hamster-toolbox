//go:build !windows

package toolbox

func archiveCanLink(source string) bool { return true }

func copyArchiveAttributes(source, target string) error { return nil }
func setArchiveMarkerAttributes(path string) error      { return nil }
func clearArchiveMarkerAttributes(path string)          {}
