[CmdletBinding()]
param(
    [string]$Version = '',
    [switch]$UpdateOnly,
    [switch]$Installer,
    [string]$ISCC = ''
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$repo = $PSScriptRoot
. (Join-Path $repo 'release\package-support.ps1')
. (Join-Path $repo 'release\installer-support.ps1')
if ($Installer) { $ISCC = Resolve-InnoCompiler $ISCC $repo }
if (-not $Version) { $Version = (Get-Content -LiteralPath (Join-Path $repo 'VERSION') -Raw).Trim() }
if ($Version -notmatch '^\d+\.\d+\.\d+(?:[-+][A-Za-z0-9.-]+)?$') { throw 'Version must look like 0.1.0 or 0.1.0-beta.1' }
if ($env:OS -ne 'Windows_NT') { throw 'Run this packaging script on Windows x64.' }
$null = Get-Command go -ErrorAction Stop
Assert-ReleaseUIPrerequisites $repo
[Console]::OutputEncoding = New-Object Text.UTF8Encoding($false)
$OutputEncoding = [Console]::OutputEncoding
$savedLocation = Get-Location
$savedEnv = @{}
foreach ($name in @('GOOS','GOARCH','CGO_ENABLED')) { $savedEnv[$name] = [Environment]::GetEnvironmentVariable($name,'Process') }

try {
    Set-Location -LiteralPath $repo
    $env:GOOS = 'windows'; $env:GOARCH = 'amd64'; $env:CGO_ENABLED = '0'
    Write-Host '[1/6] Checking pinned portable dependencies...'
    & (Join-Path $repo 'prepare-tools.ps1')

    Write-Host '[2/6] Running tests...'
    & go test ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go tests failed. No release was published.' }
    & npm run test:ui
    if ($LASTEXITCODE -ne 0) { throw 'UI tests failed. No release was published.' }
    & powershell.exe -NoProfile -ExecutionPolicy Bypass -File (Join-Path $repo 'tests\package-check.ps1')
    if ($LASTEXITCODE -ne 0) { throw 'Packaging safety checks failed. No release was published.' }

    Assert-ReleaseGoNotices $repo

    $stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
    $buildTime = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')
    $packageName = 'hamster-toolbox-' + $Version + '-windows-x64-' + $stamp
    $stage = Join-Path $repo ('work\package-' + [guid]::NewGuid().ToString('N'))
    $payload = Join-Path $stage $packageName
    New-Item -ItemType Directory -Path $payload -Force | Out-Null
    $manifestHash = (Get-FileHash -LiteralPath (Join-Path $repo 'tools-manifest.json') -Algorithm SHA256).Hash

    Write-Host '[3/6] Building release binary...'
    $exe = Join-Path $payload 'hamster-toolbox.exe'
    $flags = '-s -w -X main.appVersion=' + $Version + ' -X main.buildTime=' + $buildTime + ' -X main.toolsManifestSHA256=' + $manifestHash
    & go build -buildvcs=false -trimpath -ldflags $flags -o $exe .
    if ($LASTEXITCODE -ne 0) { throw 'Build failed.' }
    $desktopExe=Join-Path $payload 'hamster-toolbox-desktop.exe'
    & go build -buildvcs=false -trimpath -ldflags ($flags+' -H windowsgui -X main.desktopMode=true') -o $desktopExe .
    if ($LASTEXITCODE -ne 0) { throw 'Desktop launcher build failed.' }
    # Whitelist only release files; never copy the repository, data, or work.
    Copy-VerifiedReleaseTools (Join-Path $repo 'tools') (Join-Path $payload 'tools') $manifestHash
    foreach ($name in @('start-toolbox.cmd','tools-manifest.json','LICENSE')) { Copy-Item -LiteralPath (Join-Path $repo $name) -Destination $payload }
    foreach ($name in @('QUICKSTART.txt','THIRD-PARTY.txt','GO-DEPENDENCIES.txt')) { Copy-Item -LiteralPath (Join-Path $repo ('release\'+$name)) -Destination $payload }
    $Version | Set-Content -LiteralPath (Join-Path $payload 'VERSION.txt') -Encoding ASCII

    Write-Host '[4/6] Verifying the staged binary and bundled tools...'
    # Test a relocated Unicode path, empty profile, and system-only PATH before
    # publishing. Character codes keep this script compatible with PowerShell 5.
    $testFolder = Join-Path $stage (([string][char]0x4ED3)+([string][char]0x9F20)+' portable test')
    foreach ($path in @($payload,$testFolder)) {
        if (-not [IO.Path]::GetFullPath($path).StartsWith([IO.Path]::GetFullPath($stage)+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)) { throw 'Unsafe acceptance-test path.' }
    }
    Move-ReleaseDirectory $stage $payload $testFolder
    $testExe = Join-Path $testFolder 'hamster-toolbox.exe'
    $profile = Join-Path $stage 'empty-profile'
    $temp = Join-Path $profile 'temp'
    New-Item -ItemType Directory -Path $temp -Force | Out-Null
    $testEnv = @{}
    foreach ($key in @('PATH','APPDATA','LOCALAPPDATA','TEMP','TMP','MAGICK_HOME','MAGICK_CONFIGURE_PATH')) { $testEnv[$key]=[Environment]::GetEnvironmentVariable($key,'Process') }
    try {
        $env:PATH=$env:SystemRoot+'\System32;'+$env:SystemRoot
        $env:APPDATA=$profile; $env:LOCALAPPDATA=$profile; $env:TEMP=$temp; $env:TMP=$temp
        $env:MAGICK_HOME=''; $env:MAGICK_CONFIGURE_PATH=''
        Set-Location -LiteralPath $env:SystemRoot
        $checkText = & $testExe -check-tools
        if ($LASTEXITCODE -ne 0) { throw ('Portable dependency check failed: ' + ($checkText -join "`n")) }
        $check = ($checkText -join "`n") | ConvertFrom-Json
        if (-not $check.Ready) { throw 'Portable dependencies are not ready.' }
        foreach ($dependency in $check.Dependencies) {
            if (-not $dependency.Path.StartsWith((Join-Path $testFolder 'tools')+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)) { throw 'Dependency escaped the portable folder.' }
        }
        & $testExe -self-test
        if ($LASTEXITCODE -ne 0) { throw 'Portable self-test failed. No release was published.' }
        if (@(Get-ChildItem -LiteralPath $profile -File -Recurse).Count -ne 0) { throw 'Self-test unexpectedly saved profile data.' }
    } finally {
        foreach ($key in $testEnv.Keys) { [Environment]::SetEnvironmentVariable($key,$testEnv[$key],'Process') }
        Set-Location -LiteralPath $repo
        Move-ReleaseDirectory $stage $testFolder $payload
    }
    $releaseInfo = @{
        Product='Hamster Toolbox'; Version=$Version; BuiltUTC=$buildTime; Platform='windows/amd64'
        ToolsManifestSHA256=$manifestHash
        ExecutableSHA256=(Get-FileHash -LiteralPath $exe -Algorithm SHA256).Hash
        DesktopExecutableSHA256=(Get-FileHash -LiteralPath $desktopExe -Algorithm SHA256).Hash
        Dependencies=@($check.Dependencies | ForEach-Object { @{Key=$_.Key;Version=$_.Message} })
    }
    $releaseInfo | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath (Join-Path $payload 'release-info.json') -Encoding UTF8

    Write-Host '[5/6] Creating shareable ZIP files...'
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $dist = Join-Path $repo 'dist'
    New-Item -ItemType Directory -Path $dist -Force | Out-Null
    if (-not $UpdateOnly) {
        $zip = Join-Path $stage ($packageName+'.zip')
        [IO.Compression.ZipFile]::CreateFromDirectory($payload,$zip,[IO.Compression.CompressionLevel]::Optimal,$true)
    }
    $update = Join-Path $stage 'update'
    New-Item -ItemType Directory -Path $update | Out-Null
    foreach ($name in @('hamster-toolbox.exe','hamster-toolbox-desktop.exe','start-toolbox.cmd','tools-manifest.json','VERSION.txt','release-info.json','LICENSE','QUICKSTART.txt','THIRD-PARTY.txt','GO-DEPENDENCIES.txt')) { Copy-Item -LiteralPath (Join-Path $payload $name) -Destination $update }
    Copy-Item -LiteralPath (Join-Path $repo 'release\UPDATE.txt') -Destination $update
    $updateZip = Join-Path $stage ($packageName+'-update.zip')
    [IO.Compression.ZipFile]::CreateFromDirectory($update,$updateZip,[IO.Compression.CompressionLevel]::Optimal,$false)
    $artifacts = @($updateZip)
    if (-not $UpdateOnly) { $artifacts = @($zip)+$artifacts }
    if ($Installer) {
        Write-Host 'Building the optional Windows installer...'
        $installerArtifact=Build-HamsterInstaller $repo $payload $Version $stage ($packageName+'-setup') $ISCC
        $artifacts += $installerArtifact
    }
    $published = @()
    foreach ($artifact in $artifacts) {
        $name = [IO.Path]::GetFileName($artifact)
        $destination = Join-Path $dist $name
        if (Test-Path -LiteralPath $destination) { throw ('Release already exists: '+$destination) }
        Move-Item -LiteralPath $artifact -Destination $destination
        $published += $destination
        ((Get-FileHash -LiteralPath $destination -Algorithm SHA256).Hash.ToLowerInvariant()+'  '+$name) | Set-Content -LiteralPath ($destination+'.sha256') -Encoding ASCII
    }

    Write-Host '[6/6] Updating the local executable (previous copy is preserved)...'
    $localExe = Join-Path $repo 'hamster-toolbox.exe'
    if (Test-Path -LiteralPath $localExe) {
        Move-Item -LiteralPath $localExe -Destination (Join-Path $repo ('hamster-toolbox-before-'+[guid]::NewGuid().ToString('N')+'.exe'))
    }
    Copy-Item -LiteralPath $exe -Destination $localExe
    $localDesktop=Join-Path $repo 'hamster-toolbox-desktop.exe'
    if (Test-Path -LiteralPath $localDesktop) { Move-Item -LiteralPath $localDesktop -Destination (Join-Path $repo ('hamster-desktop-before-'+[guid]::NewGuid().ToString('N')+'.exe')) }
    Copy-Item -LiteralPath $desktopExe -Destination $localDesktop
    Write-Host ''
    foreach ($file in $published) { Write-Host ('READY: '+$file) }
    Write-Host 'Full ZIP: new users. Update ZIP: replace program files in an existing full folder.'
} finally {
    foreach ($name in $savedEnv.Keys) { [Environment]::SetEnvironmentVariable($name,$savedEnv[$name],'Process') }
    Set-Location -LiteralPath $savedLocation
}
