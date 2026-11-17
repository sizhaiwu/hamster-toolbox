@echo off
cd /d "%~dp0"
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0package.ps1" %*
if errorlevel 1 (
  echo Packaging failed. See the error above.
  pause
  exit /b 1
)
echo Packaging completed. Open the dist folder to share the ZIP file.
pause
