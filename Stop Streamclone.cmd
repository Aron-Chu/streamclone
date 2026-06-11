@echo off
title Streamclone - Stop
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0launchers\install-streamclone-launcher.ps1" -Action stop -LauncherRoot "%~dp0."
if errorlevel 1 pause
