[CmdletBinding()]
param([string]$ISCC='')
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
$repository=Split-Path -Parent $PSScriptRoot
. (Join-Path $repository 'release\installer-support.ps1')
$compiler=Resolve-InnoCompiler $ISCC $repository
$id=[guid]::NewGuid().ToString('N')
$qa=Join-Path ([IO.Path]::GetTempPath()) ('hamster-autostart-qa-'+$id)
$registryPath='Software\HamsterToolbox\AutostartInstallerTests-'+$id
$target=Join-Path $qa 'installed test'
$uninstaller=Join-Path $target 'unins000.exe'
New-Item -ItemType Directory -Path $qa | Out-Null
function Read-Startup {
    $key=[Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($registryPath)
    if($null -eq $key){return ''}
    try{return [string]$key.GetValue('HamsterToolbox','')}finally{$key.Dispose()}
}
function Write-Startup([string]$Value) {
    $key=[Microsoft.Win32.Registry]::CurrentUser.CreateSubKey($registryPath)
    try{$key.SetValue('HamsterToolbox',$Value)}finally{$key.Dispose()}
}
function Run-Installer([string]$Task='default') {
    $arguments=@('/CURRENTUSER','/VERYSILENT','/SUPPRESSMSGBOXES','/NORESTART','/NOICONS',('/DIR="'+$target+'"'))
    if($Task -ne 'default'){$arguments+=('/TASKS="'+$Task+'"')}
    $process=Start-Process -FilePath (Join-Path $qa 'setup.exe') -ArgumentList $arguments -WindowStyle Hidden -Wait -PassThru
    if($process.ExitCode -ne 0){throw ('QA installer failed: '+$process.ExitCode)}
}
function Run-Uninstaller {
    if(Test-Path -LiteralPath $uninstaller){
        $process=Start-Process -FilePath $uninstaller -ArgumentList @('/VERYSILENT','/SUPPRESSMSGBOXES','/NORESTART') -WindowStyle Hidden -Wait -PassThru
        if($process.ExitCode -ne 0){throw ('QA uninstall failed: '+$process.ExitCode)}
        for($i=0;$i -lt 100 -and (Test-Path -LiteralPath $uninstaller);$i++){Start-Sleep -Milliseconds 100}
        if(Test-Path -LiteralPath $uninstaller){throw 'QA uninstall did not finish'}
    }
}
try {
    # Use the real template with isolated AppId, mutex and startup registry key.
    # Tiny synthetic payloads test lifecycle behavior without running the app.
    $template=Get-Content -LiteralPath (Join-Path $repository 'release\hamster-toolbox.iss') -Raw -Encoding UTF8
    $template=$template.Replace('AppId=HamsterToolbox',('AppId=HamsterAutostartQA-'+$id)).Replace('AppMutex=Local\HamsterToolbox.Running',('AppMutex=Local\HamsterAutostartQA.'+$id)).Replace('Software\Microsoft\Windows\CurrentVersion\Run',$registryPath)
    if($template.Contains('AppId=HamsterToolbox') -or $template.Contains('Software\Microsoft\Windows\CurrentVersion\Run') -or -not $template.Contains($registryPath)){throw 'QA isolation substitution failed'}
    [IO.File]::WriteAllText((Join-Path $qa 'qa.iss'),$template,[Text.UTF8Encoding]::new($true))
    Copy-Item -LiteralPath (Join-Path $repository 'release\installer') -Destination (Join-Path $qa 'installer') -Recurse
    $payload=Join-Path $qa 'payload'
    New-Item -ItemType Directory -Path (Join-Path $payload 'tools') | Out-Null
    foreach($name in @('hamster-toolbox.exe','hamster-toolbox-desktop.exe','start-toolbox.cmd','tools-manifest.json','VERSION.txt','release-info.json','LICENSE','QUICKSTART.txt','THIRD-PARTY.txt','GO-DEPENDENCIES.txt','tools\synthetic.txt')){[IO.File]::WriteAllText((Join-Path $payload $name),'synthetic fixture, not an executable')}
    & $compiler '/Q' ('/DPayload='+$payload) '/DAppVersion=0.2.2-dev' '/DNumericVersion=0.2.2.0' ('/DBrandIcon='+(Join-Path $repository 'web\favicon.ico')) ('/DOutputDir='+$qa) '/DOutputName=setup' (Join-Path $qa 'qa.iss')
    if($LASTEXITCODE -ne 0){throw 'QA template compile failed'}
    Run-Installer
    if((Read-Startup) -ne ''){throw 'Fresh installation enabled startup by default'}
    $key=[Microsoft.Win32.Registry]::CurrentUser.CreateSubKey($registryPath)
    try{$key.SetValue('OtherApplication','preserve')}finally{$key.Dispose()}
    Run-Installer 'autostart'
    $expected='"'+(Join-Path $target 'hamster-toolbox-desktop.exe')+'" -tray -no-open'
    if((Read-Startup) -ne $expected){throw 'Selected startup command differs'}
    $custom=$expected+' -data "E:\synthetic profile"'
    Write-Startup $custom
    Run-Installer 'autostart'
    if((Read-Startup) -ne $custom){throw 'Upgrade lost custom data profile'}
    Run-Installer ''
    if((Read-Startup) -ne ''){throw 'Unchecking startup did not remove own entry'}
    Run-Installer 'autostart'
    Run-Uninstaller
    if((Read-Startup) -ne ''){throw 'Uninstall left own startup entry'}
    $foreign='"E:\another install\hamster-toolbox-desktop.exe" -tray -no-open'
    Write-Startup $foreign
    Run-Installer ''
    Run-Uninstaller
    if((Read-Startup) -ne $foreign){throw 'Uninstall deleted another installation startup'}
    $key=[Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($registryPath)
    try{if($key.GetValue('OtherApplication') -ne 'preserve'){throw 'Other registry values changed'}}finally{$key.Dispose()}
    Write-Host 'PASS: default-off install, opt-in, profile-preserving upgrade, opt-out, uninstall cleanup and foreign-entry preservation (isolated registry/AppId only).'
} finally {
    Run-Uninstaller
    [Microsoft.Win32.Registry]::CurrentUser.DeleteSubKeyTree($registryPath,$false)
    $resolved=[IO.Path]::GetFullPath($qa)
    $parent=[IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\')+'\'
    if(-not $resolved.StartsWith($parent,[StringComparison]::OrdinalIgnoreCase) -or (Split-Path -Leaf $resolved) -ne ('hamster-autostart-qa-'+$id)){throw 'QA cleanup path rejected'}
    Remove-Item -LiteralPath $resolved -Recurse -Force
}
