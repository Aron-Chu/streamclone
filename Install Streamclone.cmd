@echo off
color 0B
title Streamclone - First-time setup
echo.
echo   Streamclone - First-time setup
echo   ==============================
echo   Requires Docker Desktop (running).
echo.
echo   This will:
echo     1. Create config and secrets
echo     2. Pull Docker images and start the stack (~3-5 min)
echo     3. Add a Streamclone shortcut to your Desktop
echo     4. Open http://127.0.0.1:8090/ in your browser
echo.
echo   If setup fails, run Check Streamclone.cmd in this folder first.
echo.
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0launchers\install-streamclone-launcher.ps1" -Action install -LauncherRoot "%~dp0."
if errorlevel 1 (
  echo.
  echo Setup failed. Run Check Streamclone.cmd for details.
  pause
  exit /b 1
)
echo.
echo Setup complete. Use the Streamclone shortcut on your Desktop next time.
pause
