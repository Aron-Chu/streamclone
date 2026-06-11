@echo off
title Streamclone - Stop
echo.
echo  Streamclone - Stop
echo  ==================
echo  Stops all Streamclone Docker containers.
echo.
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0install-streamclone-launcher.ps1" -Action stop -LauncherRoot "%~dp0"
if errorlevel 1 pause
