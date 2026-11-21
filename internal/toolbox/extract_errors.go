package toolbox

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
)

// Retain both diagnostic streams: 7-Zip reports password/header failures before
// it can produce the file list, and different formats use different streams.
func runExtractCommand(ctx context.Context, c Config, source string, args ...string) ([]byte, error) {
	return runExtractCommandProgress(ctx, c, source, nil, args...)
}
func runExtractCommandProgress(ctx context.Context, c Config, source string, notify func(int), args ...string) ([]byte, error) {
	_, compression := compressedTAR(source)
	if compression != "" {
		return runCompressedTAR(ctx, c, source, compression, notify, args...)
	}
	types, err := extractTypeOptions(source)
	if err != nil {
		return nil, err
	}
	if len(types) > 0 {
		args = append(types, args...)
	}
	cmd := exec.CommandContext(ctx, c.SevenZip, args...)
	hideWindow(cmd)
	cmd.Stdin = strings.NewReader("") // Never wait for an interactive password prompt.
	var stdout bytes.Buffer
	var stderr, progressOutput videoErrorBuffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if notify != nil {
		cmd.Stdout = io.MultiWriter(&progressOutput, &sevenZipProgressWriter{notify: notify})
	}
	err = cmd.Run()
	if err == nil {
		return stdout.Bytes(), nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		return nil, errors.New("无法启动 7-Zip，请检查程序路径和同目录的 7z.dll")
	}
	return nil, classifyExtractError(c, source, stdout.String()+progressOutput.String()+"\n"+stderr.String())
}

func classifyExtractError(c Config, source, diagnostic string) error {
	// Avoid mistaking an archive filename for an error diagnostic.
	diagnostic = strings.ReplaceAll(diagnostic, source, "[archive]")
	text := strings.ToLower(diagnostic)
	has := func(values ...string) bool {
		for _, v := range values {
			if strings.Contains(text, v) {
				return true
			}
		}
		return false
	}
	if has("missing volume", "cannot find volume", "can not find volume") {
		return errors.New("缺少压缩分卷，请将所有分卷放在同一目录后重新扫描")
	}
	if has("wrong password", "password is incorrect", "password required", "password is required", "cannot open encrypted archive", "can not open encrypted archive") {
		if c.ExtractPassword == "" {
			return errors.New("压缩包已加密，请填写解压密码后重新扫描")
		}
		if has("data error in encrypted file", "crc failed in encrypted file") {
			return errors.New("密码校验失败或加密数据损坏，请先核对解压密码")
		}
		return errors.New("解压密码校验失败，请重新输入或选择其他常用密码")
	}
	if has("unexpected end of archive", "unexpected end of data") {
		if extractNumericSplitPattern.MatchString(source) || extractRARPartPattern.MatchString(source) {
			return errors.New("压缩分卷不完整或被截断，请补齐分卷后重新扫描")
		}
		return errors.New("压缩包不完整或已被截断，请重新下载完整文件")
	}
	if has("not enough space", "no space left", "disk full") {
		return errors.New("磁盘空间不足，请释放输出目录所在磁盘的空间")
	}
	if has("access is denied", "permission denied") {
		return errors.New("无法读写文件，请检查访问权限或文件是否被占用")
	}
	if has("unsupported method", "unsupported compression", "not implemented") {
		return errors.New("当前 7-Zip 不支持此压缩方式，请更换或更新 7-Zip")
	}
	if has("crc failed", "data error", "headers error") {
		return errors.New("压缩包校验失败，文件内容或目录信息可能已损坏")
	}
	if has("is not archive", "cannot open the file as archive", "can not open the file as archive") {
		return errors.New("文件不是可识别的压缩包，或压缩包文件头已损坏")
	}
	return errors.New("7-Zip 无法处理此压缩包，请确认文件完整且格式受支持")
}
