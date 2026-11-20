package main

// The same public branding is served by the toolbox, local explorer and guest
// login page. Keep this list explicit: it must never expose other embedded files.
func brandContentType(path string) string {
	switch path {
	case "hamster-logo.png", "favicon-32.png", "apple-touch-icon.png":
		return "image/png"
	case "favicon.ico":
		return "image/x-icon"
	default:
		return ""
	}
}
