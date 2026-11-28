//go:build !windows

package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
)

func prepareDesktop(data string, tray, noOpen bool) (func(), bool, error) {
	if tray {
		return func() {}, false, errors.New("系统托盘启动目前仅支持 Windows；请使用普通服务启动方式")
	}
	return func() {}, false, nil
}
func desktopError(text string) { fmt.Fprintln(os.Stderr, text) }
func detachDesktopConsole()    {}
func runDesktopTray(url string, s *server, service *http.Server, listener net.Listener) error {
	return errors.New("此平台暂不支持系统托盘")
}
