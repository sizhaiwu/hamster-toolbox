# Windows、Linux 与 macOS

[文档目录](README.md)

[返回首页](../README.md) · [资源管理器](file-browser.md)

当前源码包含 Windows、Linux/macOS 的平台实现。现有发布包与其中的 `tools` 是 Windows 组件，不能直接复制到其他系统运行。Linux/macOS 当前通过源码构建，并安装对应系统和 CPU 架构的依赖；不要使用 Windows 的 `.exe` 组件。

## 构建与启动

安装 Go 1.23.7 或更新版本，在仓库根目录运行：

```sh
sh build.sh
./work/build/hamster-toolbox
```

程序使用浏览器作为界面。macOS 通过系统 `open` 打开浏览器，Linux 尝试 `xdg-open`；没有桌面环境时使用 `-no-open`，并保留终端打印的本机入口。

```sh
./work/build/hamster-toolbox -no-open -port 8787 -data /path/to/private-toolbox-data
./work/build/hamster-toolbox cli rename --root /path/to/media --prefix '整理-'
```

CLI 默认预览，确认后添加 `--execute` 执行。页面复制命令按服务所在系统生成：Windows 为 PowerShell，Linux/macOS 为 sh/bash/zsh。自定义数据目录要放在不共享给访客的位置；不要通过穿透开放本机管理端口。

## 功能与依赖

| 功能 | Linux/macOS 情况 |
| --- | --- |
| 目录统计、批量改名、编号 | Go 核心逻辑可用，无额外依赖；文件名同时遵守跨平台安全规则及本机字节长度限制 |
| 图片、视频、媒体信息、元数据、PDF、视频拼图 | 安装当前系统的 FFmpeg/FFprobe、ImageMagick 7、ExifTool；中文图册/拼图还需要中文字体。页面下载说明与自动查找按系统调整 |
| 7-Zip 打包、解压、分卷与校验 | 使用本机 `7zz`、`7z` 或 `7za`；实际格式支持取决于所安装的版本，RAR 需要包含对应解码支持 |
| 资源管理器、内网共享与远程访问 | 可用。Windows 显示盘符；Linux/macOS 显示文件系统、用户目录及常见挂载位置，也可输入 `/mnt/...` 或 `/Volumes/...` |
| mpv 原生图片/视频窗口 | 安装本机 mpv 或指定可执行文件；独立窗口和硬解取决于桌面环境、解码器与驱动，共享访客不调用主机播放器 |
| 回收站/废纸篓 | macOS 使用系统 Finder；Linux 使用 `gio trash` 或 `trash-put`。缺少程序、没有桌面会话或磁盘不支持时报告失败并保留源文件，不自动改为永久删除 |
| 常用密码列表 | 当前只支持 Windows 账户加密；Linux/macOS 隐藏密码库入口，打包和解压仍可临时输入密码或通过 CLI 的密码环境变量使用 |
| 外部去重软件入口 | 可选择当前系统的 GUI 可执行文件；第三方软件是否提供对应版本、依赖是否齐全，以其发布页为准，需要桌面环境 |
| 显卡视频编码 | 取决于当前 FFmpeg、设备与驱动；不可用时使用已有 CPU 回退。不能保证每个平台都有相同的硬件编码器 |
| 安装器、系统托盘与登录自启开关 | 当前仅提供 Windows 实现；Linux/macOS 使用普通服务模式，自行配置系统启动方式 |

原生文件操作在 Linux/macOS 使用系统的“不覆盖目标”重命名接口。符号链接、系统目录、工具箱程序和数据目录保护仍然生效。共享磁盘/文件系统根目录时，受保护内容不会列出或开放直接访问；批量处理工具继续禁止把根目录作为输入。

## 验证范围

Windows Go/UI 回归、实际 FFmpeg 视频缩略图取帧，以及 Linux 子系统中的 Go 测试已纳入本地验证。macOS Intel/Apple Silicon 可交叉编译；当前没有 macOS 真机，不能据此宣称 Finder 废纸篓、浏览器启动和第三方组件已在真机验证。

CI 配置覆盖 Windows、Linux、macOS，运行结果以实际 Actions 记录为准。跨平台编译与测试不等于包含全部依赖的跨平台发布包；当前完整便携打包脚本面向 Windows x64。
