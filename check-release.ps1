[CmdletBinding()]
param([switch]$SourceOnly,[string]$RuntimeDirectory='')
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
$repository=$PSScriptRoot
. (Join-Path $repository 'release\package-support.ps1')
$previousLocation=Get-Location
try {
    Set-Location -LiteralPath $repository
    $null=Get-Command go -ErrorAction Stop
    Assert-ReleaseUIPrerequisites $repository
    Write-Host '[1/4] Go tests and static checks'
    & go test ./... -count=1
    if($LASTEXITCODE -ne 0){throw 'Go tests failed.'}
    & go vet ./...
    if($LASTEXITCODE -ne 0){throw 'Go vet failed.'}
    Write-Host '[2/4] Complete UI regression tests'
    & npm run test:ui
    if($LASTEXITCODE -ne 0){throw 'UI checks failed.'}
    Write-Host '[3/4] Packaging safety and license checks'
    & powershell.exe -NoProfile -ExecutionPolicy Bypass -File (Join-Path $repository 'tests\package-check.ps1')
    if($LASTEXITCODE -ne 0){throw 'Packaging checks failed.'}
    if($SourceOnly){Write-Host '[4/4] SKIPPED runtime checks (-SourceOnly)';Write-Host 'PASS: source checks only. No ZIP, commit, push or release was created.';return}
    Write-Host '[4/4] Pinned runtime integrity and portable self-test'
    if(-not $RuntimeDirectory){$RuntimeDirectory=$repository}
    $runtimeRoot=[IO.Path]::GetFullPath($RuntimeDirectory)
    $runtimeExe=Join-Path $runtimeRoot 'hamster-toolbox.exe'
    if(-not(Test-Path -LiteralPath $runtimeExe -PathType Leaf)){throw 'Runtime executable missing. Build the local program first, or specify -RuntimeDirectory.'}
    $manifestHash=(Get-FileHash -LiteralPath (Join-Path $repository 'tools-manifest.json') -Algorithm SHA256).Hash
    Copy-VerifiedReleaseTools (Join-Path $runtimeRoot 'tools') (Join-Path $runtimeRoot '.verification-no-output') $manifestHash -VerifyOnly
    & $runtimeExe -check-tools
    if($LASTEXITCODE -ne 0){throw 'Runtime component check failed.'}
    & $runtimeExe -self-test
    if($LASTEXITCODE -ne 0){throw 'Runtime media self-test failed.'}
    Write-Host 'PASS: checks completed. No ZIP, commit, push or release was created.'
} finally { Set-Location -LiteralPath $previousLocation }
