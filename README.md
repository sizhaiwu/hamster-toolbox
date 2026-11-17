# 🐹 仓鼠工具箱

一个 Windows 本地文件与媒体处理工具箱，支持批量改名、目录整理、图片和视频处理。文件在本机处理，界面通过浏览器操作。

## 功能

- **整理与命名**：目录统计命名、批量重命名、目录连续编号。
- **图片与图册**：图片压缩、元数据清理、PDF 图册索引。
- **视频与信息**：视频压缩、视频缩略图、媒体信息命名。

## 使用

1. 从 [Releases](https://github.com/sizhaiwu/hamster-toolbox/releases) 下载完整版并完整解压；尚无发布包时可按开发文档自行打包。
2. 双击 `hamster-toolbox.exe`，也可使用 `start-toolbox.cmd`。
3. 选择工具和输入目录，点击“扫描并预览”，检查后执行。

面向 **Windows 10/11 64 位**。完整版内置所需组件，无需安装开发环境。改名会直接修改名称，请先检查预览。

## 文档

- [使用指南](docs/usage.md)
- [开发与测试](docs/development.md)
- [打包与发布](docs/packaging.md)
- [外部组件与配置](docs/dependencies.md)

## 许可证

项目代码采用 [MIT](LICENSE) 许可证。随包外部组件遵循各自的许可证，见 [第三方说明](release/THIRD-PARTY.txt)。
