@echo off
chcp 65001 >nul
cd /d "%~dp0"
go test ./...
if errorlevel 1 exit /b 1
go build -buildvcs=false -trimpath -ldflags="-s -w" -o "hamster-toolbox.exe" .
