@echo off
color 0B
title Streamclone - Status check
echo.
echo   Streamclone - Status check
echo   ==========================
echo   Diagnose Docker, images, and web UI (no changes).
echo.
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\check-streamclone.ps1" -InstallDir "%~dp0."
echo.
pause
