# 计划任务导入体验

在工具箱“计划任务 → 导入”选择 [baota-example.json](baota-example.json)。这是按宝塔公开 JSON 字段手工生成的合成示例，不含真实服务器、密码或私人路径。

会显示 Shell、GET、POST 三个可导入任务；确认导入后全部暂停。两个请求地址使用 `example.invalid`，需要替换成自己的地址；Shell 示例只输出日期和文字，Windows 需自行准备 bash，或导入后改成 PowerShell 脚本。

完整限制和导出差异见[计划任务说明](../../docs/schedules.md#批量操作与导入导出)。
