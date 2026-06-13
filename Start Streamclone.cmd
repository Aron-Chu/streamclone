@echo off
color 0B
title Streamclone - Open
echo.
echo   Streamclone - Open
echo   ==================
echo   Starts the Docker stack and opens http://127.0.0.1:8090/
echo.
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0launchers\install-streamclone-launcher.ps1" -Action start -LauncherRoot "%~dp0."
if errorlevel 1 pause
