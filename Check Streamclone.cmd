@echo off
title Streamclone - Status check
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\check-streamclone.ps1" -InstallDir "%~dp0."
echo.
pause
