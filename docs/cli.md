# CLI 命令行

[文档目录](README.md)

[返回首页](../README.md)

本文描述当前完整功能基线。下载旧版时请先确认是否支持 CLI，版本号以程序 `-version` 输出为准。

11 项批处理工具可直接在 PowerShell、CMD 或脚本中运行，不启动浏览器。页面顶部“查看 / 复制 CLI 命令”会生成包含当前参数的命令（Windows 为 PowerShell，Linux/macOS 为 POSIX Shell），可选择仅预览或扫描并执行。

**逐个参数的用途、默认值、可选值和完整示例，见 [CLI 参数手册](cli-reference.md)。**

## 基本用法

```powershell
.\hamster-toolbox.exe cli --help
.\hamster-toolbox.exe cli format --root 'E:\示例资源'
.\hamster-toolbox.exe cli format --root 'E:\示例资源' --execute
.\hamster-toolbox.exe cli video --help
```

默认仅扫描，不改名、不生成文件、不删除文件。`--execute` 才会执行，会重新扫描当前目录；复制命令不是锁定的旧预览计划，目录内容变化后应先再运行预览命令。目录安全检查、重名处理、校验、编码回退与 GUI 复用同一套逻辑。

预览 JSON 和执行结果 JSON 写入标准输出，日志写入标准错误。退出码：0 成功（含无待处理项）、1 扫描或执行失败、2 参数错误、130 用户取消。Ctrl+C 取消当前任务，保留已完成结果。

## 各工具命令

以下均为预览；核对结果后添加 `--execute`。路径均为虚构示例。

| 工具 | 命令示例 |
| --- | --- |
| 目录统计命名 | `hamster-toolbox.exe cli format --root E:\示例资源` |
| 批量重命名 | `hamster-toolbox.exe cli rename --root E:\示例资源 --find old --replace new` |
| 目录批量编号 | `hamster-toolbox.exe cli number --root E:\示例资源 --number-style number --start-number 29 --number-digits 3` |
| 媒体信息命名 | `hamster-toolbox.exe cli describe --root E:\示例视频` |
| 清理元数据 | `hamster-toolbox.exe cli exif --root E:\示例媒体 --metadata-types all` |
| PDF 图册索引 | `hamster-toolbox.exe cli pdf --root E:\示例相册` |
| 压缩图片 | `hamster-toolbox.exe cli image --root E:\示例图片 --image-formats jpg,png,webp --quality 75` |
| 压缩视频 | `hamster-toolbox.exe cli video --root E:\示例视频 --video-mode efficient --video-acceleration cpu` |
| 视频缩略图 | `hamster-toolbox.exe cli thumbnail --root E:\示例视频 --thumbnail-columns 5 --thumbnail-rows 5` |
| 打包资源 | `hamster-toolbox.exe cli archive --root E:\示例资源 --archive-split none` |
| 解压资源 | `hamster-toolbox.exe cli extract --root E:\示例压缩包 --extract-conflict rename` |

布尔参数关闭时使用 `=false`，例如 `--recursive=false`、`--files=false`。清理开关默认关闭；CLI 指定清理并带 `--execute` 后会直接处理，无额外确认，请先预览。

“文件去重”另提供 `cli dedupe --dedupe-program 'E:\软件\VDF.GUI.exe'`，只检查软件；加 `--execute` 只打开外部 GUI，不执行无人值守去重。

## JSON 配置与密码

复杂参数可使用 `--config job.json`；`--config -` 从标准输入读取 UTF-8 JSON，最多 1 MB。配置字段与页面设置一致，命令行显式参数覆盖 JSON 配置；不读取 GUI 保存的工作区，避免自动任务被页面操作影响。

生成的命令包含当前选择的本地路径和名称，分享前请脱敏。若保存为 `.ps1` 并用 Windows PowerShell 5.1 运行，中文脚本请保存为 UTF-8 BOM；直接粘贴运行无需保存脚本。

```json
{"Tool":"rename","Root":"E:\\示例资源","Prefix":"new-","Files":true,"Dirs":false}
```

页面生成的 PowerShell 命令会处理中文输入编码，并在结束后恢复原来的输出编码。命令不包含压缩密码；已填写密码时会引用环境变量：

```powershell
$secret = Read-Host '输入压缩密码' -AsSecureString
$env:TOOLBOX_ARCHIVE_PASSWORD = [Net.NetworkCredential]::new('', $secret).Password
.\hamster-toolbox.exe cli archive --root 'E:\示例资源' --password-env TOOLBOX_ARCHIVE_PASSWORD --execute
Remove-Item Env:\TOOLBOX_ARCHIVE_PASSWORD
```

解压使用 `TOOLBOX_EXTRACT_PASSWORD`。不要把真实密码写入公开脚本或配置文件；环境变量也不是对当前账户内其他程序的安全隔离。

## 原生工具命令

扫描后的单项预览支持按需查看 FFmpeg、FFprobe、ImageMagick、ExifTool 或 7-Zip 命令。原生命令直接调用外部程序，不包含工具箱的完整校验、临时发布、自动回退、原包清理和“未变小就跳过”等流程。原生解压会绕过工具箱的包内路径检查，覆盖模式会覆盖同名文件。

目录改名、编号、PDF、缩略图、多容器元数据清理，以及带附加文件或标记的打包属于多步流程，请使用页面顶部的完整 CLI 命令。已有的视频压缩命令入口继续保留。原生命令中的密码同样通过环境变量引用。

## 自动验证

仓库保留实际 EXE 的集成测试 `cli_integration_test.go`。11 项批处理工具各运行一次直接 CLI 和一次页面生成的 PowerShell 命令，共 22 组；另执行图片压缩、图片元数据清理、纯净打包、解压、媒体信息读取、CPU 视频压缩的 6 组原生命令。测试使用临时生成的资源，覆盖中文、空格、单引号、方括号和百分号路径，检查预览不改动目录及文件内容、输出可读取、压缩包完整性、解压内容一致和密码不出现在输出中。

开发者可在完整便携目录构建程序后运行（普通用户无需操作）：

```powershell
$env:HAMSTER_CLI_TEST_EXE = (Resolve-Path .\hamster-toolbox.exe).Path
go test . -run '^TestCLIExecutableIntegration$' -count=1 -v
Remove-Item Env:\HAMSTER_CLI_TEST_EXE
```

这验证了当前程序和随包依赖在测试环境中的执行。不同显卡、驱动、损坏文件及其他资源格式仍需结合实际环境判断，不能据此保证所有输入均成功。
