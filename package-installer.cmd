@echo off
cd /d "%~dp0" || exit /b 1
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0package.ps1" -Installer %*
if errorlevel 1 (
  echo Installer build failed. Review the message above.
  pause
  exit /b 1
)
echo Installer and portable packages are ready in dist.
pause
