[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$repo = $PSScriptRoot
$magick = Join-Path $repo 'tools\imagemagick\magick.exe'
if (-not (Test-Path -LiteralPath $magick)) { $magick = (Get-Command magick -ErrorAction Stop).Source }
$source = Join-Path $repo 'docs\brand\hamster-mascot.png'
$savedLocation = Get-Location
try {
    Set-Location -LiteralPath $repo
    foreach ($asset in @(@('256x256','hamster-logo.png'),@('180x180','apple-touch-icon.png'),@('32x32','favicon-32.png'))) {
        & $magick $source -resize $asset[0] -strip -define png:compression-level=9 (Join-Path $repo ('web\'+$asset[1]))
        if ($LASTEXITCODE -ne 0) { throw 'PNG asset generation failed.' }
    }
    & $magick $source -define icon:auto-resize=256,128,64,48,32,16 -strip web/favicon.ico
    if ($LASTEXITCODE -ne 0) { throw 'ICO generation failed.' }
    foreach ($architecture in @('amd64','arm64')) {
        & go run github.com/akavel/rsrc@v0.10.2 -arch $architecture -ico web/favicon.ico -o ('brand_windows_'+$architecture+'.syso')
        if ($LASTEXITCODE -ne 0) { throw 'Windows icon resource generation failed.' }
    }
} finally { Set-Location -LiteralPath $savedLocation }
