# 外部组件与配置

[返回首页](../README.md)

完整版已包含下列组件。单独使用 EXE，或希望指定自定义程序时，可参考本页。

1. 选择左侧工具，再选择要处理的输入目录。
2. 完整版会自动找到随包的组件，所有工具均可在本机运行。单独使用 EXE 时，媒体工具会提示需要的程序，并提供下载入口。
3. 点击“扫描并预览”，检查目标路径与冲突，再点击“执行预览结果”。输出路径可以留空，程序会自动生成。

| 当前工具 | 需要的程序 |
| --- | --- |
| 目录统计命名、批量重命名、目录批量编号 | 无 |
| 媒体信息命名 | FFprobe |
| 清除图片信息 | ExifTool |
| 压缩图片 | ImageMagick |
| 压缩视频 | FFmpeg、FFprobe |
| 视频缩略图 | FFmpeg、FFprobe、ImageMagick、中文字体（通常使用系统字体） |
| PDF 目录 | ImageMagick、中文字体（通常使用系统自带字体） |

每个工具分别记住输入目录、输出位置、参数与依赖路径。随包组件以相对路径保存，移动或更新完整版后会自动重新定位；仍可用的外部自定义程序路径会保留。切换工具后不会沿用另一个工具的输出位置或自定义依赖；第一次打开另一工具时会复用当前输入目录，减少重复选择。
设置由后端保存在当前 Windows 用户配置目录的 media-toolbox/settings.json，随机端口变化或重启后仍会保留。-data 参数可以指定设置与记录目录。界面会显示保存状态，保存失败会提示。

“自动查找”优先检查 EXE 旁的 tools 文件夹，其次检查 PATH、ImageMagick 的常见安装位置。支持如下便携目录，须保留程序的完整文件夹内容：

- tools/ffmpeg/bin/ffmpeg.exe 与 ffprobe.exe（也可直接放在 tools/ffmpeg/）
- tools/imagemagick/magick.exe
- tools/exiftool/exiftool.exe 与它需要的 exiftool_files 文件夹
- tools/fonts/NotoSansCJKsc-Regular.otf（完整版内置，也支持同名 .ttf/.ttc）

无需修改 PATH：“选择文件”可以直接选程序或字体。“检查是否可用”会运行版本检测，并提示选错程序、无法启动或检测超时。

下载入口：

- [FFmpeg 官方列出的 Windows 构建](https://www.gyan.dev/ffmpeg/builds/)：[官方来源页](https://ffmpeg.org/download.html#build-windows)。选择 release essentials ZIP，解压后选择 bin/ffmpeg.exe；FFprobe 在同一包中。
- [ImageMagick Windows 下载](https://imagemagick.org/download/#windows)：普通 Windows 电脑选择 x64 安装包或便携版，选择 magick.exe。
- [ExifTool 官方下载](https://exiftool.org/)：[Windows 安装说明](https://exiftool.org/install.html#Windows)。解压完整包，将 exiftool(-k).exe 改为 exiftool.exe，保留随附的 exiftool_files 文件夹。
- [Noto CJK 字体](https://github.com/notofonts/noto-cjk/releases)：仅在未找到系统中文字体时需要，选择 TTF/TTC 字体。

完整版请整体移动，保持 EXE、tools 文件夹和 tools-manifest.json 在一起。正常使用不需要联网。单独的 EXE 仍需要相应外部组件，下载链接会打开用户浏览器，工具箱不会自动安装程序。
图片压缩仅开放 JPEG、PNG、静态 WebP，需本机 ImageMagick 具备相应编码能力；图片识别统计及元数据清理仍使用各自的格式范围。
清除全部元数据会移除方向/色彩配置等标签，可能改变查看器的显示效果，请检查输出副本。
PDF 标题采用中文字体栅格渲染，保持长目录名可换行；PDF 不含可选择文本或可点击书签。
视频画质提供清晰、均衡、体积优先预设。高级设置可以调整 CRF、最小体积和最小码率；低于阈值的文件会在预览扫描时过滤，想处理全部视频时将两个阈值设为 0。

随包的微软 OpenMP 运行库位于 ImageMagick 目录，准备脚本只解包所需文件，不执行系统安装。各组件的来源、固定版本和校验值见 [tools-manifest.json](../tools-manifest.json)。项目 MIT 许可证不替代第三方组件的许可，原始许可文件会随发布包保留。
