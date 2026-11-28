# Packaging helpers are also exercised with disposable fixtures by tests/package-check.ps1.
function New-HamsterPortableArchive([string]$Payload,[string]$Archive,[string]$SevenZip,[switch]$Zip) {
    $source=[IO.Path]::GetFullPath($Payload).TrimEnd('\','/')
    $output=[IO.Path]::GetFullPath($Archive)
    if(Test-Path -LiteralPath $output){throw 'Archive already exists; choose a new output path.'}
    if(-not(Test-Path -LiteralPath $SevenZip -PathType Leaf)){throw 'The verified 7-Zip runtime is required for portable packaging.'}
    $archiveArguments=@('a','-t7z','-mx=9','-m0=LZMA2','-md=128m','-mfb=128','-ms=on','-mmt=2','-bsp0',$output,(Split-Path -Leaf $source))
    if($Zip){$archiveArguments=@('a','-tzip','-mx=9','-mm=Deflate','-mmt=2','-bsp0',$output,(Split-Path -Leaf $source))}
    Push-Location -LiteralPath (Split-Path -Parent $source)
    try {
        & $SevenZip @archiveArguments | ForEach-Object {Write-Host $_}
        if($LASTEXITCODE -ne 0){throw 'Portable archive creation failed.'}
        & $SevenZip t '-bsp0' $output | ForEach-Object {Write-Host $_}
        if($LASTEXITCODE -ne 0){throw 'Portable archive verification failed.'}
    } finally {Pop-Location}
    return $output
}

function Assert-ReleaseUIPrerequisites([string]$Repository) {
    if (-not (Get-Command node -ErrorAction SilentlyContinue) -or -not (Get-Command npm -ErrorAction SilentlyContinue)) {
        throw 'Packaging requires Node.js 20.19+ and npm for UI checks. Recipients do not need Node.js.'
    }
    $nodeVersion = & node --version
    if ($LASTEXITCODE -ne 0 -or $nodeVersion -notmatch '^v(\d+)\.(\d+)\.' -or [int]$Matches[1] -lt 20 -or ([int]$Matches[1] -eq 20 -and [int]$Matches[2] -lt 19)) { throw 'Packaging requires Node.js 20.19 or newer.' }
    if (-not (Test-Path -LiteralPath (Join-Path $Repository 'node_modules\jsdom\package.json') -PathType Leaf)) {
        throw 'UI test dependencies are missing. Run npm ci --ignore-scripts before packaging.'
    }
}

function Assert-ReleaseGoNotices([string]$Repository) {
    $notice = Get-Content -LiteralPath (Join-Path $Repository 'release\GO-DEPENDENCIES.txt') -Raw -Encoding UTF8
    Push-Location -LiteralPath $Repository
    try {
        $modules = & go list -deps -f '{{if not .Standard}}{{with .Module}}{{if not .Main}}{{.Path}} {{.Version}}{{end}}{{end}}{{end}}' .
        if ($LASTEXITCODE -ne 0) { throw 'Could not inspect compiled Go dependencies.' }
        foreach ($module in @($modules | Where-Object { $_ } | Sort-Object -Unique)) {
            if (-not $notice.Contains($module)) { throw ('Update release/GO-DEPENDENCIES.txt for '+$module) }
        }
    } finally { Pop-Location }
}

function Copy-VerifiedReleaseTools([string]$Source, [string]$Destination, [string]$ManifestHash, [switch]$VerifyOnly) {
    $sourceRoot = [IO.Path]::GetFullPath($Source).TrimEnd('\','/')
    $destinationRoot = [IO.Path]::GetFullPath($Destination).TrimEnd('\','/')
    if (-not $VerifyOnly -and (Test-Path -LiteralPath $destinationRoot)) { throw 'Tools destination must be new and empty.' }
    if ((Get-Item -LiteralPath $sourceRoot -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Tools source cannot be a directory link.' }
    $markerBytes = [IO.File]::ReadAllBytes((Join-Path $sourceRoot 'bundle-files.json'))
    $inventory = [Text.Encoding]::UTF8.GetString($markerBytes).TrimStart([char]0xfeff) | ConvertFrom-Json
    if ($inventory.ManifestSHA256 -ne $ManifestHash -or @($inventory.Files).Count -lt 1) { throw 'Tools inventory does not match the release manifest.' }
    $seen = New-Object 'System.Collections.Generic.HashSet[string]' ([StringComparer]::OrdinalIgnoreCase)
    if (-not $VerifyOnly) { New-Item -ItemType Directory -Path $destinationRoot -ErrorAction Stop | Out-Null }
    foreach ($entry in $inventory.Files) {
        $relative = [string]$entry.Path
        if (-not $relative -or [IO.Path]::IsPathRooted($relative) -or $relative -match '(^|[\\/])\.\.?([\\/]|$)|:' -or $entry.SHA256 -notmatch '^[0-9a-fA-F]{64}$' -or $relative -eq 'bundle-files.json' -or -not $seen.Add($relative.Replace('/','\'))) {
            throw 'Invalid or duplicate tools inventory entry.'
        }
        $sourceFile = [IO.Path]::GetFullPath((Join-Path $sourceRoot $relative))
        $targetFile = [IO.Path]::GetFullPath((Join-Path $destinationRoot $relative))
        if (-not $sourceFile.StartsWith($sourceRoot+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase) -or -not $targetFile.StartsWith($destinationRoot+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)) { throw 'Tools path escaped its directory.' }
        $item = Get-Item -LiteralPath $sourceFile -Force -ErrorAction Stop
        if ($item.PSIsContainer) { throw 'Tools inventory must contain files only.' }
        for ($ancestor=$item; $ancestor.FullName -ne $sourceRoot; $ancestor=Get-Item -LiteralPath (Split-Path -Parent $ancestor.FullName) -Force) {
            if ($ancestor.Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Tools inventory cannot traverse a file or directory link.' }
        }
        if ((Get-FileHash -LiteralPath $sourceFile -Algorithm SHA256).Hash -ne $entry.SHA256) { throw ('Tools file changed: '+$relative) }
        if ($VerifyOnly) { continue }
        New-Item -ItemType Directory -Path (Split-Path -Parent $targetFile) -Force -ErrorAction Stop | Out-Null
        Copy-Item -LiteralPath $sourceFile -Destination $targetFile -ErrorAction Stop
        if ((Get-FileHash -LiteralPath $targetFile -Algorithm SHA256).Hash -ne $entry.SHA256) { throw ('Staged tools file failed verification: '+$relative) }
    }
    if (-not $VerifyOnly) { [IO.File]::WriteAllBytes((Join-Path $destinationRoot 'bundle-files.json'),$markerBytes) }
}
