# v0.2.6 发布验收

[文档目录](README.md) · [发布说明](releases/0.2.6.md) · [打包说明](packaging.md)

本页记录 v0.2.6 成品包的本地验收。线上附件见 [v0.2.6 发布页](https://github.com/sizhaiwu/hamster-toolbox/releases/tag/v0.2.6)，远程 CI 以对应提交的 [Actions 记录](https://github.com/sizhaiwu/hamster-toolbox/actions)为准。历史记录见 [v0.2.5 验收](releases/0.2.5-validation.md)。

## 本轮检查

- [x] 版本号、发布说明、使用文档、随包说明与公开源码脱敏；工作台和笔记看图新增合成截图。
- [x] Windows `go test ./... -count=1`、`go vet ./...`、51 项 UI 脚本与分发脚本回归。
- [x] 多工具并行、路径冲突、独立取消、任务历史及新旧页面提醒。
- [x] Windows 密码加密记忆、关闭/清空、常用密码引用与保存失败处理。
- [x] 视频原格式输出、兼容回退、内存管道截帧与 Windows 锁文件复现。
- [x] 笔记独立看图、缩放、缓存与锁定清理，以及电脑/手机浏览器布局。
- [x] 最终源码在 Windows/Linux/macOS 的 amd64/arm64 六种目标交叉编译。
- [x] 固定组件哈希、许可、中文安装/配置/临时路径、空配置和仅系统 PATH 下的 11 项便携自测。
- [x] 最终安装包版本资源、完整 ZIP 和更新 ZIP 的 CRC/白名单/SHA256、551 个组件文件与清单一致；两种 ZIP 的程序和说明一致。
- [x] 安装器自启默认关闭、选择开启、保留自定义数据目录的升级、关闭自启、卸载清理与外部启动项保留的隔离验收。

## 本地附件

以下文件位于 `dist`。安装包和完整 ZIP 包含所需组件，更新 ZIP 不含组件；组件清单与 v0.2.5 相同。每份附件都有同名 `.sha256`，已重新计算比对。

| 文件 | 大小（字节） |
| --- | ---: |
| `hamster-toolbox-0.2.6-windows-x64-20261124-180722-setup.exe` | 157143156 |
| `hamster-toolbox-0.2.6-windows-x64-20261124-180722.zip` | 177054119 |
| `hamster-toolbox-0.2.6-windows-x64-20261124-180722-update.zip` | 10020496 |

SHA256：

```text
c6eaf2b5dc7f38d0b16b54856be64499feaf062ef40e527a2966a460718318e1  hamster-toolbox-0.2.6-windows-x64-20261124-180722-setup.exe
5283ea2003071754e7565e8c0d4c26b57081306b67fba5ec17ba7944e1f06aa2  hamster-toolbox-0.2.6-windows-x64-20261124-180722.zip
6563cb69356966a40e5529d41ca71ceefa173c4a9681f58e2d9644ae9142922e  hamster-toolbox-0.2.6-windows-x64-20261124-180722-update.zip
```

完整 ZIP 已重新解压至全新中文目录，在空配置和仅系统 PATH 下运行 `-version`、`-check-tools`、`-self-test`，11 项均通过且没有留下个人配置。第一轮成品复测发现的中文临时目录 ExifTool 自检问题已修复，本表仅记录重新构建后的附件。

## 验证边界

只使用合成媒体、临时加密笔记和测试口令，不读取或分发个人数据。安装器生命周期检查使用独立 AppId、注册表和合成载荷，不替换用户现有安装。

完整发行仍为 Windows x64；交叉编译不等于 Linux/macOS 桌面或全部显卡、手机浏览器已经真机验证。本机缩略图的额外竞态检测曾因测试运行时启动错误 0xc0000139 未完成，不计为通过；常规测试与真实媒体集成另行记录。

程序和安装器未签名，笔记没有独立第三方安全审计。另一台干净电脑的实际安装验证尚未完成；本页的安装器隔离验收不代替真机验证。GitHub CI 和上传附件的最终状态以线上记录为准。
