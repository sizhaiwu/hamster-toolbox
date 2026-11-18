[CmdletBinding()]
param([switch]$Refresh)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
$repo = $PSScriptRoot
$manifestPath = Join-Path $repo 'tools-manifest.json'
$manifestHash = (Get-FileHash -LiteralPath $manifestPath -Algorithm SHA256).Hash
$preparationHash = (Get-FileHash -LiteralPath $PSCommandPath -Algorithm SHA256).Hash
$manifest = Get-Content -LiteralPath $manifestPath -Raw -Encoding UTF8 | ConvertFrom-Json
$tools = Join-Path $repo 'tools'
$marker = Join-Path $tools 'bundle-files.json'

function Test-PreparedTools {
    if (-not (Test-Path -LiteralPath $marker -PathType Leaf)) { return $false }
    try {
        $saved = Get-Content -LiteralPath $marker -Raw -Encoding UTF8 | ConvertFrom-Json
        if ($saved.ManifestSHA256 -ne $manifestHash -or $saved.PreparationSHA256 -ne $preparationHash -or @($saved.Files).Count -lt 5) { return $false }
        if (@(Get-ChildItem -LiteralPath $tools -File -Recurse).Count -ne @($saved.Files).Count + 1) { return $false }
        foreach ($entry in $saved.Files) {
            $path = [IO.Path]::GetFullPath((Join-Path $tools $entry.Path))
            if (-not $path.StartsWith($tools + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) { return $false }
            if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { return $false }
            if ((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash -ne $entry.SHA256) { return $false }
        }
        return $true
    } catch { return $false }
}

if (-not $Refresh -and (Test-PreparedTools)) { Write-Host 'Portable dependencies are ready (hashes verified).'; return }
$cache = Join-Path $repo 'work\downloads'
$stage = Join-Path $repo ('work\prepare-tools-' + [guid]::NewGuid().ToString('N'))
$newTools = Join-Path $stage 'tools'
New-Item -ItemType Directory -Path $cache,$newTools -Force | Out-Null
$tar = (Get-Command tar.exe -ErrorAction Stop).Source

function Get-VerifiedDownload($asset) {
    if ($asset.File -ne [IO.Path]::GetFileName($asset.File) -or $asset.SHA256 -notmatch '^[0-9a-fA-F]{64}$') { throw 'Invalid asset manifest entry.' }
    $path = Join-Path $cache $asset.File
    if ((Test-Path -LiteralPath $path -PathType Leaf) -and (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash -eq $asset.SHA256) { return $path }
    $temp = $path + '.' + [guid]::NewGuid().ToString('N') + '.partial'
    Write-Host ('Downloading ' + $asset.Name + ' ' + $asset.Version + ' ...')
    $done = $false
    for ($attempt = 1; $attempt -le 3; $attempt++) {
        try {
            $oldProgress = $ProgressPreference; $ProgressPreference = 'SilentlyContinue'
            try { Invoke-WebRequest -Uri $asset.URL -OutFile $temp -UseBasicParsing -TimeoutSec 600 } finally { $ProgressPreference = $oldProgress }
            if ((Get-FileHash -LiteralPath $temp -Algorithm SHA256).Hash -ne $asset.SHA256) { throw ('SHA256 mismatch: ' + $asset.Name) }
            Move-Item -LiteralPath $temp -Destination $path -Force
            $done = $true; break
        } catch { if ($attempt -eq 3) { throw }; Write-Host ('Retrying: ' + $_.Exception.Message) }
    }
    if (-not $done) { throw ('Download failed: ' + $asset.Name) }
    return $path
}

function Export-OpenMPRuntime($source, $destination) {
    # Extract Microsoft's signed redistributable as data. Never run the installer
    # or change system-wide runtimes. Burn containers contain standard CAB files.
    $runtimeStage = Join-Path $stage 'runtime-containers'
    New-Item -ItemType Directory -Path $runtimeStage -Force | Out-Null
    $bytes = [IO.File]::ReadAllBytes($source)
    $containers = @()
    for ($offset=0; $offset -lt $bytes.Length-36; $offset++) {
        if ($bytes[$offset] -eq 77 -and $bytes[$offset+1] -eq 83 -and $bytes[$offset+2] -eq 67 -and $bytes[$offset+3] -eq 70) {
            $size = [BitConverter]::ToUInt32($bytes,$offset+8)
            if ($size -ge 36 -and [long]$offset+$size -le $bytes.Length) {
                $cab = Join-Path $runtimeStage ('container-'+$containers.Count+'.cab')
                $piece = New-Object byte[] $size
                [Array]::Copy($bytes,$offset,$piece,0,$size)
                [IO.File]::WriteAllBytes($cab,$piece)
                $containers += $cab; $offset += $size-1
            }
        }
    }
    if ($containers.Count -ne 2) { throw 'Unexpected Microsoft redistributable layout.' }
    $expand = Join-Path $env:SystemRoot 'System32\expand.exe'
    $extracted = @()
    foreach ($cab in $containers) {
        $folder = Join-Path $runtimeStage ([IO.Path]::GetFileNameWithoutExtension($cab))
        New-Item -ItemType Directory -Path $folder | Out-Null
        & $expand '-F:*' $cab $folder | Out-Null
        if ($LASTEXITCODE -ne 0) { throw 'Microsoft CAB extraction failed.' }
        $extracted += $folder
    }
    $found = $false
    foreach ($file in Get-ChildItem -LiteralPath $extracted[1] -File) {
        $head = [IO.File]::ReadAllBytes($file.FullName)
        if ($head.Length -lt 4 -or [Text.Encoding]::ASCII.GetString($head,0,4) -ne 'MSCF') { continue }
        $listing = & $expand -D $file.FullName
        if ($listing -match 'vcomp140\.dll_amd64') {
            $dllDir = Join-Path $runtimeStage 'openmp'
            New-Item -ItemType Directory -Path $dllDir | Out-Null
            & $expand '-F:vcomp140.dll_amd64' $file.FullName $dllDir | Out-Null
            if ($LASTEXITCODE -ne 0) { throw 'OpenMP extraction failed.' }
            Copy-Item -LiteralPath (Join-Path $dllDir 'vcomp140.dll_amd64') -Destination (Join-Path $destination 'vcomp140.dll')
            $found = $true; break
        }
    }
    if (-not $found) { throw 'Microsoft x64 OpenMP runtime is missing.' }
    [xml]$burn = [IO.File]::ReadAllText((Join-Path $extracted[0] '0'))
    foreach ($payload in $burn.SelectNodes("//*[local-name()='Payload']")) {
        $licenseName = $payload.GetAttribute('FilePath')
        if ($licenseName -eq 'license.rtf' -or $licenseName -eq '2052\license.rtf') {
            $outputName = 'Microsoft-VCRuntime-LICENSE.rtf'
            if ($licenseName -ne 'license.rtf') { $outputName = 'Microsoft-VCRuntime-LICENSE-zh-CN.rtf' }
            Copy-Item -LiteralPath (Join-Path $extracted[0] $payload.GetAttribute('SourcePath')) -Destination (Join-Path $destination $outputName)
        }
    }
}

foreach ($asset in $manifest.Assets) {
    $archive = Get-VerifiedDownload $asset
    $dest = Join-Path $newTools $asset.Name
    if ($asset.Kind -eq 'font' -or $asset.Kind -eq 'font-license') {
        $fontDir = Join-Path $newTools 'fonts'
        New-Item -ItemType Directory -Path $fontDir -Force | Out-Null
        $name = $asset.File; if ($asset.Kind -eq 'font-license') { $name = 'OFL.txt' }
        Copy-Item -LiteralPath $archive -Destination (Join-Path $fontDir $name)
        continue
    }
    if ($asset.Kind -eq 'vcruntime') {
        Export-OpenMPRuntime $archive (Join-Path $newTools 'imagemagick')
        continue
    }
    $extract = Join-Path $stage ('extract-' + $asset.Name)
    New-Item -ItemType Directory -Path $extract,$dest -Force | Out-Null
    if ($asset.Kind -eq 'sevenzip-full') {
        $dest = Join-Path $newTools 'sevenzip'
        $bootstrap = Join-Path $dest '7za.exe'
        if (-not (Test-Path -LiteralPath $bootstrap -PathType Leaf)) { throw 'Prepare 7-Zip Extra before its full runtime.' }
        & $bootstrap x $archive ('-o'+$extract) -y -bso0
    } else { & $tar -xf $archive -C $extract }
    if ($LASTEXITCODE -ne 0) { throw ('Extraction failed: ' + $asset.Name) }
    switch ($asset.Kind) {
        'ffmpeg' {
            $ff = @(Get-ChildItem -LiteralPath $extract -Filter ffmpeg.exe -File -Recurse)
            if ($ff.Count -ne 1) { throw 'Expected one FFmpeg executable in archive.' }
            $upstreamRoot = Split-Path (Split-Path $ff[0].FullName -Parent) -Parent
            New-Item -ItemType Directory -Path (Join-Path $dest 'bin') | Out-Null
            foreach ($name in @('ffmpeg.exe','ffprobe.exe')) { Copy-Item -LiteralPath (Join-Path (Split-Path $ff[0].FullName -Parent) $name) -Destination (Join-Path $dest 'bin') }
            $notices = @(Get-ChildItem -LiteralPath $upstreamRoot -File | Where-Object { $_.Name -match '^(LICENSE|COPYING|README)' })
            if ($notices.Count -lt 1) { throw 'FFmpeg license/readme files are missing.' }
            foreach ($file in $notices) { Copy-Item -LiteralPath $file.FullName -Destination $dest }
        }
        'imagemagick' {
            $im = @(Get-ChildItem -LiteralPath $extract -Filter magick.exe -File -Recurse)
            if ($im.Count -ne 1) { throw 'Expected one ImageMagick executable in archive.' }
            $unusedFrontends = @('compare.exe','composite.exe','conjure.exe','identify.exe','mogrify.exe','montage.exe','stream.exe')
            foreach ($child in Get-ChildItem -LiteralPath (Split-Path $im[0].FullName -Parent) -Force) {
                # magick.exe contains these subcommands; duplicate standalone
                # frontends are not used by this application.
                if ($unusedFrontends -contains $child.Name) { continue }
                Copy-Item -LiteralPath $child.FullName -Destination $dest -Recurse
            }
        }
        'sevenzip' {
            $console = Join-Path $extract 'x64\7za.exe'
            if (-not (Test-Path -LiteralPath $console -PathType Leaf)) { throw '7-Zip x64 console is missing.' }
            Copy-Item -LiteralPath $console -Destination (Join-Path $dest '7za.exe')
            $licenses = @(Get-ChildItem -LiteralPath $extract -File | Where-Object { $_.Name -match '(?i)(license|copying|readme|history)' })
            if ($licenses.Count -lt 1) { throw '7-Zip license files are missing.' }
            foreach ($file in $licenses) {
                $noticeName = '7za-'+$file.Name
                Copy-Item -LiteralPath $file.FullName -Destination (Join-Path $dest $noticeName)
            }
        }
        'sevenzip-full' {
            foreach ($name in @('7z.exe','7z.dll','License.txt','readme.txt','History.txt')) {
                $file = Join-Path $extract $name
                if (-not (Test-Path -LiteralPath $file -PathType Leaf)) { throw ('Missing full 7-Zip file: '+$name) }
                Copy-Item -LiteralPath $file -Destination (Join-Path $dest $name) -Force
            }
        }
        'exiftool' {
            $et = @(Get-ChildItem -LiteralPath $extract -Filter 'exiftool*.exe' -File -Recurse)
            if ($et.Count -ne 1) { throw 'Expected one ExifTool executable in archive.' }
            foreach ($child in Get-ChildItem -LiteralPath (Split-Path $et[0].FullName -Parent) -Force) { Copy-Item -LiteralPath $child.FullName -Destination $dest -Recurse }
            if ($et[0].Name -ne 'exiftool.exe') { Move-Item -LiteralPath (Join-Path $dest $et[0].Name) -Destination (Join-Path $dest 'exiftool.exe') }
        }
        default { throw ('Unknown asset kind: ' + $asset.Kind) }
    }
}

$files = @(Get-ChildItem -LiteralPath $newTools -File -Recurse | Sort-Object FullName | ForEach-Object {
    @{ Path = $_.FullName.Substring($newTools.Length + 1).Replace('\','/'); SHA256 = (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash }
})
@{ManifestSHA256=$manifestHash; PreparationSHA256=$preparationHash; Files=$files} | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $newTools 'bundle-files.json') -Encoding UTF8

# Only move resolved paths inside this repository; preserve previous tools.
foreach ($path in @($tools,$newTools)) {
    if (-not [IO.Path]::GetFullPath($path).StartsWith($repo+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)) { throw 'Unsafe tools path.' }
}
if (Test-Path -LiteralPath $tools) { Move-Item -LiteralPath $tools -Destination (Join-Path $repo ('work\tools-backup-'+[guid]::NewGuid().ToString('N'))) }
Move-Item -LiteralPath $newTools -Destination $tools
Write-Host 'Portable dependencies prepared. Archives are cached under work/downloads.'
