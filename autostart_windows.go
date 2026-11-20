//go:build windows

package main

import (
	"errors"
	"path/filepath"
	"strings"
	"unicode/utf16"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const autostartRunKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const autostartValue = "HamsterToolbox"

type registryAutostartStore struct {
	root       registry.Key
	path, name string
}

func platformAutostartStore() autostartStore {
	return registryAutostartStore{registry.CURRENT_USER, autostartRunKey, autostartValue}
}
func (s registryAutostartStore) Read() (string, error) {
	key, err := registry.OpenKey(s.root, s.path, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer key.Close()
	value, _, err := key.GetStringValue(s.name)
	if errors.Is(err, registry.ErrNotExist) {
		return "", nil
	}
	return value, err
}
func (s registryAutostartStore) Write(value string) error {
	key, _, err := registry.CreateKey(s.root, s.path, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	return key.SetStringValue(s.name, value)
}
func (s registryAutostartStore) Delete() error {
	key, err := registry.OpenKey(s.root, s.path, registry.SET_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer key.Close()
	err = key.DeleteValue(s.name)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	return err
}
func autostartCommand(program, data, defaultData string) (string, error) {
	p, err := filepath.Abs(program)
	if err != nil {
		return "", err
	}
	if strings.ContainsAny(p, "\"\x00") {
		return "", errors.New("程序路径无效")
	}
	command := `"` + p + `" -tray -no-open`
	if data != "" {
		data, err = filepath.Abs(data)
		if err != nil {
			return "", err
		}
		if !sameHistoryPath(data, defaultData) {
			command += " -data " + windows.EscapeArg(data)
		}
	}
	if len(utf16.Encode([]rune(command))) > 260 {
		return "", errors.New("自启命令过长，请缩短程序或数据目录路径")
	}
	return command, nil
}
func autostartCommandsEqual(a, b string) bool {
	aa, e := windows.DecomposeCommandLine(a)
	bb, f := windows.DecomposeCommandLine(b)
	if e != nil || f != nil || len(aa) == 0 || len(aa) != len(bb) {
		return false
	}
	for i := range aa {
		if i == 0 || i > 0 && aa[i-1] == "-data" {
			if !sameHistoryPath(aa[i], bb[i]) {
				return false
			}
		} else if aa[i] != bb[i] {
			return false
		}
	}
	return true
}
