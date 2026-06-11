@echo off
title Streamclone — First-time setup
echo.
echo  Streamclone — First-time setup
echo  ==============================
echo  Requires Docker Desktop (running).
echo  Installs to: %USERPROFILE%\streamclone
echo.
echo  This will:
echo    1. Download the latest release
echo    2. Create config and secrets
echo    3. Pull Docker images and start the stack (~3-5 min)
echo    4. Add Start/Stop shortcuts and open the welcome page
echo.
powershell -NoProfile -ExecutionPolicy Bypass -Command "& { $ErrorActionPreference='Stop'; $tempInstall=Join-Path $env:TEMP 'streamclone-install.ps1'; Invoke-WebRequest -Uri 'https://raw.githubusercontent.com/Aron-Chu/streamclone/master/scripts/install.ps1' -OutFile $tempInstall -UseBasicParsing; & $tempInstall -Release -NonInteractive -DesktopShortcut }"
if errorlevel 1 (
  echo.
  echo Setup failed. Fix any errors above and try again.
  pause
  exit /b 1
)
echo.
echo Setup complete. Use "Start Streamclone" on your Desktop next time.
pause
