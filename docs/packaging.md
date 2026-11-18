# 打包与发布

[返回首页](../README.md)

发布给其他人使用时，双击 package.cmd。维护者电脑需要 Go 和 Windows 自带的 PowerShell、tar；接收者只需要解压使用，不需要开发环境。

首次打包会根据 tools-manifest.json 下载固定版本的 FFmpeg、ImageMagick 便携版、ExifTool、Noto 中文字体，以及图片组件所需的微软运行库，核对 SHA256 后整理到 tools。以后会校验并复用已有组件与 work/downloads 下载缓存，日常更新无需重复下载。

打包顺序：检查组件完整性 → Go 测试（已有 Node/jsdom 时也运行界面测试）→ 构建带版本号的 EXE → 严格检查随包组件 → 使用临时文件实际验证十一种工具 → 生成 ZIP 和 SHA256 校验文件。

打包结果在 dist 目录：

- hamster-toolbox-版本-windows-x64-时间.zip：完整版，发给新用户，包含依赖、开源字体、原组件许可说明和快速使用说明。
- hamster-toolbox-版本-windows-x64-时间-update.zip：程序更新包，发给已有对应完整版的用户；先退出旧程序，再复制到原目录覆盖程序文件，保留 tools。
- 对应 .sha256 文件：用于检查压缩包完整性。

更新包也包含新版快速说明和第三方声明，避免更新程序后仍保留过时的使用提示。

完整版与更新包使用文件白名单生成，不会复制个人设置、执行日志、data、work、源码、测试文件或旧版 EXE。设置保存在当前 Windows 用户配置目录，程序更新不会覆盖。

后续发布先修改 VERSION（例如 0.1.1），然后双击 package.cmd。也可以在 PowerShell 中指定版本：

    .\package.ps1 -Version 0.1.1

只需要程序更新 ZIP 时：

    .\package.ps1 -Version 0.1.1 -UpdateOnly

更新组件时，需要修改 tools-manifest.json 中的版本、下载地址和经核对的 SHA256，再重新打包。程序会识别组件清单与版本不匹配并提示使用新版完整版；组件发生变化时应向用户提供完整版。

运行时诊断：

    hamster-toolbox.exe -version
    hamster-toolbox.exe -check-tools
    hamster-toolbox.exe -self-test

`-check-tools` 只检查随包组件，不会用 PATH 中的媒体程序替代缺失组件；`-self-test` 在临时目录生成测试数据，实际验证十一种工具，不读取或修改个人设置及用户媒体。依赖缺失、测试失败或压缩失败时，脚本不会发布不完整的 ZIP。

版本号和构建信息保存在 release-info.json，界面页脚也会显示程序版本。打包脚本同时更新项目目录下的 hamster-toolbox.exe，旧程序保存在 work 中。

## GitHub 发布

源码及维护脚本提交到 Git；完整 ZIP、更新 ZIP 和对应的 SHA256 文件作为 GitHub Release 附件发布。`tools`、`dist`、`work` 和个人记录目录已由忽略规则排除。

项目 MIT 许可证会包含在完整包和更新包中；第三方组件的原始许可与来源清单也会保留。CI 仅运行测试，不会自动提交、推送或发布 Release。

界面的“文件去重”是可选外部软件入口，不随包附带 Video Duplicate Finder。十一项批处理工具执行便携自测，外部入口的路径识别、设置保存与界面跳转由 Go/API 和界面测试验证，不会在打包自测时打开本机去重软件。
