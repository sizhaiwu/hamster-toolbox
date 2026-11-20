@echo off
cd /d "%~dp0" || exit /b 1
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0check-release.ps1" %*
if errorlevel 1 (
  echo Checks failed. Review the message above. No package was created.
  pause
  exit /b 1
)
echo Checks completed. No package was created.
pause
