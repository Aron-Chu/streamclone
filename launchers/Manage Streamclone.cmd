@echo off
color 0B
title Streamclone
echo.
echo   Streamclone
echo   ===========
echo   Start, stop, repair, update, and uninstall.
echo.
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0install-streamclone-launcher.ps1" -Action manage -LauncherRoot "%~dp0.."
if errorlevel 1 pause
