@echo off
color 0B
title Streamclone
echo.
echo   Streamclone
echo   ===========
echo   Start, stop, repair, update, and uninstall.
echo.
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\streamclone-manager.ps1" -Action menu
if errorlevel 1 pause
