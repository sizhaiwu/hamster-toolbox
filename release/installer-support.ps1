function Resolve-InnoCompiler([string]$Compiler = '', [string]$Repository = '') {
    $candidates = @()
    if ($Compiler) { $candidates = @($Compiler) }
    else {
        $command = Get-Command ISCC.exe -ErrorAction SilentlyContinue
        if ($command) { $candidates += $command.Source }
        foreach ($base in @(${env:ProgramFiles(x86)},$env:ProgramFiles,$env:LOCALAPPDATA)) {
            if ($base) { foreach ($suffix in @('Inno Setup 7\ISCC.exe','Inno Setup 6\ISCC.exe','Programs\Inno Setup 7\ISCC.exe','Programs\Inno Setup 6\ISCC.exe')) { $candidates += Join-Path $base $suffix } }
        }
        if ($Repository) { $candidates += Join-Path $Repository 'work\inno-compiler\ISCC.exe' }
    }
    foreach ($path in $candidates) {
        if (Test-Path -LiteralPath $path -PathType Leaf) {
            $item=Get-Item -LiteralPath $path
            # ISCC itself can carry a 0.0.0.0 file version. The preprocessor
            # script enforces the compiler version instead of guessing here.
            return $item.FullName
        }
    }
    throw 'Inno Setup compiler not found. Get it from https://jrsoftware.org/inno-setup/downloads or specify -ISCC with the full path to ISCC.exe. Portable ZIP packaging does not need it.'
}

function Build-HamsterInstaller([string]$Repository,[string]$Payload,[string]$Version,[string]$OutputDirectory,[string]$OutputName,[string]$Compiler) {
    . (Join-Path $Repository 'release\desktop-bundle-support.ps1')
    $compilerPath=Resolve-InnoCompiler $Compiler
    if ($Version -notmatch '^(\d+)\.(\d+)\.(\d+)(?:[-+][A-Za-z0-9.-]+)?$') { throw 'Invalid installer version.' }
    $numericVersion=$Matches[1]+'.'+$Matches[2]+'.'+$Matches[3]+'.0'
    if ($OutputName -notmatch '^[A-Za-z0-9][A-Za-z0-9._-]+$') { throw 'Invalid installer output name.' }
    $payloadPath=[IO.Path]::GetFullPath($Payload)
    $outputPath=[IO.Path]::GetFullPath($OutputDirectory)
    foreach ($name in @('hamster-toolbox.exe','hamster-toolbox-desktop.exe','start-toolbox.cmd','tools-manifest.json','VERSION.txt','release-info.json','LICENSE','QUICKSTART.txt','THIRD-PARTY.txt','GO-DEPENDENCIES.txt')) {
        $file=Get-Item -LiteralPath (Join-Path $payloadPath $name) -ErrorAction Stop
        if ($file.PSIsContainer -or ($file.Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Installer payload must contain ordinary files.' }
    }
    $manifestHash=(Get-FileHash -LiteralPath (Join-Path $Repository 'tools-manifest.json') -Algorithm SHA256).Hash
    if ((Get-FileHash -LiteralPath (Join-Path $payloadPath 'tools-manifest.json') -Algorithm SHA256).Hash -ne $manifestHash) { throw 'Installer payload manifest mismatch.' }
    Copy-VerifiedReleaseTools (Join-Path $payloadPath 'tools') (Join-Path $outputPath '.verify-no-output') $manifestHash -VerifyOnly
    $artifact=Join-Path $outputPath ($OutputName+'.exe')
    if (Test-Path -LiteralPath $artifact) { throw 'Installer already exists; choose a new output name.' }
    New-Item -ItemType Directory -Path $outputPath -Force | Out-Null
    $arguments=@('/Qp',('/DPayload='+$payloadPath),('/DAppVersion='+$Version),('/DNumericVersion='+$numericVersion),('/DOutputDir='+$outputPath),('/DOutputName='+$OutputName),('/DBrandIcon='+(Join-Path $Repository 'web\favicon.ico')),(Join-Path $Repository 'release\hamster-toolbox.iss'))
    $desktop=Get-Content -LiteralPath (Join-Path $payloadPath 'desktop-client-info.json') -Raw|ConvertFrom-Json
    Test-HamsterDesktopRuntime (Join-Path $payloadPath 'desktop-runtime') $Version $desktop.SHA256
    # Compiler progress belongs to the host, not the function's return value.
    # Returning progress/blank lines alongside the path breaks callers that
    # hash or publish the installer artifact.
    & $compilerPath @arguments | ForEach-Object { Write-Host $_ }
    if ($LASTEXITCODE -ne 0 -or -not(Test-Path -LiteralPath $artifact -PathType Leaf)) { throw 'Installer compilation failed; no installer was published.' }
    return $artifact
}
