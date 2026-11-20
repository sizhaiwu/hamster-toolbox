//go:build !windows

package main

import "os"

func addNativeFileProperties(*os.File, map[string]any) {}
