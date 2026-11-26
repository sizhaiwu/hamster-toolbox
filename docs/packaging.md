# 打包与发布

[文档目录](README.md) · [安装与更新](installation.md) · [发布检查](release-readiness.md)

打包脚本只生成本地分发文件，不提交源码、不推送 Git、不创建 GitHub Release。版本默认读取 `VERSION`；当前版本说明见 [v0.2.7](releases/0.2.7.md)。正式操作前以维护者明确授权为准。

## 构建环境

维护者需要 Windows x64、Go 1.26.11 或更新版本、Node.js 20.19+ / npm，以及 PowerShell 和系统 tar。首次运行 `npm ci --ignore-scripts` 安装开发依赖。若修改过 `scripts/notes-rich-editor.js`，先运行 `npm run build:notes-editor`，将编辑器产物和许可声明一并更新。安装包另需 Inno Setup 6.5+ 或 7；可通过 `-ISCC` 指定编译器。

接收者无需 Go、Python、Node.js、Inno Setup 或配置 PATH。源码 ZIP 与不含组件的更新 ZIP 不能作为完整版单独使用。

打包前等待任务结束，从托盘退出本目录的工具箱；控制台模式则退出控制台，并正常关闭正在使用组件的 mpv 等程序。脚本不会强制结束用户进程，组件被占用时应先处理占用再重试。

## 检查与打包命令

只检查源码、界面和分发脚本，不生成包：

```powershell
.\check-release.ps1 -SourceOnly
```

核对某个完整运行目录的组件、哈希和十一项批处理自测：

```powershell
.\check-release.ps1 -RuntimeDirectory 'D:\toolbox-verification'
```

省略参数时检查当前项目运行目录。新旧组件清单不符时会失败，不会自动替换正在使用的组件。单独源码检查不等于最终发行包已经通过验收。

生成完整便携 ZIP 和程序更新 ZIP：

```powershell
.\package.ps1 -Version 0.2.7
```

同时生成安装包：

```powershell
.\package.ps1 -Version 0.2.7 -Installer
# 编译器不在默认位置时
.\package.ps1 -Version 0.2.7 -Installer -ISCC 'D:\build-tools\Inno Setup\ISCC.exe'
```

也可双击 `package.cmd` / `package-installer.cmd`，默认读取 `VERSION`。当前 `-Installer` 会在 ZIP 之外增加安装 EXE，不是仅安装器模式；`-UpdateOnly` 只省略完整 ZIP，更新包仍不含 `tools`。

测试包可显式使用 `-Version 0.2.7-dev`。正式发行须从包含全部修复的源码重新构建，使用正式版本号，不重命名旧测试包冒充正式包。

## 组件与构建流程

1. 根据 `tools-manifest.json` 准备固定版本的 FFmpeg/FFprobe、ImageMagick、ExifTool、7-Zip、中文字体、mpv 及所需运行库，核对来源与 SHA256。可复用已经核验的 `tools` 和下载缓存。
2. 强制重新运行 Go 测试（`-count=1`，不复用测试结果缓存）、`npm run test:ui` 全部界面测试和分发脚本回归；缺少 Node/npm/jsdom、许可声明或测试失败会停止。静态检查由发布前的 `check-release.ps1` 覆盖。
3. 构建带版本与构建时间的控制台 EXE 和无控制台托盘 EXE，按白名单复制组件与说明。组件清单 `tools/bundle-files.json` 中每项均须通过哈希、路径和普通文件校验，拒绝链接、越界及重复条目。
4. 在中文含空格目录、中文配置/临时路径、空配置与仅系统 PATH 下做组件检查和十一项批处理自测。mpv 的版本探测不等于所有显卡已实测硬解，原生播放应另做样本验收。
5. 生成发行文件、`release-info.json` 和每份附件对应的 SHA256，然后更新项目目录下的启动文件并保留旧 EXE。验收目录更名遇到短暂文件占用时会有限重试；持续失败则停止，不强行结束其他进程。程序、组件与用户资源不混合打包。

更新组件时修改清单中的固定版本、下载地址、经核对的 SHA256，并同步 [组件说明](dependencies.md)和第三方许可。组件变化时应向用户提供本版完整包，单独更新程序不能更新旧 `tools`。

## 分发文件

结果位于项目 `dist` 目录，文件名包含版本、平台和构建标识：

| 文件 | 内容与用途 |
| --- | --- |
| `hamster-toolbox-版本-windows-x64-标识-setup.exe` | 完整安装包，推荐普通用户；安装后创建快捷方式，可选登录自启 |
| `hamster-toolbox-版本-windows-x64-标识.zip` | 完整便携包，包含两个 EXE、组件、字体、版本信息与许可证 |
| `hamster-toolbox-版本-windows-x64-标识-update.zip` | 程序和说明更新包，不含组件；仅用于已有匹配组件的完整目录 |
| 每份附件的 `.sha256` | 校验文件是否传输完整，不代替发布者签名 |

本版从 v0.2.1 或更早版本升级时优先使用安装包或完整 ZIP。安装版用户优先运行新安装器；便携完整包解压到新目录后使用，避免混用 DLL。更新前结束任务、备份私人数据，详见 [安装说明](installation.md)及随包 `UPDATE.txt`。

白名单包含程序、`start-toolbox.cmd`、组件清单、版本/构建信息、快速说明、MIT、Go 依赖及第三方许可。安装器另带 Inno Setup 许可。不会加入个人设置、密码库、历史、日志、缩略图缓存、源码、测试素材、`.git`、`data`、`work` 或旧版 EXE。Windows 个人数据默认在当前用户配置目录的 `hamster-toolbox` 下，与程序目录分开。

## 最终包验收

对真正准备上传的附件核对 SHA256；完整 ZIP 重新解压到新目录、最终安装器在测试机或虚拟机安装。运行以下命令并从快捷方式打开界面：

```powershell
.\hamster-toolbox.exe -version
.\hamster-toolbox.exe -check-tools
.\hamster-toolbox.exe -self-test
```

再用仓库合成素材检查共享首次登录、媒体预览、压缩/缩略图、分卷解压及实时进度、开关自启和升级配置保留。原始用户资源不作为可公开分发的验收素材。生成文件通过检查后再交付，不能用旧测试安装包或开发目录代替。

## GitHub 发布

维护者授权后再提交并推送所有应公开的源码、网页、脚本、文档、测试和品牌素材；组件 `tools`、安装包 `dist`、本地验收 `work` 及私人数据不进入 Git。

正式标签与 `VERSION` 一致。GitHub Release 的附件包括完整 ZIP、更新 ZIP、安装 EXE（本版构建时）及各自 SHA256；下载说明区分三种用途，并附本版更新与限制。发布说明以[对应版本说明](releases/0.2.7.md)为基础。CI 只运行检查，不会自动提交、推送或上传 Release。

当前程序和安装器未签名；如另行配置签名，应在签名后重新生成 SHA256。最终附件和新测试机验收状态记录到[发布检查](release-readiness.md)。
