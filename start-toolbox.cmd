@echo off
chcp 65001 >nul
title Hamster Toolbox
cd /d "%~dp0"
"hamster-toolbox.exe"
if errorlevel 1 pause
