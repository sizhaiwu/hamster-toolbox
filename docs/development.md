# 开发与贡献

[文档目录](README.md) · [测试与验收](testing.md) · [平台支持](platforms.md)

## 环境

- Go：`go.mod` 声明 Go 1.26.0，工具链为 1.26.11；使用可满足该要求的 Go 环境。
- Node.js：用于界面回归和笔记富文本编辑器构建，要求见 `package.json`；CI 使用 Node 24。普通用户运行不需要 Node/npm。
- Windows 完整组件由 `prepare-tools.ps1` 准备；Linux/macOS 自行安装本机依赖。

后端为 Go HTTP 服务，前端为内嵌 HTML/CSS/JavaScript；仓库保留已构建的笔记富文本编辑器，普通 Go 构建无需再次打包前端。修改编辑器源码时需按下文重新构建并保留产物。Go 模块依赖见 `go.mod`/`go.sum`，包括平台系统接口、Windows 命名管道、Argon2id、Markdown 与本地二维码编码。私密笔记测试只使用临时目录和合成密码，不得将真实笔记、恢复码或备份提交到仓库。

## 获取源码和运行

```powershell
git clone https://github.com/sizhaiwu/hamster-toolbox.git
cd hamster-toolbox
go test ./...
go build -buildvcs=false -trimpath -ldflags="-s -w" -o hamster-toolbox.exe .
.\hamster-toolbox.exe
```

Windows 也可运行 `build.cmd`，Linux/macOS 使用 `sh build.sh`。源码构建不等于含依赖的便携包，发布流程见[打包说明](packaging.md)。Windows 媒体组件准备：

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\prepare-tools.ps1
```

脚本从固定清单下载、校验 SHA256 并复用缓存。源码树构建的 Go 程序默认显示 `dev`；脚本可能按 `VERSION` 注入开发标识，打包时注入正式版本和构建信息。

## 代码结构

| 位置 | 职责 |
| --- | --- |
| `main.go`、`preview.go`、`settings.go` | 服务启动、批处理接口、状态和设置 |
| `cli.go`、`commands.go` | CLI 参数、完整命令与底层命令导出 |
| `internal/toolbox` | 扫描、执行、媒体处理、路径防护、依赖与便携自测 |
| `file_browser*.go`、`file_sharing*.go` | 浏览、文件操作、媒体缓存、共享与来源权限 |
| `native_player*.go`、平台辅助文件 | mpv 启动与受限 IPC、系统应用和文件操作 |
| `web` | 工具箱、资源管理器、播放器与共享设置界面 |
| `tests` | DOM/模拟接口交互回归，`npm run test:ui` 为完整入口 |
| `samples`、`try-samples.cmd` | 合成素材、场景配置和体验副本 |
| `release`、`tools-manifest.json` | 随包说明、许可、固定组件来源和校验值 |
| `docs/brand`、`generate-brand.ps1`、`brand_windows_*.syso` | 原始品牌图、技术尺寸和 Windows 图标资源 |

## 修改与验证

共用视觉变量位于 `web/theme.css`；工具箱布局在 `web/toolbox-layout.css`，资源管理器布局在 `web/explorer-layout.css`，悬浮窗口样式与逻辑单独维护。修改后检查桌面、手机、弹窗、悬停和键盘焦点。

[测试指南](testing.md)集中列出基础、专项、真实媒体、硬件与便携验收命令。CI 配置在 `.github/workflows/ci.yml`，在 Windows/Linux/macOS 运行 Go、静态检查和 UI 测试，不自动发布。某次远程运行是否成功应查看实际 Actions 结果。

## 贡献和文档

功能变化同步[功能清单](features.md)、对应操作指南与输入输出示例；CLI 参数变化同步[参数手册](cli-reference.md)，组件变化同步清单和第三方说明。默认值、格式范围和保护规则以代码为准，不把目标功能写成已经支持。

公开截图只使用合成媒体。不要提交私人路径、日志、设置、密码库、下载缓存、第三方二进制文件或体验副本。问题反馈提供程序版本、复现步骤和脱敏日志。项目代码采用 MIT；第三方组件遵守各自许可。

## 笔记编辑器

前端开发使用 Node.js 20.19 或更高版本。修改 `scripts/notes-rich-editor.js` 后运行 `npm run build:notes-editor`，将本地打包结果 `web/notes-rich.js` 随源码保留；最终用户不需要 Node.js。编辑器依赖许可包含在 `release/THIRD-PARTY.txt`。
