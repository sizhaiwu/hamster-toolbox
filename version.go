package main

import "runtime"

import "github.com/sizhaiwu/hamster-toolbox/internal/toolbox"

// Release scripts set these with -ldflags; local builds remain visibly marked.
var appVersion = "dev"
var buildTime = "unknown"
var toolsManifestSHA256 = ""

func appInfo() map[string]string {
	return map[string]string{"Name": "仓鼠工具箱", "Version": appVersion, "BuildTime": buildTime, "Platform": runtime.GOOS + "/" + runtime.GOARCH, "BundleWarning": toolbox.PortableBundleWarning(toolsManifestSHA256)}
}
