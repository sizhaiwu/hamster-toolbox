# CLI 参数手册

[返回 CLI 入门](cli.md) · [返回首页](../README.md)

适用于 0.2.2 开发版。以下默认值是直接运行 CLI 且没有提供 JSON 配置时的值，**不会沿用页面保存的设置**；页面复制的命令则会携带当时的页面参数。示例路径均为虚构。

工具索引：[目录统计命名](#format-目录统计命名)、[批量重命名](#rename-批量重命名)、[目录批量编号](#number-目录批量编号)、[媒体信息命名](#describe-媒体信息命名)、[清理元数据](#exif-清理元数据)、[PDF 图册索引](#pdf-pdf-图册索引)、[压缩图片](#image-压缩图片)、[压缩视频](#video-压缩视频)、[视频缩略图](#thumbnail-视频缩略图)、[打包资源](#archive-打包资源)、[解压资源](#extract-解压资源)、[文件去重](#dedupe-文件去重)。

## 如何阅读命令

```powershell
.\hamster-toolbox.exe cli image --root 'E:\示例图片' --image-formats png --png-mode webp-png --png-quality 90
```

这条命令表示：使用图片压缩工具，扫描指定目录，只选 PNG，预览采用质量 90 的 WebP 编码、保留 `.png` 文件名的结果。**不带 `--execute` 就不会压缩**；检查预览后追加 `--execute` 才实际执行。

- 参数名以 `--` 开头，后面跟值；也可写 `--quality=75`。
- 开关写 `--recursive` 或 `--recursive=true`；关闭必须写 `--recursive=false`，不能写 `--recursive false`。
- 空字符串使用 `--replace=`，避免 Windows PowerShell 5.1 丢失空参数。
- 路径或文字包含空格时加引号。这里使用 PowerShell 单引号；CMD 请使用双引号。正则替换中的 `$1` 在 PowerShell 中也要用单引号包裹。
- 所有参数均位于工具名称之后。`cli video --help` 查看该工具的中文参数帮助。
- 表格中的“JSON 字段”用于 `--config` 文件，命令行参数本身仍使用左列拼写。JSON 中的路径反斜杠要写成 `\\`，布尔值不加引号，图片格式使用字符串数组。

## 通用参数

各工具章节会列出其支持的通用参数；不支持的参数会报错，不要把不同工具的参数混用。

| 参数 | JSON 字段 | 默认值 | 控制什么 |
| --- | --- | --- | --- |
| `--help` / `-h` | 无 | 不启用 | 显示当前工具帮助并退出，不处理文件。 |
| `--execute` | 无 | `false` | 重新扫描后执行；省略时仅输出预览。清理开关开启后会按配置直接清理，没有页面的二次确认。 |
| `--config <文件或->` | 无 | 不读取文件 | 读取 UTF-8 JSON 配置，最多 1 MB；`-` 表示从标准输入读取。显式命令行参数覆盖 JSON，JSON 未填写项使用 CLI 默认值。 |
| `--password-env <变量名>` | 无 | 不读取环境变量 | 仅打包、解压使用；从指定环境变量获取密码。变量缺失或为空时报错。不是直接填写密码。 |
| `--root <目录>` | `Root` | 无，批处理必填 | 输入目录。不是单个文件路径。不能使用盘符根目录、系统或其他受保护目录。 |
| `--output <路径>` | `Output` | 各工具不同 | 通常为输出目录；PDF 必须是 `.pdf` 文件路径。默认输出见各节。输出目录不能与输入目录相同或包含输入目录。 |
| `--recursive` | `Recursive` | `true` | 是否继续扫描子目录；关闭后通常只处理下一层。统计命名的统计、清理范围有例外，见该工具章节。编号、打包、PDF 不提供此参数。 |
| `--files` | `Files` | `true` | 批量重命名、打包时是否处理文件。 |
| `--dirs` | `Dirs` | `true` | 批量重命名、打包时是否处理文件夹。与 `--files` 不能同时关闭。 |

退出码：`0` 成功或没有待处理项，`1` 扫描/执行失败，`2` 参数错误，`130` 用户取消。预览与结果 JSON 写入标准输出，日志写入标准错误；Ctrl+C 取消任务，已完成的结果保留。

## 外部程序路径

完整版通常自动找到随包组件，无需填写。更换组件时使用以下参数，各工具只接受自己需要的程序。路径默认根据运行机器自动查找，不是固定个人目录。

| 参数 | JSON 字段 | 控制什么 |
| --- | --- | --- |
| `--ffmpeg <路径>` | `FFmpeg` | `ffmpeg.exe`，用于视频压缩、视频元数据流复制、缩略图取帧。 |
| `--ffprobe <路径>` | `FFprobe` | `ffprobe.exe`，用于读取媒体信息与校验视频。 |
| `--magick <路径>` | `Magick` | ImageMagick 的 `magick.exe`，用于图片处理、PDF 和缩略图拼接。 |
| `--exiftool <路径>` | `Exiftool` | `exiftool.exe`，用于清理图片及部分视频容器标签。 |
| `--font <路径>` | `Font` | 中文字体文件，支持 TTF/TTC/OTF；优先使用随包字体，也会查找系统字体。 |
| `--seven-zip <路径>` | `SevenZip` | `7z.exe`；使用 RAR 时需要带对应组件的完整版，不能只放精简版 `7za.exe`。 |

## format 目录统计命名

统计子目录内的图片、视频、音频数量和总大小，刷新目录名中的统计部分，例如 `相册 [70P2V-517MB]` → `相册 [70P-2V-517.64MB]`。大小由实际内容重新计算。

通用参数：`--root`、`--recursive`。不需要外部程序；直接修改子目录名称，不生成独立输出目录，也不改输入根目录本身的名称。

| 参数 | JSON 字段 | 默认值 | 控制什么 |
| --- | --- | --- | --- |
| `--format-clean-other` | `FormatCleanOther` | `false` | 清理资源子目录中无法识别为图片、视频、音频的普通文件，如无关 TXT/HTML。先在预览核对清单。输入根目录直接放置的文件不在此清理范围。 |
| `--format-delete-mode` | `FormatDeleteMode` | `recycle` | `recycle` 移入回收站；`permanent` 永久删除。仅清理开关开启时生效。 |

`--recursive=false` 只限制哪些层级的目录会改名。每个目录的统计仍包含深层内容；**开启无关文件清理后，清理会遍历所有资源子目录，不受此开关限制**。识别按扩展名进行，不代表对应媒体一定可以解码或压缩。

```powershell
# 仅刷新直接子目录名称，但统计其全部内部内容
.\hamster-toolbox.exe cli format --root 'E:\示例资源' --recursive=false
# 预览无关文件清单，并按清理后的内容计算统计名称；执行时移入回收站
.\hamster-toolbox.exe cli format --root 'E:\示例资源' --format-clean-other --format-delete-mode recycle
```

## rename 批量重命名

查找替换文件与文件夹名称，也可统一添加前后缀。直接改名，不移动到输出目录。

通用参数：`--root`、`--recursive`、`--files`、`--dirs`。默认文件和文件夹都处理；只改文件请加 `--dirs=false`。不需要外部程序。

| 参数 | JSON 字段 | 默认值 | 控制什么 |
| --- | --- | --- | --- |
| `--rename-mode` | `RenameMode` | 空，等同普通文字 | `text` 精确文字替换；`example` 按真实名称示例匹配；`regex` 正则表达式。建议不熟悉正则时用 `example`。 |
| `--find` | `Find` | 空 | 要替换的文字、名称片段示例或正则表达式。普通文字区分大小写；示例模式必须填写。查找针对完整名称，包含文件扩展名。 |
| `--replace` | `Replace` | 空 | 替换内容；空表示删除匹配部分。示例模式可使用 `{编号}` 保留原数字；正则模式可用 `${1}` 等捕获组。 |
| `--prefix` | `Prefix` | 空 | 替换完成后，给名称添加前缀。 |
| `--suffix` | `Suffix` | 空 | 替换完成后，给名称添加后缀；文件后缀插在扩展名之前。 |
| `--example-number` | `ExampleNumber` | `true` | 仅示例模式：把示例中最后一段连续数字作为可变编号，`001`、`002` 都能匹配。其他文字保持匹配，空格可匹配连续空白。 |
| `--example-at-start` | `ExampleAtStart` | `true` | 仅示例模式：只匹配名称开头。关闭后可匹配名称中间。开头删除时也会去掉匹配片段后紧接的空白。 |
| `--regex` | `Regex` | `false` | 兼容旧参数。未指定 `--rename-mode` 时开启正则；显式指定查找方式后，以 `--rename-mode` 为准。 |

正则采用 Go 语法，不加 `/.../g`，不支持前后断言。前后缀会应用于本次扫描选择的全部项目，并非只应用于查找命中的名称。替换后为空、非法或重名会标记冲突。

```powershell
# test 第001期 相册 → 相册；002、003 等编号也匹配
.\hamster-toolbox.exe cli rename --root 'E:\示例资源' --rename-mode example --find 'test 第001期' --replace=
# test 第001期 相册 → 更新第001期 相册
.\hamster-toolbox.exe cli rename --root 'E:\示例资源' --rename-mode example --find 'test 第001期' --replace '更新第{编号}期'
# 只给文件名添加前缀：照片.jpg → 已整理-照片.jpg
.\hamster-toolbox.exe cli rename --root 'E:\示例资源' --dirs=false --prefix '已整理-'
# 正则删除名称开头的“test 第数字期”
.\hamster-toolbox.exe cli rename --root 'E:\示例资源' --rename-mode regex --find '^test\s+第[0-9]+期\s*' --replace=
```

## number 目录批量编号

给输入目录下尚未编号的直接子目录添加连续编号，跳过能识别为已有编号的目录。不处理文件或深层目录。

通用参数：`--root`。不需要外部程序。CLI 不会自动使用页面“识别已有编号”的结果；续编必须填写 `--start-number`，同编号占用会标记冲突。

| 参数 | JSON 字段 | 默认值 | 控制什么 |
| --- | --- | --- | --- |
| `--number-name` | `NumberName` | 空 | 编号使用的资源名称。留空不加名称，不会自动从输入目录名推断。纯编号样式忽略此项。 |
| `--number-style` | `NumberStyle` | `dash` | `dash`：`名称 - NO.029`；`underscore`：`名称_NO.029`；`space`：`名称 NO.029`；`number`：`029`；`custom`：使用自定义模板。前三种不填名称时均为 `NO.029`。 |
| `--number-template` | `NumberTemplate` | 空 | 仅 `custom` 使用。必须有一次 `{number}`，可不写 `{name}` 或写一次；同时使用两者时用文字或符号分隔，不能直接相连。 |
| `--start-number` | `StartNumber` | `1` | 起始编号，整数 `0–999999999`。例如从 099 更新则填 `99`。 |
| `--number-digits` | `NumberDigits` | `3` | 最少位数 `1–9`，不足补零。例如 29 → 029；实际数字更长时不会截断。 |
| `--number-keep-original` | `NumberKeepOriginal` | `true` | 编号后追加空格和原目录名。`number` 样式额外使用 ` - ` 分隔，例如 `029 - 原目录`；关闭后仅保留生成名称。 |
| `--number-order` | `NumberOrder` | `name` | `name` 按自然名称排序（资源2 在资源10前）；`modified` 按修改时间从旧到新，同时间再按名称排序。 |
| `--number-filter` | `NumberFilter` | 空 | 只处理名称包含指定文字的未编号目录，不区分大小写；空表示不过滤。不是正则。 |

```powershell
# 相册A → 029 - 相册A；下一目录为 030 - 相册B
.\hamster-toolbox.exe cli number --root 'E:\示例资源' --number-style number --start-number 29
# 相册A → 029（只保留编号）
.\hamster-toolbox.exe cli number --root 'E:\示例资源' --number-style number --start-number 29 --number-keep-original=false
# 相册A → 示例作者 第099期 相册A
.\hamster-toolbox.exe cli number --root 'E:\示例资源' --number-style custom --number-name '示例作者' --number-template '{name} 第{number}期' --start-number 99
```

## describe 媒体信息命名

将分辨率、时长和文件大小写入音视频文件名，例如 `片段.mp4` → `片段「1920x1080,00分24秒,25.18MB」.mp4`。已有末尾 `「…」` 描述会刷新；数值来自实际媒体。

| 参数 | JSON 字段 | 默认值 | 控制什么 |
| --- | --- | --- | --- |
| `--root` | `Root` | 必填 | 存放音视频的目录。 |
| `--recursive` | `Recursive` | `true` | 是否扫描更深的子目录。 |
| `--ffprobe` | `FFprobe` | 自动查找 | 读取媒体信息的程序路径。 |

不重新编码，直接修改原文件名，无独立输出目录。读取失败的项目会在预览中标记。

```powershell
.\hamster-toolbox.exe cli describe --root 'E:\示例视频' --recursive=false
```

## exif 清理元数据

移除图片、视频容器中的可清理标签，保留原件，输出清理后的副本。不会移除画面中已经压入的视频水印或字幕广告。

通用参数：`--root`、`--recursive`、`--output`。默认输出：`<输入目录>\toolbox-output\exif`，保留相对目录结构。

程序参数：`--exiftool`；选择视频或全部时还可设置 `--ffmpeg`、`--ffprobe`，见外部程序路径表。图片及 MP4/MOV/M4V/QT 使用 ExifTool，其他支持的视频容器以 FFmpeg 流复制清理，不重新编码，并校验轨道。

| 参数 | JSON 字段 | 默认值 | 控制什么 |
| --- | --- | --- | --- |
| `--metadata-types` | `MetadataTypes` | `images` | `images` 仅图片；`videos` 仅视频；`all` 图片和视频。 |

视频扩展名包括 MP4、MOV、M4V、QT、MKV、WebM、AVI、TS、MTS、M2TS、M2T、MPG、MPEG、MPE、WMV、ASF、FLV；具体容器和轨道不能安全处理时会保留原件并报告失败。

```powershell
.\hamster-toolbox.exe cli exif --root 'E:\示例媒体' --metadata-types all --output 'E:\示例输出\已清理'
```

## pdf PDF 图册索引

把每个直接子目录作为一个封面页，使用目录标题和首张可解码图片，合并成一份 PDF；不是把所有图片逐张转成 PDF。

| 参数 | JSON 字段 | 默认值 | 控制什么 |
| --- | --- | --- | --- |
| `--root` | `Root` | 必填 | 相册父目录；只按直接子目录生成页面，封面图片可在其内部寻找。 |
| `--output` | `Output` | `<输入目录>\toolbox-output\目录索引.pdf` | 输出 PDF 文件路径，必须以 `.pdf` 结尾。 |
| `--magick` | `Magick` | 自动查找 | 用于封面处理与标题绘制的 ImageMagick 路径。 |
| `--font` | `Font` | 自动查找 | 标题使用的中文字体文件。 |

```powershell
.\hamster-toolbox.exe cli pdf --root 'E:\示例相册' --output 'E:\示例输出\相册索引.pdf'
```

## image 压缩图片

按所选扩展名压缩图片，保留原件、分辨率和相对目录结构。默认输出：`<输入目录>\toolbox-output\image`。结果没有变小时跳过输出。

通用参数：`--root`、`--recursive`、`--output`。程序参数：`--magick`。

| 参数 | JSON 字段 | 默认值 | 控制什么 |
| --- | --- | --- | --- |
| `--image-formats` | `ImageFormats` | `jpg,jpeg,jpe,jfif` | 本次处理的扩展名，逗号分隔；支持 `jpg,jpeg,jpe,jfif,png,webp`，可带点。不填不会处理 PNG/WebP。JSON 使用数组，如 `["png"]`。 |
| `--quality` | `Quality` | `60` | JPEG 与静态 WebP 的质量，`1–100`；越低通常越小。不是保留原文件大小的百分比，不控制 PNG 转码画质。 |
| `--png-mode` | `PngMode` | `lossless` | `lossless` 真正 PNG 无损优化；`webp-png` 转 WebP 但保留 `.png` 名；`webp` 转 WebP 并改为 `.webp` 后缀。仅选中 PNG 时有效。 |
| `--png-quality` | `PngQuality` | `90` | PNG 转 WebP 的质量，`1–100`，建议 `85–95`；无损模式忽略。透明度保留，颜色仍为有损压缩。 |

动画 PNG/WebP 会跳过，GIF/TIFF 等当前不支持压缩。按扩展名筛选后会检查内容；以 `.png` 命名但实际已经是 WebP 的图片仍允许再次处理，不按“以前压缩过”自动跳过。`webp-png` 可能不兼容只按后缀识别图片的软件，需要标准后缀时选 `webp`。

```powershell
# 只补压 PNG，同目录中的 JPG 不处理
.\hamster-toolbox.exe cli image --root 'E:\示例图片' --image-formats png --png-mode webp-png --png-quality 90
# 混合处理 JPEG 与静态 WebP
.\hamster-toolbox.exe cli image --root 'E:\示例图片' --image-formats jpg,jpeg,jpe,jfif,webp --quality 75
```

## video 压缩视频

保持画面尺寸和帧率，以质量等级压缩，保留原件，输出 MP4。默认输出：`<输入目录>\toolbox-output\video`，保留相对目录结构；结果没有变小时跳过输出。

通用参数：`--root`、`--recursive`、`--output`。程序参数：`--ffmpeg`、`--ffprobe`。

| 参数 | JSON 字段 | 默认值 | 控制什么 |
| --- | --- | --- | --- |
| `--video-mode` | `VideoMode` | `efficient` | `compatible`：H.264，播放兼容更好；`efficient`：H.265，通常更省空间；`archive`：H.265 CPU 慢速，耗时较长，适合不急于完成的任务。 |
| `--video-acceleration` | `VideoAcceleration` | `auto` | `auto` 依次尝试 NVIDIA、Intel、AMD 后回退 CPU；`nvidia`/`intel`/`amd` 优先所选设备、失败回退 CPU；`cpu` 只用 CPU。不是 GPU 解码开关。`archive` 始终使用 CPU。 |
| `--crf` | `CRF` | `18` | 质量等级 `0–51`，越低通常越清晰、体积越大。建议先试 `18` 清晰优先、`23` 均衡、`28` 体积优先。不同编码器会换算，不能把不同设备的同值当作相同画质。`0` 使用 CPU 的特殊质量设置，不代表工具箱承诺无损。 |
| `--min-mb` | `MinMB` | `100` | 文件至少多大才处理，整数，单位 MiB（`1024×1024` 字节）。`0` 不限制大小。 |
| `--min-kbps` | `MinKbps` | `1000` | 视频码率至少多少才处理，整数，单位 kbps（`1000` bit/s）；读不到视频流码率时使用容器码率。`0` 不限制码率。与大小条件必须同时满足。 |

CLI 默认 `--crf 18`，页面的初始“均衡”选择为 23；复制页面命令会带上页面实际值。HDR/Dolby Vision 暂不自动压缩。扩展名支持 MP4、MPG、MPE、MPEG、QT、MOV、M4V、WMV、AVI、WebM、FLV、MKV、TS，能否成功还取决于内部编码和文件完整性。更多模式比较见 [视频压缩说明](video-compression.md)。

```powershell
# CPU 高效均衡，连小文件也处理
.\hamster-toolbox.exe cli video --root 'E:\示例视频' --video-mode efficient --video-acceleration cpu --crf 23 --min-mb 0 --min-kbps 0
# 优先 Intel 核显，输出兼容性更好的 H.264
.\hamster-toolbox.exe cli video --root 'E:\示例视频' --video-mode compatible --video-acceleration intel --crf 23
```

## thumbnail 视频缩略图

每个视频均匀取帧，生成一张带文件信息和画面时间的拼图。默认输出：`<输入目录>\视频预览图`；保留相对目录结构，名称会附加原视频扩展名，例如 `片段.mp4.jpg`。

通用参数：`--root`、`--recursive`、`--output`。程序参数：`--ffmpeg`、`--ffprobe`、`--magick`、`--font`。

| 参数 | JSON 字段 | 默认值 | 控制什么 |
| --- | --- | --- | --- |
| `--thumbnail-columns` | `ThumbnailColumns` | `5` | 横向列数，`1–10`。 |
| `--thumbnail-rows` | `ThumbnailRows` | `5` | 纵向行数，`1–10`；总画面数为行×列，最多 100 格。 |
| `--thumbnail-width` | `ThumbnailWidth` | `1024` | 整张拼图总宽度，`320–4096` 像素，不是每一格的宽度；高度按视频比例、行数及文字自动计算。 |
| `--thumbnail-gap` | `ThumbnailGap` | `5` | 外边距与画面格间距，`0–40` 像素。每格扣除间距后至少需 80 像素宽，过密组合会报错。 |
| `--thumbnail-format` | `ThumbnailFormat` | `jpg` | `jpg` JPEG，通常更小；`png` PNG 无损保存拼图。 |
| `--thumbnail-quality` | `ThumbnailQuality` | `90` | JPEG 质量，`1–100`；PNG 不使用此有损质量参数。 |
| `--thumbnail-background` | `ThumbnailBackground` | `#ffffff` | 背景色，格式 `#RRGGBB`。请加引号，避免 `#` 被解释为注释。 |
| `--thumbnail-info` | `ThumbnailInfo` | `true` | 显示文件名、分辨率等文件信息；关闭不显示顶部信息。 |
| `--thumbnail-time` | `ThumbnailTime` | `true` | 显示各格画面对应时间；关闭只保留画面。 |

```powershell
.\hamster-toolbox.exe cli thumbnail --root 'E:\示例视频' --thumbnail-columns 5 --thumbnail-rows 5 --thumbnail-width 1024 --thumbnail-quality 90
# 生成 3×3 PNG，无顶部信息，深色背景
.\hamster-toolbox.exe cli thumbnail --root 'E:\示例视频' --thumbnail-columns 3 --thumbnail-rows 3 --thumbnail-format png --thumbnail-info=false --thumbnail-background '#202020'
```

## archive 打包资源

用 7-Zip 把直接子目录、文件分别打成独立压缩包，成功后校验，保留原资源。目录内的内容会整体包含，不是把输入目录合成一个大包。默认输出：`<输入目录>\0utput_7z`，开头是数字 `0`。

通用参数：`--root`、`--output`、`--files`、`--dirs`、`--password-env`。程序参数：`--seven-zip`。没有 `--recursive`；会跳过脚本、旧压缩包和工具生成的清单等项目。

| 参数 | JSON 字段 | 默认值 | 控制什么 |
| --- | --- | --- | --- |
| `--archive-mode` | `ArchiveMode` | `pure` | `pure` 纯净打包原内容；`extras` 在每项资源中加入指定附加文件，可加标记。 |
| `--archive-extras` | `ArchiveExtras` | 空 | 附加文件的完整路径，每行一个普通文件，不能是目录或链接；仅 `extras` 生效。多行推荐用 JSON 配置。附加文件不能重名。 |
| `--archive-marker` | `ArchiveMarker` | `false` | 在附加文件模式中加入 `.packing_date` 打包标记；纯净模式忽略此项。 |
| `--archive-split` | `ArchiveSplit` | `script` | `script` 首卷使用独立大小、后续使用统一大小；`uniform` 所有分卷等大；`none` 不分卷。 |
| `--archive-first-volume` | `ArchiveFirstVolume` | `10k` | `script` 的首卷大小。正整数加 `b/k/m/g`，例如 `10k`；其他模式忽略。 |
| `--archive-volume` | `ArchiveVolume` | `3g` | 后续或等大分卷大小，例如 `500m`、`3g`，不支持小数；`none` 模式忽略。 |
| `--archive-dir-prefix` | `ArchiveDirPrefix` | `D_` | 文件夹压缩包名称前缀，例如 `D_001.7z.001`。 |
| `--archive-file-prefix` | `ArchiveFilePrefix` | `F_` | 单文件压缩包名称前缀，例如 `F_001.7z.001`。 |
| `--archive-start` | `ArchiveStart` | `1` | 起始编号，`1–999999999`。文件与文件夹各自计数，不共用序号。 |
| `--archive-digits` | `ArchiveDigits` | `3` | 编号最少位数，`1–9`，不足补零。 |
| `--archive-level` | `ArchiveLevel` | `0` | `0` 仅存储，适合已压缩媒体；`1` 快速；`5` 标准；`9` 最高。更高强度通常更慢，对已压缩媒体未必明显变小。 |

不分卷时名称为 `D_001.7z` / `F_001.7z`。分卷大小使用整数单位，7-Zip 的 k/m/g 按 1024 进制解释。已有同名包或分卷时不会覆盖，请改输出目录或起始编号。密码说明见 [CLI 入门](cli.md#json-配置与密码)，不要把真实密码写进命令。

```powershell
# 与默认脚本规则一致：首卷 10k，后续 3g，文件夹与文件分别编号
.\hamster-toolbox.exe cli archive --root 'E:\示例资源' --archive-split script --archive-first-volume 10k --archive-volume 3g
# 只打包文件夹，不分卷
.\hamster-toolbox.exe cli archive --root 'E:\示例资源' --files=false --archive-split none
# 使用已在终端设置的密码变量
.\hamster-toolbox.exe cli archive --root 'E:\示例资源' --password-env TOOLBOX_ARCHIVE_PASSWORD
```

附加文件配置示例（保存为 `archive-job.json`，不是命令行参数）：

```json
{
  "Tool": "archive",
  "Root": "E:\\示例资源",
  "ArchiveMode": "extras",
  "ArchiveExtras": "E:\\示例说明\\说明.txt\nE:\\示例说明\\主页.html",
  "ArchiveMarker": true,
  "ArchiveSplit": "none"
}
```

```powershell
.\hamster-toolbox.exe cli archive --config archive-job.json
```

## extract 解压资源

使用 7-Zip 扫描并解压资源，先读取包内容并检查路径安全，默认保留原压缩包。默认输出：`<输入目录>\解压资源`。

通用参数：`--root`、`--recursive`、`--output`、`--password-env`。程序参数：`--seven-zip`。

| 参数 | JSON 字段 | 默认值 | 控制什么 |
| --- | --- | --- | --- |
| `--extract-conflict` | `ExtractConflict` | `rename` | `rename` 自动重命名重复文件；`skip` 跳过已有文件；`overwrite` 覆盖同名文件。 |
| `--extract-layout` | `ExtractLayout` | `separate` | `separate` 每个压缩包使用独立目录；`merge` 合并到输出目录，重名按上面的规则处理。 |
| `--extract-delete-source` | `ExtractDeleteSource` | `false` | 该包解压成功、校验完成后清理原包及其相关分卷；失败时保留。 |
| `--extract-delete-mode` | `ExtractDeleteMode` | `recycle` | 开启原包清理时使用：`recycle` 移入 Windows 回收站；`permanent` 永久删除。回收站操作失败会报告，不自动改成永久删除。 |

支持 7z/ZIP/RAR 及可识别分卷，例如 `.7z.001`、`.zip.001`、`.part1.rar`、RAR 的 `.rar + .r00`、ZIP 的 `.zip + .z01`。相关卷必须放在一起，程序只把入口卷作为一个任务；不是承诺支持 7-Zip 所有可读取格式。

```powershell
# 默认自动重命名重复文件，保留原包
.\hamster-toolbox.exe cli extract --root 'E:\示例压缩包' --extract-conflict rename
# 合并解压，成功后把原包及分卷移入回收站（先检查预览）
.\hamster-toolbox.exe cli extract --root 'E:\示例压缩包' --extract-layout merge --extract-delete-source --extract-delete-mode recycle
# 使用密码变量
.\hamster-toolbox.exe cli extract --root 'E:\示例压缩包' --password-env TOOLBOX_EXTRACT_PASSWORD
```

## dedupe 文件去重

检测或打开 Video Duplicate Finder 外部软件，它可查找相似视频和图片。此 CLI 不提供无人值守扫描、选择或删除重复文件。

| 参数 | JSON 字段 | 默认值 | 控制什么 |
| --- | --- | --- | --- |
| `--dedupe-program` | `DedupeProgram` | 自动查找 | 去重软件可执行文件路径。不带 `--execute` 输出检测信息；加上后仅打开其界面。 |

支持通用 `--config`、`--help`、`--execute`；不支持 `--root`、`--output`、`--recursive`。检测未找到软件时也会返回检测信息，需要查看其中是否可用；打开失败才返回执行错误。

```powershell
.\hamster-toolbox.exe cli dedupe --dedupe-program 'E:\示例软件\VDF.GUI.exe'
.\hamster-toolbox.exe cli dedupe --dedupe-program 'E:\示例软件\VDF.GUI.exe' --execute
```

## 完整 CLI 与原生命令的区别

上面的参数都属于 `hamster-toolbox.exe cli`。页面预览项还可复制 FFmpeg、FFprobe、ImageMagick、ExifTool、7-Zip 原生命令，其参数由工具箱按当前预览生成，不能把工具箱的 `--crf` 等参数直接拼到它们后面。

原生命令会绕过工具箱的部分或全部校验、暂存发布、编码回退、“结果未变小就跳过”和原包清理。普通批量任务建议使用完整 CLI。多步 PDF、缩略图、复杂元数据清理、带附加文件的打包也使用完整 CLI；具体边界见 [CLI 入门](cli.md#原生工具命令)。
