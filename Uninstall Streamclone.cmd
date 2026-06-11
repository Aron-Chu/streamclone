@echo off
title Streamclone — Uninstall
echo.
echo  Streamclone — Complete uninstall
echo  ================================
echo  Stops containers, deletes data volumes, removes secrets,
echo  removes shortcuts, and deletes this install folder.
echo.
echo  To pause without deleting anything, use Stop Streamclone.cmd instead.
echo.
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0launchers\install-streamclone-launcher.ps1" -Action uninstall -LauncherRoot "%~dp0."
if errorlevel 1 pause
