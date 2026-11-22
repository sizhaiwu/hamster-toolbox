package main

import "bytes"

// Use the same settings dialogs in all three owner interfaces.
func sharedSettingsHTML(page []byte) []byte {
	fragment, err := assets.ReadFile("web/shared-settings.html")
	if err != nil {
		panic(err)
	}
	return bytes.Replace(page, []byte("</body>"), append(fragment, []byte("</body>")...), 1)
}
