# 测试与验收

[文档目录](README.md) · [开发说明](development.md)

测试以临时合成素材为主，不使用个人媒体。自动化覆盖不等于所有硬件、第三方解码器和系统桌面均已真机验证。

缩略图定位兼容：`go test ./internal/toolbox -run 'TestThumbnailEmptyFastSeek|TestThumbnailRetry'` 覆盖 FFmpeg 成功退出但未输出图片、短区间与从头精确定位重试、截取时间保持、临时帧清除与取消；设置 `TOOLBOX_INTEGRATION=1` 后可运行 `TestThumbnailTransportStreamNamedMP4Integration`，用长 GOP 的 MPEG-TS 模拟错误的 MP4 后缀。可选 `TOOLBOX_THUMBNAIL_SAMPLE_GLOB` 仅用于本机明确选择的只读样本，验收生成物存于临时目录并清理，原视频执行前后比对 SHA256。

共享首次登录：`go test . -run TestGuestLogin` 会从实际共享 HTML 提取脚本、样式和图片地址，逐项验证无会话时可加载；目录、资源与管理接口仍须认证。另验证登录设置 Cookie 后不重新 GET 页面即可读取目录。`node tests/explorer-login-check.cjs` 验证首次输入口令直接进入、错误口令、重复提交保护、会话过期后重登、Cookie 未保存与初始化错误提示；这些情况不需要手动刷新页面。

解压实时进度：`go test . -run TestOperationProgress`、`node tests/extract-progress-check.cjs`，以及设置 `TOOLBOX_INTEGRATION=1` 后的 `go test ./internal/toolbox -run 'TestSevenZipProgress|TestExtract' -count=1`。覆盖真实 7-Zip 输出、任意分块与回车/退格、文件名伪百分比排除、事件限频、检查/写出阶段、复制大文件时的字节回调、取消和源文件保留；页面验证当前包与总包数分离、预计时间、已知零进度与未知进度、刷新恢复和任务结束后的清除。完成阶段只有在安全检查与写出成功后报告。

版本检查：`go test . -run 'TestRelease|TestVersionUpdate'` 和 `node tests/version-info-check.cjs`。使用本地 HTTP 合成响应，覆盖正式版与开发版比较、数字版本排序、缓存与并行请求合并、取消/超时、无发布、限流、无效或过大响应、仅当前会话可访问、页面失败提示和共享访客隔离；不会在自动化测试中连接 GitHub。应用只请求固定仓库的公开 latest release API，不接受任意请求地址或外部下载执行指令。

无格式后缀分卷：设置 `TOOLBOX_INTEGRATION=1` 后运行 `go test ./internal/toolbox -run TestExtractNumeric -count=1`。合成 7z/ZIP/RAR 的 `.001/.002` 分卷，验证真实文件头识别、首卷去重、内部文件清单、正确解压、命令导出、加密密码提示、缺卷拒绝、默认保留和显式清理原卷，以及上传旁文件与普通数字后缀文件的保留。路径穿越样本必须在读取内部清单时被阻止，不会把分卷简单合并成一个大文件。

Windows 开机自启：`go test . -run TestAutostart` 和 `node tests/autostart-check.cjs` 验证显式开关、路径和参数转义、系统状态重读、失败回滚、异步关闭重开及共享访客隔离。注册表测试仅访问随机隔离键，不修改真实 Run 项。`powershell -NoProfile -ExecutionPolicy Bypass -File tests/installer-autostart-check.ps1` 使用 Inno Setup 编译器、合成文件、独立 AppId 与注册表键，验证默认关闭、安装勾选、升级保留数据目录、取消和卸载清理，以及其他安装位置自启项的保留；不会运行合成程序或让电脑自动启动它。托盘启动命令另用 GUI 集成测试验证，无需注销或重启系统。

## 基础与集成测试

Windows 维护者可直接运行 `check-release.cmd`，或 `./check-release.ps1 -SourceOnly` 只检查源码；完整运行还要求准备好匹配的 EXE 和组件。脚本不打包，详见[打包说明](packaging.md)。


基础检查不依赖媒体程序：

```powershell
go test ./...
go vet ./...
```

前端交互回归：

```powershell
npm ci --ignore-scripts
npm run test:ui
```

Windows 可单独运行分发脚本回归（只操作临时合成文件，不生成 ZIP、不发布）：

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File tests/package-check.ps1
```

测试使用隔离 DOM 和模拟接口，覆盖工具切换、参数保存、依赖提示、预览、编号和冲突处理。

准备好 `tools` 后，运行真实媒体集成测试：

```powershell
$env:TOOLBOX_INTEGRATION = '1'
go test ./... -count=1
Remove-Item Env:TOOLBOX_INTEGRATION
```

集成测试优先使用仓库内准备的组件，也可使用 PATH 中的程序。默认样本在临时目录生成；HEIC 编码器未随包提供，可额外设置 `TOOLBOX_HEIC_FIXTURE` 指向一份 HEIC 测试图片，验证读取与兼容预览。不设置时只跳过这一个外部样本测试。可参考 [libheif 官方样本](https://github.com/strukturag/libheif/tree/v1.23.7/examples)，请放在忽略的 `work` 目录内，勿把外部图片加入发布包。

资源管理器专项集成测试覆盖普通与分片 MP4、MKV、AVI、TS、MOV、WebM、HEVC 10-bit、WMV、FLV、MPG、3GP 的封面和兼容输出，以及 JPEG、PNG、WebP、GIF、BMP、TIFF、AVIF 和可选 HEIC 的图片解码。流式测试也检查取消后结束处理、源文件不变及缓存容量。

视频专项测试使用生成的样本，覆盖三种模式、故障回退、音频重试、10-bit、VFR 时间戳、旋转、奇数尺寸、AAC 数据不变以及 HLG / HDR10 保留、SDR 色调映射与 HDR 安全跳过。具有 NVIDIA 和 Intel 核显的开发机可额外启用实际硬件编码验证（普通 CI 不要求显卡）：

```powershell
$env:TOOLBOX_INTEGRATION = '1'
$env:TOOLBOX_HARDWARE_INTEGRATION = '1'
go test ./internal/toolbox -run 'TestVideo|TestHDR' -v
Remove-Item Env:TOOLBOX_INTEGRATION
Remove-Item Env:TOOLBOX_HARDWARE_INTEGRATION
```

发布构建还提供随包验证命令：

```powershell
.\hamster-toolbox.exe -version
.\hamster-toolbox.exe -check-tools
.\hamster-toolbox.exe -self-test
```

## 专项回归

音轨时间戳：`go test ./internal/toolbox -run 'TestVideoAudioTimestamp|TestVideoAudioRepair'`；设置 `TOOLBOX_INTEGRATION=1` 后包含实际 FFmpeg 回归。测试生成带重叠音频片段的合成 MP4，验证真实复制失败后在原编码器重试、成品解码与单调音轨时间戳、源文件不变，以及随后切换视频编码器时沿用音轨修复。错误识别区分音轨 DTS、视频 DTS、普通封装错误及磁盘不足。

剪贴板入口：`node tests/explorer-clipboard-check.cjs` 验证菜单图标点击、持续显示已复制/剪切数量、无需空白区域的顶部粘贴、直接粘贴到目录、剪切成功后清空、快捷键与文本输入隔离，以及无效选择和只读权限。真实浏览器使用合成文件，在满列表中完成复制，再通过目标文件夹菜单完成剪切粘贴，核对源/目标内容。

目录大小与合并：`go test ./... -run 'TestBrowserDirectorySize|TestResourceDirectorySize|TestBrowserMerge|TestBrowserTransferConflict|TestDesktop'` 和 `node tests/explorer-properties-check.cjs`。覆盖按需统计、取消、共享边界、同名选择、替换前校验与原件备份、目标独有文件保留及移动时跳过源文件保留。Windows 无控制台与重复启动集成测试需设置 `TOOLBOX_DESKTOP_TEST_EXE` 指向新构建的 GUI EXE，再运行 `go test . -run TestDesktopGUIAndSingleInstanceIntegration`；使用独立临时数据目录，不打开浏览器。安装包构建与验收说明见[安装与托盘](installation.md)。

任务历史：`go test . -run TestTaskHistory`、`go test ./internal/toolbox -run TestRemainingPlan`、`node tests/task-history-check.cjs`。覆盖中断识别、跨进程租约、密码排除、历史清理、记录损坏降级、定向重扫与筛选边界；`TOOLBOX_INTEGRATION=1 go test . -run TestTaskHistoryObservedMediaIntegration` 使用合成图片验证并发执行、失败记录与重扫仅处理未成功文件。

常用文件图标、悬停播放、失败提示、触屏排除及共享范围：`node tests/explorer-hover-check.cjs`；后端权限与资源限制：`go test . -run 'TestBrowserHover|TestBrowserMediaRoutes'`。

资源打包的实际分卷、加密、解压、附加文件、取消清理和输出冲突测试：`TOOLBOX_INTEGRATION=1 go test ./internal/toolbox -run TestArchive -count=1`。Windows PowerShell 先设置 `$env:TOOLBOX_INTEGRATION="1"`。测试仅使用临时合成资源，不读取私人脚本或媒体。

解压集成测试：`TOOLBOX_INTEGRATION=1 go test ./internal/toolbox -run TestExtract -count=1`。覆盖重名规则、加密分卷、损坏包、路径安全与原生 RAR 解码；使用合成资源，不依赖个人压缩包。Windows 账户加密密码列表由 `go test . -run TestPasswordVault` 验证。

跨平台构建与功能限制见 [platforms.md](platforms.md)。资源管理器增强测试：`node tests/explorer-enhancements-check.cjs`。视频短片/可跳转取帧验证：`TOOLBOX_INTEGRATION=1 go test ./internal/toolbox -run TestBrowserVideoThumbnailSeekableAndShortIntegration -count=1`。

当前实现及验证边界见 [optimization-review.md](optimization-review.md)。

悬浮媒体测试：`node tests/explorer-floating-check.cjs` 验证多个独立窗口、原生视频控件与菜单中的兼容选项、进度/音量隔离、拖动/调整大小/层级置顶、切换目录保留、普通预览隔离、大量窗口下的置顶顺序、关闭取消、共享根目录固定和登录失效清理。播放器由 `createVideoPlayback` 创建独立控制器，普通预览和每个悬浮窗口分别持有与释放。

原生 mpv 测试：`node tests/explorer-native-check.cjs` 验证本机入口、文件版本引用、真实硬解/CPU 状态、安全文件名、独立控制以及共享访客隔离。`go test . -run 'TestNativePlayer|TestVideoRanges|TestLightweight'` 验证本机认证与来源校验、IPC 指令白名单、文件名参数隔离、结束后重播、Range 连续读取/跳转/下载以及轻量封装检查。IPC 只允许暂停、播放、静音、置顶、跳转和关闭自己启动的窗口，不向浏览器开放 mpv 任意命令。Windows 使用随机命名管道，Linux/macOS 使用私有临时目录中的 Unix socket。

硬件验收应使用真实机器：同时打开多个合成测试视频，确认时间持续增加、“原生播放器”显示 `hwdec-current` 的实际值（如 `d3d11va`），并在此时打开图片；仅传入 `--hwdec=auto` 或看到进程启动不算硬解通过。浏览器播放应跨越多个 Range 块并测试跳转。各显卡、驱动与媒体编码仍需要分别验证。

目录快捷方式测试：`go test . -run TestDirectoryShortcut -count=1` 验证格式/版本校验，以及 Windows 原生 ShellLink 读取、中文与特殊字符路径、共享内跳转、共享外拦截、失效目标和程序入口拒绝；不执行快捷方式目标。`node tests/explorer-shortcut-check.cjs` 验证双击、新标签页、错误提示、过期响应隔离及共享相对路径。Windows 测试使用 COM 创建合成 `.lnk`，不依赖 PowerShell、个人资源或外部程序。

资源交互测试：`node tests/explorer-interaction-check.cjs` 验证鼠标侧键与驱动 Alt 导航、Pointer/Mouse 事件去重、弹窗隔离、属性安全渲染和关闭取消；`go test . -run TestResourcePropertiesScopeAndIdentity` 验证属性接口的文件身份、共享根与越界保护、主机路径隐藏和本机认证。原生图片入口、图片状态与视频控件隔离由 `explorer-native-check.cjs` 验证，静态图无限显示与控件缩放参数由 `TestNativePlayerIPCWhitelistAndArgumentIsolation` 验证。

资源管理器导航测试：`node tests/explorer-navigation-check.cjs`，覆盖侧栏折叠、设置记忆、右键菜单、键盘操作、多选、返回顶部和共享权限；自然排序及分页接口由 `go test . -run "TestBrowserNaturalOrderingAndDirection|TestBrowserPaginationAndSavedControls|TestToolboxSidebarSetting"` 验证。

资源列表与预览布局测试：`node tests/explorer-layout-check.cjs`，覆盖更多操作入口、可点击表头、同类文件列表、键盘切换、节点复用、关闭清理及只读共享。界面验证使用合成媒体，不把外部分享页或私人资源写入仓库。

本机打开方式：`go test . -run TestOpenWith` 与 `node tests/explorer-open-with-check.cjs` 验证所有者权限、文件版本、普通文件限制、程序参数隔离、取消和并发保护。Windows 使用 [SHOpenWithDialog](https://learn.microsoft.com/en-us/windows/win32/api/shlobj_core/nf-shlobj_core-shopenwithdialog)，macOS 使用固定 AppleScript 选择应用后以独立参数调用 open；Linux 显式选择可执行程序。不会把文件名拼成 shell 命令。

选择交互由 `node tests/explorer-selection-check.cjs` 验证：单击仅高亮且不勾选隐藏复选框，双击/Enter/工具栏打开、触屏双击、Ctrl/⌘ 与 Shift 多选、显式多选复选框和只读共享。原有预览用例通过真实 click/click/dblclick 顺序激活项目，不改变测试环境中 click 的含义。

快速访问由 `node tests/explorer-quick-access-check.cjs` 验证：重复标题栏移除、侧栏底部返回、旧收藏保留、当前目录/右键目录添加、重复防护、导航右键取消固定及访客隔离。沿用原来的 favorites 存储字段，配合 `explorer-preferences-check.cjs` 覆盖保存失败回滚、重启恢复和旧缓存隔离。取消固定只保存偏好，不调用文件改名、移动或删除接口。

## 时长、预览列表和长目录返回

```powershell
node tests/explorer-media-details-check.cjs
go test . -run 'TestResource.*Properties' -count=1
```

覆盖千视频目录/预览侧栏的按需读取、最多 2 个并发、同一文件进行中的请求去重、共享缓存、隐藏/关闭侧栏取消、未知时长、共享范围，以及滚动/分页/视图与刷新后历史恢复。图片尺寸按真实编码读取，包含后缀不一致的样本。真实编码器的图片/视频属性测试需要 `TOOLBOX_INTEGRATION=1`。

## 人工界面与硬件验收

共享保存反馈由 `node tests/explorer-share-feedback-check.cjs` 验证：成功/关闭、错误、部分目录离线提示，错误文本安全显示、密码清空、重复保存保护和关闭设置后的延迟响应不重新弹窗。人工检查保存结果在设置弹窗之上居中显示，确认后返回设置，长错误可滚动阅读。

标签菜单和导航排序专项：`node tests/explorer-reorder-check.cjs`。覆盖指针拖动、后台标签不切换、不重新读取目录、历史及刷新顺序保留、复制和批量关闭、快速访问保存失败回滚、外部拖放拒绝及共享访客隔离。真实浏览器还应验证左右/上下拖动、点击与拖动的阈值、释放后不误触，以及触屏的“···”菜单。

- 电脑与手机：单击选择、双击打开、工具栏不推移项目；小屏不溢出，按钮可操作。
- 长列表：进入可见区域中部的子目录，再返回，确认分页、视图和原位置恢复；验证不同标签历史互不干扰。
- 预览：图片/视频底部和侧栏缩略图、侧栏视频时长、图片缩放、未知时长或失败回退；滚动侧栏只读取附近项目。
- 本机多路 mpv：确认独立声音/进度、窗口大小、置顶与实际硬解状态；打开图片不应等待视频读完整文件。
- 共享：只读和可写账户、手机访问、Range 跳转、代理来源与 HTTPS 配置；不使用私人外网账号做默认验收。
- 便携包：在新路径、空用户配置、系统 PATH 下运行 `-check-tools` 和 `-self-test`；不能用源码测试通过代替便携包验收。

记录使用的系统、硬件、组件和测试命令。跳过的集成样本应明确说明；交叉编译通过不等于真机测试通过。文档或截图变更只需检查链接、示例、隐私和显示，无需启动批处理修改用户文件。

图标间距由 `go test . -run TestExplorerGridGapPersistenceAndCompatibility` 与 `node tests/explorer-preferences-check.cjs` 验证：兼容旧设置、0px 保存与重启、非法值不改写、图标大小独立、调整不重新读取目录、列表隔离和访客本地记忆。人工验收还需检查电脑/手机弹出面板与实际行列间距。

任务提醒由 `node tests/task-feedback-check.cjs` 和 `go test . -run TestTaskNotificationSettingsPersistence` 验证：完成状态分类、快速结束、刷新恢复、确认去重、提示音试听与静音、通用设置跨重启保存、互不覆盖、保存失败恢复，以及浏览器存储和声音受限时的降级。真实界面需执行一项合成素材任务，检查结果弹窗，并试听提示音。

打包高级选项的展开状态由 `go test . -run TestArchiveAdvancedDisclosurePersistence` 和 `node tests/ui-check.cjs` 验证，覆盖展开/收起保存、旧设置兼容、跨重启恢复、独立参数保存，以及保存失败时恢复原状态。

视频自动播放由 `node tests/explorer-playback-check.cjs` 验证：双击与切换后启动、优先有声播放、浏览器限制时静音重试、重试失败提示、分片与兼容模式延续，以及用户暂停和关闭后的异步隔离。真实浏览器使用仓库合成视频检查时间递增、暂停状态和换片播放。

扩展资源操作由 `node tests/explorer-upgrades-check.cjs`、`go test . -run 'TestResumable|TestFullDirectory|TestPersistentThumbnail|TestTransferStreaming'` 和 `go test ./internal/toolbox -run 'TestParallelFile|TestBrowserVerifiedCopy|TestBrowserCopyRejectsChangedSource'` 验证。包括超过旧上限的完整搜索/排序、跨重启分段上传、路径越界和重名保护、缓存淘汰、复制内容校验、源变化和取消保护、跨页预览与通用设置联动。跨盘仍应在实际不同卷上检查。

`TOOLBOX_INTEGRATION=1 go test ./internal/toolbox -run TestVideoTimestampPrecisionIntegration` 使用合成可变帧率视频验证精细时间基准，检查帧数与时长；公开测试不使用私人媒体。

标签长期恢复由 `TestSavedExplorerWorkspaceAcrossServerRestart` 与 UI 扩展测试验证，包括跨端口所需的服务端导航快照、超过旧页数范围的位置、共享范围和凭据字段拒绝。Windows 本机一次 race 运行器在启动内部工具测试前返回 `0xc0000139`，未获得该包的 race 结果；普通测试和确定性并发限制测试另行执行。
