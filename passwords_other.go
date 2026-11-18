//go:build !windows

package main

import "errors"

func passwordCrypt(data []byte, encrypt bool) ([]byte, error) {
	return nil, errors.New("常用密码列表需要 Windows 账户加密支持")
}
