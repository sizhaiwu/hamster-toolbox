# 体验素材

[返回首页](../README.md) · [工具功能示例](../docs/examples.md) · [CLI 参数手册](../docs/cli-reference.md)

无需寻找自己的资源，使用仓库中的合成素材即可体验。原始素材在 [assets](assets)，总计约 7.5 MiB；图片、测试视频、提示音均为程序生成，采用本项目 MIT 许可证，不含真实人物、个人文件、定位或私人密码。少量 `Hamster Demo` 作者、标题、注释是故意加入的公开演示标签，用于元数据清理。

## 一键准备

1. 获取仓库（GitHub 上点 **Code → Download ZIP** 并完整解压，或使用 Git 克隆），在根目录双击 **`try-samples.cmd`**。
2. 自动打开独立体验目录和 `START-HERE.txt`；说明中列出每项工具可以直接复制的完整输入路径、建议设置和预期结果。
3. 打开工具箱，选择对应工具，粘贴该项的 `input` 路径，按建议设置扫描预览，确认后执行。输出路径留空。

工具箱会保护自己的程序目录，因此不要直接把仓库里的 `samples` 当作输入，也不要把整个体验目录作为一个工具的输入。脚本只复制已有素材，不下载程序、不调用 FFmpeg 等组件、不启动处理任务、不修改工具箱设置。每次运行都会在当前用户临时目录创建新的 `hamster-demo-xxxxxxxx` 文件夹，保留旧体验目录和仓库原件。

如 Windows 提示下载文件安全警告，请先核对脚本来源和内容。只查看素材无需安装开发环境；执行媒体工具仍需下载工具箱完整版，或按文档构建并准备外部组件。[发布下载](https://github.com/sizhaiwu/hamster-toolbox/releases)中的 0.2.1 可使用 GUI 体验，样本生成的 CLI 配置需 0.2.2 开发版。

提示音和视频含测试音，试听前请调低音量。系统可能清理临时目录，需长期保留时复制到自己的普通资源目录。演示素材不是实际照片、影片的画质或性能基准，压缩率与速度不能代表真实资源。

## 各项体验

以下路径相对于脚本生成的体验目录。完整绝对路径以打开的说明为准，`…` 表示本行的 `input` 目录。

| 工具 | 选择哪个输入目录 | 建议设置与观察结果 |
| --- | --- | --- |
| 目录统计命名 | `01-format/input` | 两个相册使用旧统计名，扫描后刷新实际 P/V/A 数量和大小。可再勾选清理无关文件，观察子目录中的 `说明.txt` 被列入清理预览，默认回收站。 |
| 批量重命名 | `02-rename/input` | 按示例查找 `test 第001期`，替换留空，编号可变、只替换开头均开启。001/002/003 的名称前缀都会删除。 |
| 目录批量编号 | `03-number/input` | 仅编号、起始 29、位数 3、保留原名；得到 `029 - 资源1`、`030 - 资源2`、`031 - 资源10`，已有 028 跳过。 |
| 媒体信息命名 | `04-describe/input` | 直接扫描；视频与音频名追加真实时长、体积，视频还追加分辨率。 |
| 清理元数据 | `05-exif/input` | 选择图片和视频；JPG、MP4、MKV 清理后写入 `…/toolbox-output/exif`，原件保留。 |
| PDF 图册索引 | `06-pdf/input` | 两个相册各一页，输出 `…/toolbox-output/目录索引.pdf`。 |
| 压缩图片 | `07-image/input` | 勾选 JPG/PNG/WebP，普通质量 75，PNG 选 WebP 编码保留 `.png` 名、质量 90。输出 `…/toolbox-output/image`；也可只选 PNG 体验补压。 |
| 压缩视频 | `08-video/input` | 高效、仅 CPU、均衡 23；**高级设置中最小体积、最小码率都设 0**，否则 5 秒小样本可能被默认过滤。输出 `…/toolbox-output/video`。 |
| 视频缩略图 | `09-thumbnail/input` | 3 行 × 3 列、总宽 960，显示信息和时间。输出 `…/视频预览图/五秒彩色运动.mp4.jpg`。 |
| 打包资源 | `10-archive/input` | 纯净打包、不分卷；输出 `…/0utput_7z/D_001.7z`、`F_001.7z` 和清单。附加文件模式可选择 `10-archive/extras/说明.txt`。 |
| 解压资源 | `11-extract/input` | 公开演示密码 **`hamster-demo`**。默认每包独立目录，自动重命名、保留原包。包含 2 个 ZIP、1 个加密 7z、1 套两卷加密 7z，共 4 个任务。切换合并目录可观察 `note.txt` 重名处理。 |
| 文件去重 | `12-dedupe/input` | 通过外部 Video Duplicate Finder 手动扫描，查看完全相同图片/视频、近似图片的分组。外部软件须另行安装，相似结果受阈值影响；工具箱不会自动删除。 |

改名、编号会改变当前体验副本。再次执行可能没有待处理项；已有输出也可能造成冲突，需要重新试用时再次双击 `try-samples.cmd`，得到一份新副本。原包清理和无关文件清理默认关闭；开启前检查预览，只在体验副本上练习。

## CLI 体验

每项批处理体验目录内有 `job.json`，保存该项的建议设置和本次输入路径，供 CLI 读取；它不是页面设置的导入文件。完整路径在 `START-HERE.txt` 中。去重场景不生成任务配置，需使用外部软件手动选择路径。

```powershell
# 替换为说明中当前这一份 job.json 的路径，默认只预览
.\hamster-toolbox.exe cli image --config '某个体验目录\07-image\job.json'
# 检查后执行
.\hamster-toolbox.exe cli image --config '某个体验目录\07-image\job.json' --execute
```

解压配置中保存的是刻意公开的演示密码 `hamster-demo`，不用于私人资源；自己的密码应通过 [密码环境变量](../docs/cli.md#json-配置与密码)传入。体验目录移动后配置中的旧绝对路径不会自动更新，重新运行准备脚本或自行更正 `Root`。

只准备素材而不打开窗口，或选择其他存放位置：

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\samples\create-playground.ps1 -NoOpen
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\samples\create-playground.ps1 -OutputParent 'E:\体验素材' -NoOpen
```

自定义位置必须在仓库外，不经过符号链接或目录联接；脚本始终创建一个新子目录，不覆盖或删除已有文件。

## 素材清单与维护

- `assets/color-card.jpg`：高质量合成色卡，包含演示作者和注释。
- `assets/transparent-card.png`：未经压缩优化的透明几何图。
- `assets/color-card.webp`、`color-card-near.jpg`：静态 WebP 与近似 JPEG，供压缩、去重体验。
- `assets/motion-demo.mp4`：640×360、24 fps、约 5 秒的彩色测试画面和提示音，含演示标题。
- `assets/metadata-demo.mkv`：同一视频的 MKV 容器，体验 FFmpeg 流复制元数据清理。
- `assets/tone-demo.wav`：约 1 秒的合成提示音。
- `assets/plain.zip`、`second.zip`、`encrypted.7z`、`split.7z.001/.002`：普通、加密与加密分卷练习包。
- `assets/manifest.json`：素材来源、许可证、大小与 SHA256；准备脚本会校验源文件完整性。
- `scenarios.json`：各工具的复制布局、建议参数、体验说明；不含个人路径。

开发者使用 `generate-assets.ps1` 配合仓库内已准备的工具生成素材；普通用户无需运行。为避免意外覆盖，目标目录必须尚不存在；重新生成时可指定独立的新目录，再核对文件与清单后更新仓库：

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\samples\generate-assets.ps1 -OutputDirectory 'E:\新合成素材'
```
