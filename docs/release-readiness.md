# v0.2.7 发布验收

[文档目录](README.md) · [发布说明](releases/0.2.7.md) · [打包说明](packaging.md)

本页记录 v0.2.7 成品包的本地验收。线上附件见 [v0.2.7 发布页](https://github.com/sizhaiwu/hamster-toolbox/releases/tag/v0.2.7)，三平台 CI 以对应提交的 [Actions 记录](https://github.com/sizhaiwu/hamster-toolbox/actions)为准。历史见 [v0.2.6 验收](releases/0.2.6-validation.md)。

## 本轮检查

- [x] 版本号、发布说明、使用文档、第三方许可和源码脱敏检查。
- [x] 固定依赖重新安装，使用 Node 24 构建离线笔记/代码编辑器及许可；63 项界面脚本全部通过。
- [x] Windows Go 测试强制重跑、静态检查、Go 源文件格式和打包安全回归。
- [x] 资源管理器内解压：实际 7-Zip、仅处理所选包、保留同名与原包、历史记录、关闭继续、任务编号隔离、窄屏操作和结果目录。
- [x] 文件新建/编辑、多文件标签与草稿、编码/换行、冲突保护、集中备份及 20 版本上限、取消恢复和大文件输入回归。
- [x] 图片首帧和顶部布局、网页有界缓存与单张预读、取消与过期隔离；原生 mpv 图片缩放、窗口记忆/排列、视频独立进度和音量；相关并发检查通过。
- [x] Windows 托盘原生指针生命周期、目录大小按需计算、特殊 ZIP、相对输出路径、打包独立编号及 TypeScript/TS 区分。
- [x] Windows/Linux/macOS 的 amd64/arm64 六种目标交叉编译。
- [x] 固定组件哈希与许可、中文目录和空配置、仅系统 PATH 下的 11 项便携自测。
- [x] 最终 ZIP/更新 ZIP 的完整性、白名单、程序一致性与 SHA256；551 个组件文件与清单一致；完整 ZIP 重新解压后再次通过版本、组件和 11 项自测。
- [x] 安装器版本资源为 0.2.7；自启默认关闭/选择开启、升级保留数据、关闭自启、卸载清理和外部自启项保留的隔离测试。

GitHub 发布在标签所指提交的 Windows、Linux、macOS CI 全部通过后完成；不以交叉编译代替远程测试。

## 本地附件

三种附件均由当前源码重新构建，组件清单沿用 v0.2.6，每份均有 `.sha256`。

| 文件 | 大小（字节） |
| --- | ---: |
| `hamster-toolbox-0.2.7-windows-x64-20261126-202438.zip` | 182483182 |
| `hamster-toolbox-0.2.7-windows-x64-20261126-202438-update.zip` | 10973592 |
| `hamster-toolbox-0.2.7-windows-x64-20261126-202438-setup.exe` | 157826205 |

SHA256：

```text
4c2dbe6be2473b4ded133f7d2a1571507f74c44b0e0b5cf8b8de4f2bf6c43911  hamster-toolbox-0.2.7-windows-x64-20261126-202438.zip
bb93e87ef11408440c4312393c387dcfe88aa855d1cc180adbfad45a5f54d6cd  hamster-toolbox-0.2.7-windows-x64-20261126-202438-update.zip
1ecd284977bfc89442f80aa2661a757329d6357aa5948ddb8798001f2e3308d1  hamster-toolbox-0.2.7-windows-x64-20261126-202438-setup.exe
```

## 验证边界

仅使用合成媒体和临时数据，安装器生命周期测试使用独立 AppId、注册表和载荷，不替换现有安装。完整发行面向 Windows x64；跨平台编译和 CI 不代表其他系统桌面、所有手机或全部显卡已经真机验证。

另一台干净电脑上的本版实际安装测试尚未完成；本地安装器隔离检查不代替它。程序和安装器未签名，笔记没有独立第三方安全审计。
