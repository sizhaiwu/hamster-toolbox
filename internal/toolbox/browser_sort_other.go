//go:build !windows

package toolbox

func BrowserNameLess(a, b string) bool { return naturalLess(a, b) }
