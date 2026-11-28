[CmdletBinding()]
param([Parameter(Mandatory=$true)][string]$Payload,[string]$ISCC='')
$ErrorActionPreference='Stop'
$repo=Split-Path -Parent $PSScriptRoot
. (Join-Path $repo 'release\installer-support.ps1')
. (Join-Path $repo 'release\desktop-bundle-support.ps1')
$compiler=Resolve-InnoCompiler $ISCC $repo
$info=Get-Content (Join-Path $Payload 'desktop-client-info.json') -Raw|ConvertFrom-Json
Test-HamsterDesktopRuntime (Join-Path $Payload 'desktop-runtime') $info.Version $info.SHA256
$qa=Join-Path $repo ('work\bundled-installer-qa-'+[guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $qa|Out-Null
$target=Join-Path $qa 'installed'
$data=Join-Path $qa 'profile'
$tag=Split-Path -Leaf $qa
$template=Get-Content (Join-Path $repo 'release\hamster-toolbox.iss') -Raw -Encoding UTF8
$template=$template.Replace('AppId=HamsterToolbox',('AppId='+$tag)).Replace('AppMutex=Local\HamsterToolbox.Running',('AppMutex=Local\'+$tag)).Replace('Software\Microsoft\Windows\CurrentVersion\Run',('Software\HamsterToolbox\'+$tag))
[IO.File]::WriteAllText((Join-Path $qa 'qa.iss'),$template,[Text.UTF8Encoding]::new($true))
Copy-Item (Join-Path $repo 'release\installer') (Join-Path $qa 'installer') -Recurse
& $compiler '/Q' ('/DPayload='+$Payload) ('/DAppVersion='+$info.Version) '/DNumericVersion=0.2.7.0' ('/DBrandIcon='+(Join-Path $repo 'web\favicon.ico')) ('/DOutputDir='+$qa) '/DOutputName=setup' (Join-Path $qa 'qa.iss')
if($LASTEXITCODE -ne 0){throw 'Installer compile failed'}
$backend=$null
try {
 $p=Start-Process (Join-Path $qa 'setup.exe') -ArgumentList @('/CURRENTUSER','/VERYSILENT','/SUPPRESSMSGBOXES','/NORESTART','/NOICONS','/TASKS=""',('/DIR="'+$target+'"')) -WindowStyle Hidden -Wait -PassThru
 if($p.ExitCode -ne 0){throw 'Installer failed'}
 Test-HamsterDesktopRuntime (Join-Path $target 'desktop-runtime') $info.Version $info.SHA256
 New-Item -ItemType Directory -Path $data -Force|Out-Null
 [IO.File]::WriteAllText((Join-Path $data 'keep-note.txt'),'synthetic note survives uninstall')
 $program=Join-Path $target 'hamster-toolbox.exe'
 $backend=Start-Process $program -ArgumentList '-no-open','-data',$data -WindowStyle Hidden -RedirectStandardOutput (Join-Path $qa 'startup.log') -RedirectStandardError (Join-Path $qa 'stderr.log') -PassThru
 $url=''
 for($i=0;$i -lt 100;$i++){
  $startup=Get-Content (Join-Path $qa 'startup.log') -Raw -ErrorAction SilentlyContinue
  if($startup){$match=[regex]::Match([string]$startup,'http://127\.0\.0\.1:\d+/\?session=([a-f0-9]+)');if($match.Success){$url=$match.Value;$token=$match.Groups[1].Value;break}}
  Start-Sleep -Milliseconds 100
 }
 if(-not $url){throw 'Installed backend did not start'}
 $status=Invoke-RestMethod ($url.Split('?')[0]+'api/desktop-client') -Method Post -Headers @{'X-Toolbox-Token'=$token} -ContentType 'application/json' -Body '{"Action":"status"}'
 if(-not $status.Installed -or $status.Mode -ne 'desktop'){throw 'Fresh installation did not default to bundled desktop'}
 if((Get-Process -Id $backend.Id).Path -ne $program){throw 'QA process identity changed'}
 Stop-Process -Id $backend.Id;$backend=$null
 $p=Start-Process (Join-Path $target 'unins000.exe') -ArgumentList '/VERYSILENT','/SUPPRESSMSGBOXES','/NORESTART' -WindowStyle Hidden -Wait -PassThru
 if($p.ExitCode -ne 0){throw 'Uninstall failed'}
 for($i=0;$i -lt 100 -and (Test-Path (Join-Path $target 'unins000.exe'));$i++){Start-Sleep -Milliseconds 100}
 if(Test-Path (Join-Path $target 'desktop-runtime')){throw 'Runtime remained after uninstall'}
 if(-not(Test-Path (Join-Path $data 'keep-note.txt'))){throw 'User data lost'}
 @{Installer=(Join-Path $qa 'setup.exe');Bytes=(Get-Item (Join-Path $qa 'setup.exe')).Length;Passed=$true}|ConvertTo-Json|Set-Content (Join-Path $qa 'result.json')
 Write-Output ('PASS integrated offline install/default desktop/backend startup/uninstall/data retention: '+$qa)
} finally {
 if($backend -and (Get-Process -Id $backend.Id -ErrorAction SilentlyContinue).Path -eq (Join-Path $target 'hamster-toolbox.exe')){Stop-Process -Id $backend.Id}
 if(Test-Path (Join-Path $target 'unins000.exe')){Start-Process (Join-Path $target 'unins000.exe') -ArgumentList '/VERYSILENT','/SUPPRESSMSGBOXES','/NORESTART' -WindowStyle Hidden -Wait|Out-Null}
}
