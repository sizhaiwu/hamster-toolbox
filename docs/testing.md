# 开发测试

[文档目录](README.md) · [开发说明](development.md) · [构建分发包](packaging.md)

本页供贡献者使用，普通用户无需运行测试。

## 基础检查

安装符合 `go.mod` 要求的 Go 和 Node.js 20.19+，在仓库根目录运行：

```sh
npm ci --ignore-scripts
go test ./... -count=1
go vet ./...
npm run test:ui
```

Go 测试覆盖文件保护、媒体处理、笔记、共享、计划任务和桌面接口。界面测试使用模拟接口，检查交互、状态恢复、错误处理和请求隔离。

修改笔记或代码编辑器源码后，还应重新构建对应前端产物：

```sh
npm run build:notes-editor
npm run build:file-editor
```

## 外部组件和图形环境

外部媒体测试需要对应系统的 FFmpeg/FFprobe、ImageMagick、ExifTool、7-Zip 或 mpv。相关测试使用临时目录和演示素材，不要替换成私人文件。

Windows PowerShell 中启用原生 mpv 集成测试：

```powershell
$env:TOOLBOX_NATIVE_MEDIA_INTEGRATION='1'
go test . -run '^TestNativeMediaRealPlayer$' -count=1 -v -timeout 120s
```

该测试需要可用图形环境，检查真实播放、窗口控制、AB 重复和播放结束动作。Linux 需要桌面显示环境；不同显卡和驱动仍需分别检查。

## 运行目录自检

完整目录可使用以下命令检查组件和批处理流程：

```powershell
.\hamster-toolbox.exe -version
.\hamster-toolbox.exe -check-tools
.\hamster-toolbox.exe -self-test
```

仓库的 `check-release.ps1 -SourceOnly` 汇总源码、界面和分发脚本检查，不生成分发包。打包方法见[构建分发包](packaging.md)。
