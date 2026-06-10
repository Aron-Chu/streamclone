@echo off
title Streamclone
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0install-streamclone-launcher.ps1" -Action start -LauncherRoot "%~dp0"
if errorlevel 1 pause
