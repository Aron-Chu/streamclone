@echo off
title Streamclone Installer
echo.
echo  Streamclone installer
echo  ---------------------
echo  Requires Docker Desktop (running).
echo  Installs to: %USERPROFILE%\streamclone
echo.
powershell -NoProfile -ExecutionPolicy Bypass -Command "& { $ErrorActionPreference='Stop'; $tempInstall=Join-Path $env:TEMP 'streamclone-install.ps1'; Write-Host 'Downloading installer...'; Invoke-WebRequest -Uri 'https://raw.githubusercontent.com/Aron-Chu/streamclone/main/scripts/install.ps1' -OutFile $tempInstall -UseBasicParsing; & $tempInstall -Release -NonInteractive -DesktopShortcut }"
if errorlevel 1 (
  echo.
  echo Install failed. Fix any errors above and try again.
  pause
  exit /b 1
)
echo.
echo Install complete. Double-click "Start Streamclone" on your Desktop.
pause
