# 仓鼠形象与品牌素材

一只开心地抱着资源文件夹的动漫仓鼠。圆脸、金白色毛发和粉色小耳朵表达可爱，文件夹里的图片卡片与播放图标表达“喜欢收集和整理资源”。标志不含文字，透明背景，适配工具箱、资源管理器、共享登录页和应用图标。

- 原始生成图：[hamster-mascot.png](hamster-mascot.png)，1254×1254，保留透明通道。
- 界面与 README：[hamster-logo.png](../../web/hamster-logo.png)，256×256。
- 浏览器：[favicon-32.png](../../web/favicon-32.png)、[favicon.ico](../../web/favicon.ico)。
- 移动端主屏幕：[apple-touch-icon.png](../../web/apple-touch-icon.png)，180×180。
- Windows 程序图标：ICO 含 16、32、48、64、128、256 六种尺寸，`brand_windows_amd64.syso` 和 `brand_windows_arm64.syso` 随源码提供，普通 `go build` 自动嵌入，无需用户安装图像工具。

## 重新生成技术尺寸

修改源图后，在仓库根目录运行 `./generate-brand.ps1`。它使用随包或系统 ImageMagick 生成尺寸变体，使用固定版本的 [rsrc v0.10.2](https://github.com/akavel/rsrc/releases/tag/v0.10.2) 生成 Windows 图标资源。首次调用构建工具需要联网；不更改 Go 项目的运行依赖。不运行此脚本也可直接构建现有标志。
