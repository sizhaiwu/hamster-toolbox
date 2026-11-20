@echo off
setlocal
chcp 65001 >nul
title Hamster Toolbox
cd /d "%~dp0" || exit /b 1
if not exist "hamster-toolbox.exe" (
    echo.
    echo   仓鼠工具箱 - 启动失败
    echo   找不到 hamster-toolbox.exe。
    echo   请先完整解压发布包，或在源码目录运行 build.cmd 编译。
    echo.
    pause
    exit /b 1
)
"hamster-toolbox.exe" %*
set "toolbox_exit_code=%errorlevel%"
if not "%toolbox_exit_code%"=="0" (
    echo.
    echo   程序已停止，请查看上方提示。
    pause
)
exit /b %toolbox_exit_code%
