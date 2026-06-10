@echo off
title Streamclone Installer
echo.
echo  Streamclone installer
echo  ---------------------
echo  Requires Docker Desktop (running).
echo  Installs to: %USERPROFILE%\streamclone
echo.
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0launchers\install-streamclone-launcher.ps1" -Action install -LauncherRoot "%~dp0"
if errorlevel 1 (
  echo.
  echo Install failed. Fix any errors above and try again.
  pause
  exit /b 1
)
echo.
echo Install complete. Double-click "Start Streamclone" on your Desktop.
pause
