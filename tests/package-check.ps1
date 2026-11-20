$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$repository = Split-Path -Parent $PSScriptRoot
. (Join-Path $repository 'release\package-support.ps1')
. (Join-Path $repository 'release\installer-support.ps1')
$testRoot = Join-Path ([IO.Path]::GetTempPath()) ('hamster-package-check-'+[guid]::NewGuid().ToString('N'))
$testRoot = [IO.Path]::GetFullPath($testRoot)
New-Item -ItemType Directory -Path $testRoot | Out-Null
function Expect-Failure([scriptblock]$Action, [string]$Message) {
    $failed = $false
    try { & $Action } catch { $failed = $true }
    if (-not $failed) { throw ('Expected failure: '+$Message) }
}
try {
    $source = Join-Path $testRoot 'tools'
    New-Item -ItemType Directory -Path (Join-Path $source 'bin') | Out-Null
    [IO.File]::WriteAllText((Join-Path $source 'bin\demo.exe'),'synthetic runtime')
    [IO.File]::WriteAllText((Join-Path $source 'personal-note.txt'),'must never be distributed')
    $manifest = 'a'*64
    $entry = @{Path='bin/demo.exe';SHA256=(Get-FileHash -LiteralPath (Join-Path $source 'bin\demo.exe') -Algorithm SHA256).Hash}
    function Save-Inventory($Entries, [string]$Hash=$manifest) {
        @{ManifestSHA256=$Hash;Files=@($Entries)} | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $source 'bundle-files.json') -Encoding UTF8
    }
    Save-Inventory @($entry)
    Copy-VerifiedReleaseTools $source (Join-Path $testRoot 'verify-only') $manifest -VerifyOnly
    if (Test-Path -LiteralPath (Join-Path $testRoot 'verify-only')) { throw 'Verification mode created a payload.' }
    $destination = Join-Path $testRoot 'verified'
    Copy-VerifiedReleaseTools $source $destination $manifest
    if ((Get-ChildItem -LiteralPath $destination -File -Recurse).Count -ne 2 -or (Test-Path -LiteralPath (Join-Path $destination 'personal-note.txt'))) { throw 'Unlisted files entered the payload.' }
    Expect-Failure { Copy-VerifiedReleaseTools $source $destination $manifest } 'existing destination'
    Expect-Failure { Copy-VerifiedReleaseTools $source (Join-Path $testRoot 'wrong-manifest') ('b'*64) } 'manifest mismatch'
    [IO.File]::WriteAllText((Join-Path $source 'bin\demo.exe'),'modified runtime')
    Expect-Failure { Copy-VerifiedReleaseTools $source (Join-Path $testRoot 'corrupt') $manifest } 'modified runtime'
    Save-Inventory @(@{Path='../outside.txt';SHA256='a'*64})
    Expect-Failure { Copy-VerifiedReleaseTools $source (Join-Path $testRoot 'escape') $manifest } 'parent traversal'
    Save-Inventory @(@{Path='bin/demo.exe:secret';SHA256='a'*64})
    Expect-Failure { Copy-VerifiedReleaseTools $source (Join-Path $testRoot 'ads') $manifest } 'alternate data stream'
    $entry.SHA256=(Get-FileHash -LiteralPath (Join-Path $source 'bin\demo.exe') -Algorithm SHA256).Hash
    Save-Inventory @($entry,@{Path='BIN/DEMO.EXE';SHA256=$entry.SHA256})
    Expect-Failure { Copy-VerifiedReleaseTools $source (Join-Path $testRoot 'duplicate') $manifest } 'case-insensitive duplicate'
    Expect-Failure { Assert-ReleaseUIPrerequisites $testRoot } 'missing UI test dependencies'
    Assert-ReleaseUIPrerequisites $repository
    Assert-ReleaseGoNotices $repository
    # Parse the actual entry point without invoking it or generating packages.
    $parseTokens=$null; $parseErrors=$null
    [void][Management.Automation.Language.Parser]::ParseFile((Join-Path $repository 'package.ps1'),[ref]$parseTokens,[ref]$parseErrors)
    if ($parseErrors.Count -ne 0) { throw 'package.ps1 has syntax errors.' }
    [void][Management.Automation.Language.Parser]::ParseFile((Join-Path $repository 'release\installer-support.ps1'),[ref]$parseTokens,[ref]$parseErrors)
    if ($parseErrors.Count -ne 0) { throw 'installer-support.ps1 has syntax errors.' }
    $installer=Get-Content -LiteralPath (Join-Path $repository 'release\hamster-toolbox.iss') -Raw -Encoding UTF8
    foreach ($required in @('PrivilegesRequired=lowest','AppMutex=Local\HamsterToolbox.Running','CloseApplications=no','hamster-toolbox-desktop.exe','{autodesktop}')) {if(-not $installer.Contains($required)){throw ('Installer safety setting missing: '+$required)}}
    if ($installer -match '(?im)^\[UninstallDelete\]|^\[Registry\]|^PrivilegesRequired=admin') {throw 'Installer must preserve user data and avoid unconditional registry changes.'}
    foreach ($required in @('Name: "autostart";', 'Flags: unchecked', 'RegWriteStringValue(HKCU, StartupRunKey, StartupValue', 'else if OwnStartupEntry then', '(CurUninstallStep = usUninstall) and OwnStartupEntry', '-tray -no-open')) {if(-not $installer.Contains($required)){throw ('Optional current-user startup safety missing: '+$required)}}
    # The compiler can print blank/progress lines. They must never become
    # artifact paths when the caller captures the function's result.
    $installerPayload=Join-Path $testRoot 'installer-payload'
    New-Item -ItemType Directory -Path $installerPayload | Out-Null
    foreach ($name in @('hamster-toolbox.exe','hamster-toolbox-desktop.exe','start-toolbox.cmd','VERSION.txt','release-info.json','LICENSE','QUICKSTART.txt','THIRD-PARTY.txt','GO-DEPENDENCIES.txt')) {
        [IO.File]::WriteAllText((Join-Path $installerPayload $name),'synthetic installer fixture')
    }
    Copy-Item -LiteralPath (Join-Path $repository 'tools-manifest.json') -Destination $installerPayload
    $realManifest=(Get-FileHash -LiteralPath (Join-Path $repository 'tools-manifest.json') -Algorithm SHA256).Hash
    Save-Inventory @($entry) $realManifest
    Copy-VerifiedReleaseTools $source (Join-Path $installerPayload 'tools') $realManifest
    $compilerStub=Join-Path $testRoot 'compiler-stub.ps1'
    @'
param([Parameter(ValueFromRemainingArguments=$true)][string[]]$CompilerArguments)
Write-Output ''
Write-Output 'Synthetic compiler progress'
$outputDir=($CompilerArguments | Where-Object { $_.StartsWith('/DOutputDir=') }).Substring(12)
$outputName=($CompilerArguments | Where-Object { $_.StartsWith('/DOutputName=') }).Substring(13)
[IO.File]::WriteAllText((Join-Path $outputDir ($outputName+'.exe')),'synthetic installer artifact')
$global:LASTEXITCODE=0
'@ | Set-Content -LiteralPath $compilerStub -Encoding UTF8
    $installerOutput=Join-Path $testRoot 'installer-output'
    $built=@(Build-HamsterInstaller $repository $installerPayload '0.2.2-dev' $installerOutput 'fixture-setup' $compilerStub)
    if ($built.Count -ne 1 -or $built[0] -ne (Join-Path $installerOutput 'fixture-setup.exe')) {throw 'Compiler progress polluted the returned installer path.'}
    Write-Output 'PASS: release UI prerequisites, whitelist copy, hashes, traversal, ADS, duplicate and destination protection; packaging entry point parses'
} finally {
    $tempRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\','/')+[IO.Path]::DirectorySeparatorChar
    if (-not $testRoot.StartsWith($tempRoot,[StringComparison]::OrdinalIgnoreCase) -or [IO.Path]::GetFileName($testRoot) -notmatch '^hamster-package-check-[0-9a-f]{32}$') { throw 'Unsafe test cleanup path.' }
    Remove-Item -LiteralPath $testRoot -Recurse -Force
}
