# 开发与测试

[返回首页](../README.md)

## 环境

- Windows 10/11 64 位。
- Go 1.23 或更新版本。
- Node.js 22 或更新的 LTS 版本，用于可选的前端交互测试。

Go 后端仅使用标准库，前端为内嵌 HTML/CSS/JavaScript。Node.js 和 npm 仅用于开发测试，不是用户运行依赖。

供普通用户动手体验的合成素材和复制脚本见 [体验素材](../samples/README.md)。素材完整性与场景覆盖检查由 `go test . -run TestSampleAssetsAndScenarioCoverage` 运行；设置 `HAMSTER_CLI_TEST_EXE` 后，`go test . -run TestSample -count=1 -v` 会在临时副本中实际执行 11 项批处理，不启动去重软件。

## 获取源码

```powershell
git clone https://github.com/sizhaiwu/hamster-toolbox.git
cd hamster-toolbox
```

## 构建和运行

双击 `build.cmd`，或执行：

```powershell
go test ./...
go build -buildvcs=false -trimpath -ldflags="-s -w" -o hamster-toolbox.exe .
.\hamster-toolbox.exe
```

开发构建显示为 `dev`。完整发布构建及版本号注入由 `package.cmd` 完成，详见[打包文档](packaging.md)。

媒体功能需要外部组件，可先执行：

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\prepare-tools.ps1
```

脚本会下载清单中的固定版本，检查 SHA256，并整理到 `tools`，后续可复用下载缓存。

## 测试

基础检查不依赖媒体程序：

```powershell
go test ./...
go vet ./...
```

前端交互回归：

```powershell
npm ci --ignore-scripts
npm run test:ui
```

测试使用隔离 DOM 和模拟接口，覆盖工具切换、参数保存、依赖提示、预览、编号和冲突处理。

准备好 `tools` 后，运行真实媒体集成测试：

```powershell
$env:TOOLBOX_INTEGRATION = '1'
go test ./... -count=1
Remove-Item Env:TOOLBOX_INTEGRATION
```

集成测试优先使用仓库内准备的组件，也可使用 PATH 中的程序。所有样本由测试生成，读写范围是临时目录。

视频专项测试使用生成的样本，覆盖三种模式、故障回退、音频重试、10-bit、VFR 时间戳、旋转、奇数尺寸、AAC 数据不变以及 HDR 跳过。具有 NVIDIA 和 Intel 核显的开发机可额外启用实际硬件编码验证（普通 CI 不要求显卡）：

```powershell
$env:TOOLBOX_INTEGRATION = '1'
$env:TOOLBOX_HARDWARE_INTEGRATION = '1'
go test ./internal/toolbox -run TestVideo -v
Remove-Item Env:TOOLBOX_INTEGRATION
Remove-Item Env:TOOLBOX_HARDWARE_INTEGRATION
```

发布构建还提供随包验证命令：

```powershell
.\hamster-toolbox.exe -version
.\hamster-toolbox.exe -check-tools
.\hamster-toolbox.exe -self-test
```

## 代码结构

| 位置 | 内容 |
| --- | --- |
| `main.go`、`preview.go`、`settings.go` | 本地服务、预览接口和工具设置 |
| `internal/toolbox` | 扫描、改名、媒体处理及便携依赖逻辑 |
| `web` | 内嵌界面 |
| `tests` | 前端交互测试 |
| `samples`、`try-samples.cmd` | 合成体验素材、各工具练习布局和独立副本准备脚本 |
| `release` | 发布包中的快速说明和第三方说明 |
| `docs` | 使用、开发和发布文档 |
| `tools-manifest.json` | 外部组件版本、来源和 SHA256 |

`.github/workflows/ci.yml` 在 Windows 上运行 Go 和前端测试，不会发布程序或改动仓库。

## 贡献

提交修改前运行相关测试。增加处理能力时同步说明支持格式、输出位置和已有文件的处理方式；新增组件时同步维护下载清单与第三方说明。

问题反馈请提供版本、复现步骤及经过脱敏的日志。不要把个人媒体、完整路径、私人配置或第三方二进制文件提交到源码仓库。

资源打包的实际分卷、加密、解压、附加文件、取消清理和输出冲突测试：`TOOLBOX_INTEGRATION=1 go test ./internal/toolbox -run TestArchive -count=1`。Windows PowerShell 先设置 `$env:TOOLBOX_INTEGRATION="1"`。测试仅使用临时合成资源，不读取私人脚本或媒体。

解压集成测试：`TOOLBOX_INTEGRATION=1 go test ./internal/toolbox -run TestExtract -count=1`。覆盖重名规则、加密分卷、损坏包、路径安全与原生 RAR 解码；使用合成资源，不依赖个人压缩包。Windows 账户加密密码列表由 `go test . -run TestPasswordVault` 验证。
