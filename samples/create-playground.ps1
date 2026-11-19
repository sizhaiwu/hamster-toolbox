[CmdletBinding()]
param(
    [string]$OutputParent = [IO.Path]::GetTempPath(),
    [switch]$NoOpen,
    [switch]$Json
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$repo = [IO.Path]::GetFullPath((Split-Path $PSScriptRoot -Parent))
$assets = Join-Path $PSScriptRoot 'assets'
$manifest = Get-Content -LiteralPath (Join-Path $assets 'manifest.json') -Encoding UTF8 -Raw | ConvertFrom-Json
$scenarios = Get-Content -LiteralPath (Join-Path $PSScriptRoot 'scenarios.json') -Encoding UTF8 -Raw | ConvertFrom-Json
$parent = [IO.Path]::GetFullPath($OutputParent)
$workspace = Join-Path $parent ('hamster-demo-' + [guid]::NewGuid().ToString('N').Substring(0,8))

function Assert-Under([string]$Root, [string]$Path) {
    $rootPrefix = [IO.Path]::GetFullPath($Root).TrimEnd([IO.Path]::DirectorySeparatorChar) + [IO.Path]::DirectorySeparatorChar
    $fullPath = [IO.Path]::GetFullPath($Path)
    if (-not $fullPath.StartsWith($rootPrefix,[StringComparison]::OrdinalIgnoreCase)) { throw ('路径超出体验目录：'+$Path) }
    return $fullPath
}
function Write-UTF8([string]$Path, [string]$Text) {
    [IO.File]::WriteAllText($Path,$Text,[Text.UTF8Encoding]::new($true))
}
function Get-AssetSHA256([string]$Path) {
    $stream = [IO.File]::OpenRead($Path)
    $sha = [Security.Cryptography.SHA256]::Create()
    try { return [BitConverter]::ToString($sha.ComputeHash($stream)).Replace('-','').ToLowerInvariant() }
    finally { $sha.Dispose(); $stream.Dispose() }
}
$repoPrefix = $repo.TrimEnd([IO.Path]::DirectorySeparatorChar) + [IO.Path]::DirectorySeparatorChar
if ($workspace.StartsWith($repoPrefix,[StringComparison]::OrdinalIgnoreCase)) { throw '体验目录必须放在仓库外，工具箱会保护自己的程序目录。' }
# Never follow a junction/symlink in the chosen output parent.
$walk = $parent
while ($walk) {
    if (Test-Path -LiteralPath $walk) {
        $entry = Get-Item -LiteralPath $walk -Force
        if (($entry.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw '体验输出路径不能经过链接或目录联接。' }
    }
    $next = Split-Path $walk -Parent
    if ($next -eq $walk) { break }
    $walk = $next
}
$assetMap = @{}
foreach ($asset in $manifest.Files) {
    $source = Assert-Under $assets (Join-Path $assets $asset.Name)
    $entry = Get-Item -LiteralPath $source -Force
    if ($entry.PSIsContainer -or ($entry.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw ('无效素材：'+$asset.Name) }
    if ((Get-AssetSHA256 $source) -ne $asset.SHA256) { throw ('素材校验失败：'+$asset.Name) }
    $assetMap[$asset.Name] = $source
}
if (Test-Path -LiteralPath $workspace) { throw '体验目录已存在，请重新运行生成新目录。' }
$null = [IO.Directory]::CreateDirectory($workspace)
$guide = [Collections.Generic.List[string]]::new()
$guide.Add('仓鼠工具箱体验素材')
$guide.Add('')
$guide.Add('本次目录：'+$workspace)
$guide.Add('这些是合成的色卡、透明图、5秒测试视频、1秒提示音和演示压缩包，不含个人资源。提示音/视频包含测试音，试听前请调低音量。')
$guide.Add('每项工具使用自己的 input 目录；不要把整个体验目录作为输入。输出路径留空即可。先扫描预览，再执行。')
$guide.Add('文件去重需要另行安装外部软件；其他工具需要工具箱完整版或已配置对应组件。')
$guide.Add('素材仅供功能体验，不是画质、速度或压缩率基准。所有压缩包公开演示密码：hamster-demo。')
$guide.Add('再次双击 try-samples.cmd 会创建另一份，不会覆盖或清理本次体验目录。临时目录可能被系统清理，需要长期保存时请复制到自己的资源目录。')
$guide.Add('')
$results = @()
foreach ($scenario in $scenarios) {
    $scenarioRoot = Assert-Under $workspace (Join-Path $workspace $scenario.Directory)
    $input = Join-Path $scenarioRoot 'input'
    $null = [IO.Directory]::CreateDirectory($input)
    $files = @($scenario.Files)
    if ($scenario.PSObject.Properties['IncludeSplitVolumes'] -and $scenario.IncludeSplitVolumes) {
        foreach ($name in @($assetMap.Keys | Where-Object { $_ -match '^split\.7z\.\d+$' -and $_ -ne 'split.7z.001' } | Sort-Object)) {
            $files += [pscustomobject]@{Asset=$name; Path=('input/'+$name)}
        }
    }
    foreach ($file in $files) {
        if (-not $assetMap.ContainsKey($file.Asset)) { throw ('未找到素材：'+$file.Asset) }
        $target = Assert-Under $scenarioRoot (Join-Path $scenarioRoot $file.Path)
        $null = [IO.Directory]::CreateDirectory((Split-Path $target -Parent))
        if (Test-Path -LiteralPath $target) { throw ('不覆盖已有体验文件：'+$target) }
        [IO.File]::Copy($assetMap[$file.Asset],$target,$false)
    }
    $config = @{Tool=$scenario.Tool; Root=$input}
    foreach ($property in $scenario.Settings.PSObject.Properties) { $config[$property.Name]=$property.Value }
    if ($scenario.PSObject.Properties['Extras']) { $config.ArchiveExtras=Assert-Under $scenarioRoot (Join-Path $scenarioRoot $scenario.Extras) }
    $configFile = Join-Path $scenarioRoot 'job.json'
    if ($scenario.Tool -ne 'dedupe') { Write-UTF8 $configFile ($config | ConvertTo-Json -Depth 6) }
    $section = @($scenario.Title,('输入目录：'+$input),('设置：'+$scenario.Hint),('预期结果：'+$scenario.Expected))
    if ($scenario.Tool -ne 'dedupe') { $section += 'CLI配置：'+$configFile+'（终端用 cli '+$scenario.Tool+' --config 配置文件路径；默认仅预览）' }
    Write-UTF8 (Join-Path $scenarioRoot '体验说明.txt') ($section -join "`r`n")
    foreach ($line in $section) { $guide.Add($line) }
    $guide.Add('')
    $results += [pscustomobject]@{Tool=$scenario.Tool; Input=$input; Config=$(if($scenario.Tool -ne 'dedupe'){$configFile}else{''}); Hint=$scenario.Hint; Expected=$scenario.Expected}
}
Copy-Item -LiteralPath (Join-Path $repo 'LICENSE') -Destination (Join-Path $workspace 'LICENSE.txt')
$guidePath = Join-Path $workspace 'START-HERE.txt'
Write-UTF8 $guidePath ($guide -join "`r`n")
Write-UTF8 (Join-Path $workspace 'scenarios.json') (@{Workspace=$workspace; Scenarios=$results} | ConvertTo-Json -Depth 6)
if ($Json) { @{Workspace=$workspace; Guide=$guidePath; Scenarios=$results} | ConvertTo-Json -Depth 6 }
else { Write-Output ('已创建体验素材：'+$workspace); Write-Output ('操作说明：'+$guidePath) }
if (-not $NoOpen) {
    Start-Process -FilePath explorer.exe -ArgumentList ('"'+$workspace+'"')
    Start-Process -FilePath notepad.exe -ArgumentList ('"'+$guidePath+'"')
}
