//go:build !windows

package main

func platformAutostartStore() autostartStore                             { return nil }
func autostartCommand(program, data, defaultData string) (string, error) { return "", nil }
func autostartCommandsEqual(a, b string) bool                            { return a != "" && a == b }
