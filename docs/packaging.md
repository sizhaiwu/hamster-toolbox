# 构建分发包

[文档目录](README.md) · [开发说明](development.md) · [安装与更新](installation.md)

本页供需要自行构建的贡献者使用。普通用户直接下载 [Releases](https://github.com/sizhaiwu/hamster-toolbox/releases) 中的安装包或完整便携包。

## 环境

- Windows x64、满足 `go.mod` 的 Go、Node.js 20.19+ / npm、PowerShell。
- 安装包还需要 [Inno Setup](https://jrsoftware.org/isdl.php) 6.5+ 或 7，可通过 `-ISCC` 指定编译器。
- 运行 `npm ci --ignore-scripts` 安装开发依赖。修改编辑器源码后，重新构建前端产物和许可证声明。

普通用户不需要安装这些构建工具。Linux/macOS 运行方法见[平台说明](platforms.md)。

## 构建命令

先在 `VERSION` 中填写要构建的版本号，再运行：

```powershell
# 完整便携 7z 与程序更新 ZIP
.\package.ps1

# 同时生成安装包
.\package.ps1 -Installer

# 额外提供完整 ZIP
.\package.ps1 -Installer -PortableZip

# 指定 Inno Setup 编译器
.\package.ps1 -Installer -ISCC 'D:\build-tools\Inno Setup\ISCC.exe'
```

也可双击 `package.cmd` 或 `package-installer.cmd`。`-Version` 可覆盖 `VERSION`，`-UpdateOnly` 省略完整便携包。结果保存在 `dist`，脚本不会自动提交、推送或上传 GitHub。

## 分发文件

| 文件 | 内容 |
| --- | --- |
| `*-setup.exe` | 完整安装包，支持快捷方式、卸载和可选登录自启 |
| 完整 `.7z`，可选完整 `.zip` | 两个启动 EXE、桌面运行时、媒体组件、字体与说明 |
| `*-update.zip` | 程序、桌面运行时和说明，不含媒体 `tools` |
| `.sha256` | 对应附件的文件完整性校验值 |

完整包内置 Electron 桌面界面，用户无需另选组件或联网下载。构建过程会验证固定媒体组件和桌面运行时的来源、哈希及许可证；只裁剪非必要翻译和上游演示资源，保留编解码及图形回退组件。

更新包只适用于已有匹配媒体组件的完整目录。组件清单变化时应提供新版完整包，不能仅覆盖程序。组件与许可证说明见[依赖说明](dependencies.md)。

## 检查产物

打包脚本会执行 Go、界面及分发脚本检查，测试失败会停止。可先独立运行：

```powershell
.\check-release.ps1 -SourceOnly
```

完整包解压到新目录后，运行 `-version`、`-check-tools` 和 `-self-test`，再检查启动、托盘、安装升级与数据保留。安装包签名后应重新生成 SHA256。

发布时保持 Git 标签、程序版本与更新说明一致。配置、笔记、密码库、历史、日志、下载缓存、测试素材和本地生成目录不得加入分发包；用户数据保留规则见[安装说明](installation.md)。
