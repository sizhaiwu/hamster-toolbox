; Build through package-installer.cmd or package.ps1 -Installer.
#if Ver < EncodeVer(6, 5, 0)
  #error Inno Setup 6.5 or newer is required
#endif
#ifndef Payload
  #error Payload is required
#endif
#ifndef AppVersion
  #error AppVersion is required
#endif
[Setup]
AppId=HamsterToolbox
AppName=仓鼠工具箱
AppVersion={#AppVersion}
AppPublisher=sizhaiwu
AppPublisherURL=https://github.com/sizhaiwu/hamster-toolbox
AppSupportURL=https://github.com/sizhaiwu/hamster-toolbox/issues
DefaultDirName={localappdata}\Programs\HamsterToolbox
DefaultGroupName=仓鼠工具箱
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
MinVersion=10.0
DisableProgramGroupPage=yes
DisableDirPage=no
UsePreviousAppDir=yes
UsePreviousTasks=yes
AppMutex=Local\HamsterToolbox.Running
CloseApplications=no
RestartApplications=no
SetupIconFile={#BrandIcon}
UninstallDisplayIcon={app}\hamster-toolbox-desktop.exe
VersionInfoVersion={#NumericVersion}
OutputDir={#OutputDir}
OutputBaseFilename={#OutputName}
Compression=lzma2/ultra64
LZMAUseSeparateProcess=yes
LZMADictionarySize=131072
LZMANumFastBytes=128
LZMANumBlockThreads=1
SolidCompression=yes
WizardStyle=modern
Uninstallable=yes
SetupLogging=yes

[Languages]
Name: "chinesesimplified"; MessagesFile: "installer\ChineseSimplified.isl"
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "创建桌面快捷方式"; GroupDescription: "快捷方式："
Name: "autostart"; Description: "登录 Windows 后自动在系统托盘运行"; GroupDescription: "启动选项："; Flags: unchecked

[Files]
Source: "{#Payload}\hamster-toolbox-desktop.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Payload}\hamster-toolbox.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Payload}\start-toolbox.cmd"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Payload}\tools-manifest.json"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Payload}\VERSION.txt"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Payload}\release-info.json"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Payload}\desktop-client-info.json"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Payload}\LICENSE"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Payload}\QUICKSTART.txt"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Payload}\THIRD-PARTY.txt"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Payload}\GO-DEPENDENCIES.txt"; DestDir: "{app}"; Flags: ignoreversion
Source: "installer\INNO-LICENSE.txt"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Payload}\tools\*"; DestDir: "{app}\tools"; Flags: ignoreversion recursesubdirs createallsubdirs
Source: "{#Payload}\desktop-runtime\*"; DestDir: "{app}\desktop-runtime"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{group}\仓鼠工具箱"; Filename: "{app}\hamster-toolbox-desktop.exe"; WorkingDir: "{app}"; AppUserModelID: "HamsterToolbox"
Name: "{autodesktop}\仓鼠工具箱"; Filename: "{app}\hamster-toolbox-desktop.exe"; WorkingDir: "{app}"; AppUserModelID: "HamsterToolbox"; Tasks: desktopicon
Name: "{group}\卸载仓鼠工具箱"; Filename: "{uninstallexe}"

[Run]
Filename: "{app}\hamster-toolbox-desktop.exe"; Description: "打开仓鼠工具箱"; Flags: nowait postinstall skipifsilent

[UninstallDelete]
; Verified runtime binaries only; personal data is stored separately.
Type: filesandordirs; Name: "{app}\desktop-runtime"

; The optional Run entry belongs only to the current user. No service,
; firewall rule is created. The bundled desktop runtime is removed on uninstall.
; User settings/history remain in the existing per-user data directory.

[Code]
const
  StartupRunKey = 'Software\Microsoft\Windows\CurrentVersion\Run';
  StartupValue = 'HamsterToolbox';
var
  StartupChoiceLoaded: Boolean;
function OwnStartupEntry: Boolean;
var
  Value, Prefix: String;
begin
  Result := False;
  if RegQueryStringValue(HKCU, StartupRunKey, StartupValue, Value) then begin
    Prefix := '"' + ExpandConstant('{app}\hamster-toolbox-desktop.exe') + '"';
    Result := (CompareText(Copy(Value, 1, Length(Prefix)), Prefix) = 0) and
      ((Length(Value) = Length(Prefix)) or (Copy(Value, Length(Prefix) + 1, 1) = ' '));
  end;
end;

procedure CurPageChanged(CurPageID: Integer);
begin
  if (CurPageID = wpSelectTasks) and not StartupChoiceLoaded then begin
    StartupChoiceLoaded := True;
    if (Pos('/TASKS=', Uppercase(GetCmdTail)) = 0) and
      (Pos('/MERGETASKS=', Uppercase(GetCmdTail)) = 0) then begin
      if OwnStartupEntry then WizardSelectTasks('autostart')
      else WizardSelectTasks('!autostart');
    end;
  end;
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssPostInstall then begin
    if WizardIsTaskSelected('autostart') then begin
      if not OwnStartupEntry then
        if not RegWriteStringValue(HKCU, StartupRunKey, StartupValue,
          '"' + ExpandConstant('{app}\hamster-toolbox-desktop.exe') + '" -tray -no-open') then
          MsgBox('无法保存开机自启选项，可安装后在通用设置中重试。', mbError, MB_OK);
    end else if OwnStartupEntry then
      RegDeleteValue(HKCU, StartupRunKey, StartupValue);
  end;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if (CurUninstallStep = usUninstall) and OwnStartupEntry then
    RegDeleteValue(HKCU, StartupRunKey, StartupValue);
end;
