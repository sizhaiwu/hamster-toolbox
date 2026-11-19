# Developer-only: regenerate synthetic media with the repository's bundled tools.
[CmdletBinding()]
param([string]$OutputDirectory = (Join-Path $PSScriptRoot 'assets'))
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$repo = Split-Path $PSScriptRoot -Parent
$assets = [IO.Path]::GetFullPath($OutputDirectory)
if (Test-Path -LiteralPath $assets) { throw 'Assets already exist. Use -OutputDirectory with a new directory; existing files are never deleted.' }
$magick = Join-Path $repo 'tools\imagemagick\magick.exe'
$ffmpeg = Join-Path $repo 'tools\ffmpeg\bin\ffmpeg.exe'
$exiftool = Join-Path $repo 'tools\exiftool\exiftool.exe'
$sevenzip = Join-Path $repo 'tools\sevenzip\7z.exe'
foreach ($program in @($magick,$ffmpeg,$exiftool,$sevenzip)) { if (-not (Test-Path -LiteralPath $program)) { throw ('Missing bundled program: '+$program) } }
$null = New-Item -ItemType Directory -Path $assets
function Invoke-Checked([string]$Program, [string[]]$Arguments) {
    & $Program @Arguments
    if ($LASTEXITCODE -ne 0) { throw ('Asset generation failed: '+[IO.Path]::GetFileName($Program)) }
}
$jpg = Join-Path $assets 'color-card.jpg'
$png = Join-Path $assets 'transparent-card.png'
$webp = Join-Path $assets 'color-card.webp'
$video = Join-Path $assets 'motion-demo.mp4'
Invoke-Checked $magick @('-size','960x540','gradient:#18344f-#e5b67c','-fill','#57c9bb','-draw','roundrectangle 60,60 420,380 35,35','-fill','#f47b67','-draw','circle 700,240 850,240','-fill','white','-font','Arial','-pointsize','44','-annotate','+90+145','HAMSTER TOOLBOX','-pointsize','26','-annotate','+90+200','SYNTHETIC DEMO','-quality','100',$jpg)
Invoke-Checked $magick @('-size','960x540','xc:none','-fill','#4c97dcaa','-draw','roundrectangle 30,30 700,480 45,45','-fill','#e9925dcc','-draw','circle 680,260 900,260','-fill','white','-font','Arial','-pointsize','42','-annotate','+70+130','PNG / ALPHA DEMO','-define','png:compression-level=0',$png)
Invoke-Checked $magick @($jpg,'-define','webp:lossless=true',$webp)
Invoke-Checked $magick @($jpg,'-quality','78',(Join-Path $assets 'color-card-near.jpg'))
Invoke-Checked $exiftool @('-overwrite_original','-Artist=Hamster Demo','-Comment=Synthetic metadata for cleanup practice',$jpg)
Invoke-Checked $ffmpeg @('-v','error','-nostdin','-n','-f','lavfi','-i','testsrc2=size=640x360:rate=24','-f','lavfi','-i','sine=frequency=440:sample_rate=48000','-t','5','-c:v','libx264','-preset','medium','-crf','1','-pix_fmt','yuv420p','-c:a','aac','-b:a','96k','-metadata','title=Hamster Demo Title','-metadata','comment=Synthetic video for toolbox practice','-movflags','+faststart',$video)
Invoke-Checked $ffmpeg @('-v','error','-nostdin','-n','-i',$video,'-map','0','-c','copy','-metadata','title=Hamster Demo MKV Title',(Join-Path $assets 'metadata-demo.mkv'))
Invoke-Checked $ffmpeg @('-v','error','-nostdin','-n','-f','lavfi','-i','sine=frequency=440:sample_rate=22050','-t','1','-c:a','pcm_s16le','-metadata','artist=Hamster Demo',(Join-Path $assets 'tone-demo.wav'))
[IO.File]::WriteAllText((Join-Path $assets 'note.txt'),"Hamster Toolbox synthetic sample.`r`nThis is public demo text, not a real resource or a private document.`r`n",[Text.UTF8Encoding]::new($false))
Push-Location -LiteralPath $assets
try {
    Invoke-Checked $sevenzip @('a','-tzip','-mx=0','plain.zip','note.txt','color-card.jpg')
    Invoke-Checked $sevenzip @('a','-tzip','-mx=0','second.zip','note.txt')
    Invoke-Checked $sevenzip @('a','-t7z','-mx=0','-phamster-demo','-mhe=on','encrypted.7z','note.txt')
    Invoke-Checked $sevenzip @('a','-t7z','-mx=0','-v64k','-phamster-demo','-mhe=on','split.7z','note.txt','color-card.jpg')
} finally { Pop-Location }
$files = @(Get-ChildItem -LiteralPath $assets -File | Sort-Object Name | ForEach-Object { @{Name=$_.Name; Bytes=$_.Length; SHA256=(Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant()} })
@{Origin='Synthetic images, test pattern video, sine-wave audio and demo archives generated for Hamster Toolbox'; License='MIT'; PublicDemoPassword='hamster-demo'; Files=$files} | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $assets 'manifest.json') -Encoding UTF8
Write-Output ('Generated '+$files.Count+' synthetic assets.')
