@echo off
title Streamclone - Uninstall
echo.
echo  Streamclone - Complete uninstall
echo  ================================
echo  Stops containers, deletes data volumes, removes secrets,
echo  removes shortcuts, and deletes this install folder.
echo.
echo  To pause without deleting anything, use Stop Streamclone.cmd instead.
echo.
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0launchers\install-streamclone-launcher.ps1" -Action uninstall -LauncherRoot "%~dp0."
if errorlevel 2 if not errorlevel 3 (
  echo.
  echo Uninstall cancelled.
  pause
  exit /b 0
)
if errorlevel 1 (
  echo.
  echo Uninstall failed. See messages above for details.
  pause
  exit /b 1
)
echo.
echo Uninstall complete.
echo Desktop shortcuts removed. The install folder will be deleted shortly.
echo You can close this window.
echo.
pause
