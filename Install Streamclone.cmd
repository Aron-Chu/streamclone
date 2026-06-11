@echo off
title Streamclone - First-time setup
echo.
echo  Streamclone - First-time setup
echo  ==============================
echo  Requires Docker Desktop (running).
echo.
echo  This will:
echo    1. Create config and secrets
echo    2. Pull Docker images and start the stack (~3-5 min)
echo    3. Add Start/Stop shortcuts to your Desktop
echo    4. Open http://localhost:8090/welcome in your browser
echo.
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0launchers\install-streamclone-launcher.ps1" -Action install -LauncherRoot "%~dp0."
if errorlevel 1 (
  echo.
  echo Setup failed. Fix any errors above and try again.
  pause
  exit /b 1
)
echo.
echo Setup complete. Use "Start Streamclone" on your Desktop next time.
pause
