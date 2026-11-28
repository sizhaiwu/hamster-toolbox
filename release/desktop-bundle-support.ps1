function Build-HamsterDesktopClient([string]$Repository,[string]$Version,[string]$OutputDirectory) {
    if ($Version -notmatch '^\d+\.\d+\.\d+(?:[-+][A-Za-z0-9.-]+)?$') { throw 'Invalid desktop client version.' }
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $pin=Get-Content -LiteralPath (Join-Path $Repository 'desktop-client\runtime.json') -Raw | ConvertFrom-Json
    if($pin.Version -notmatch '^\d+\.\d+\.\d+$' -or $pin.SHA256 -notmatch '^[a-f0-9]{64}$'){throw 'Invalid Electron runtime pin.'}
    $cache=Join-Path $Repository 'work\desktop-runtime'
    New-Item -ItemType Directory -Path $cache -Force | Out-Null
    $runtimeName='electron-v'+$pin.Version+'-win32-x64.zip'
    $runtimeZip=Join-Path $cache $runtimeName
    if(-not(Test-Path -LiteralPath $runtimeZip) -or (Get-FileHash -LiteralPath $runtimeZip -Algorithm SHA256).Hash -ne $pin.SHA256){
        $partial=$runtimeZip+'.partial'
        Invoke-WebRequest -UseBasicParsing -TimeoutSec 900 -Uri ('https://github.com/electron/electron/releases/download/v'+$pin.Version+'/'+$runtimeName) -OutFile $partial
        if((Get-FileHash -LiteralPath $partial -Algorithm SHA256).Hash -ne $pin.SHA256){throw 'Electron download checksum mismatch.'}
        Move-Item -LiteralPath $partial -Destination $runtimeZip -Force
    }
    $root=Join-Path ([IO.Path]::GetFullPath($OutputDirectory)) 'desktop-runtime'
    if(Test-Path -LiteralPath $root){throw 'Desktop runtime destination must be new and empty.'}
    [IO.Compression.ZipFile]::ExtractToDirectory($runtimeZip,$root)
    $originalBytes=[long](Get-ChildItem -LiteralPath $root -Recurse -File|Measure-Object Length -Sum).Sum
    # Only remove optional translations and the unused upstream demonstration app.
    # Codecs, GPU/software renderers, ICU, snapshots and legal notices stay intact.
    $locales=Join-Path $root 'locales'
    foreach($file in Get-ChildItem -LiteralPath $locales -File){
        if($file.Name -notin @('en-US.pak','zh-CN.pak','zh-TW.pak')){Remove-Item -LiteralPath $file.FullName -Force}
    }
    $demo=Join-Path $root 'resources\default_app.asar'
    if(Test-Path -LiteralPath $demo){Remove-Item -LiteralPath $demo -Force}
    Move-Item -LiteralPath (Join-Path $root 'electron.exe') -Destination (Join-Path $root 'hamster-desktop-client.exe')
    $app=Join-Path $root 'resources\app'
    New-Item -ItemType Directory -Path $app -Force | Out-Null
    foreach($file in @('main.cjs','policy.cjs','experience.cjs','preload.cjs','media.cjs')){Copy-Item -LiteralPath (Join-Path $Repository ('desktop-client\'+$file)) -Destination $app}
    Copy-Item -LiteralPath (Join-Path $Repository 'web\favicon.ico') -Destination $app
    Copy-Item -LiteralPath (Join-Path $Repository 'LICENSE') -Destination (Join-Path $app 'LICENSE')
    $packageJSON=@{name='hamster-toolbox-desktop-client';version=$Version;main='main.cjs';productName='Hamster Toolbox';license='MIT'} | ConvertTo-Json
    [IO.File]::WriteAllText((Join-Path $app 'package.json'),$packageJSON,[Text.UTF8Encoding]::new($false))
    $prefix=$root.TrimEnd('\')+'\'
    $files=@(Get-ChildItem -LiteralPath $root -Recurse -File | Sort-Object FullName | ForEach-Object { @{Path=$_.FullName.Substring($prefix.Length).Replace('\','/');SHA256=(Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant();Size=$_.Length} })
    $inventory=@{Version=$Version;Runtime=$pin.Version;Files=$files}|ConvertTo-Json -Depth 5
    $manifest=Join-Path $root 'bundle.json'
    [IO.File]::WriteAllText($manifest,$inventory,[Text.UTF8Encoding]::new($false))
    $hash=(Get-FileHash -LiteralPath $manifest -Algorithm SHA256).Hash.ToLowerInvariant()
    $bytes=[long](Get-ChildItem -LiteralPath $root -Recurse -File|Measure-Object Length -Sum).Sum
    return @{Version=$Version;Root=$root;SHA256=$hash;Bytes=$bytes;OriginalBytes=$originalBytes;RemovedBytes=($originalBytes-$bytes);Runtime=$pin.Version;FileCount=$files.Count}
}

function Test-HamsterDesktopRuntime([string]$Root,[string]$Version,[string]$ExpectedHash) {
    $base=[IO.Path]::GetFullPath($Root).TrimEnd('\','/')
    if($ExpectedHash -notmatch '^[a-fA-F0-9]{64}$' -or (Get-Item -LiteralPath $base -Force).Attributes -band [IO.FileAttributes]::ReparsePoint){throw 'Invalid desktop runtime root or inventory hash.'}
    $manifest=Join-Path $base 'bundle.json'
    if((Get-Item -LiteralPath $manifest -Force).Attributes -band [IO.FileAttributes]::ReparsePoint){throw 'Desktop runtime inventory cannot be a link.'}
    if((Get-FileHash -LiteralPath $manifest -Algorithm SHA256).Hash -ne $ExpectedHash){throw 'Desktop runtime inventory hash mismatch.'}
    $inventory=Get-Content -LiteralPath $manifest -Raw -Encoding UTF8|ConvertFrom-Json
    if($inventory.Version -ne $Version -or @($inventory.Files).Count -lt 1){throw 'Desktop runtime version mismatch.'}
    $seen=New-Object 'System.Collections.Generic.HashSet[string]' ([StringComparer]::OrdinalIgnoreCase)
    foreach($entry in $inventory.Files){
        $relative=[string]$entry.Path
        if(-not $relative -or [IO.Path]::IsPathRooted($relative) -or $relative -match '(^|[\\/])\.\.?([\\/]|$)|:' -or -not $seen.Add($relative.Replace('/','\')) -or $entry.SHA256 -notmatch '^[a-f0-9]{64}$'){throw 'Invalid desktop inventory entry.'}
        $path=[IO.Path]::GetFullPath((Join-Path $base $relative))
        if(-not $path.StartsWith($base+'\',[StringComparison]::OrdinalIgnoreCase)){throw 'Desktop inventory path escaped its directory.'}
        $item=Get-Item -LiteralPath $path -Force -ErrorAction Stop
        if($item.PSIsContainer -or $item.Length -ne $entry.Size){throw 'Desktop runtime file size mismatch.'}
        for($ancestor=$item;$ancestor.FullName -ne $base;$ancestor=Get-Item -LiteralPath (Split-Path -Parent $ancestor.FullName) -Force){if($ancestor.Attributes -band [IO.FileAttributes]::ReparsePoint){throw 'Desktop inventory cannot traverse a link.'}}
        if((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash -ne $entry.SHA256){throw ('Desktop runtime file changed: '+$relative)}
    }
    $actual=@(Get-ChildItem -LiteralPath $base -Recurse -File -Force)
    if($actual.Count -ne ($seen.Count+1)){throw 'Desktop runtime contains unexpected files.'}
    foreach($required in @('hamster-desktop-client.exe','icudtl.dat','resources.pak','resources/app/main.cjs','resources/app/experience.cjs','resources/app/preload.cjs','resources/app/media.cjs','resources/app/package.json','LICENSE','LICENSES.chromium.html','locales/zh-CN.pak','locales/en-US.pak')){if(-not $seen.Contains($required.Replace('/','\'))){throw ('Desktop runtime required file missing: '+$required)}}
}
