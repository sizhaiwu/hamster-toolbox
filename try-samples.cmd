@echo off
setlocal
chcp 65001 >nul
title Hamster Toolbox Samples
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0samples\create-playground.ps1"
if errorlevel 1 (
  echo Failed to prepare demo files. No existing demo folder was deleted.
  pause
  exit /b 1
)
