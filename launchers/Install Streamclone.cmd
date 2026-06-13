@echo off
color 0B
title Streamclone - First-time setup
echo.
echo   Streamclone - First-time setup
echo   ==============================
echo   Requires Docker Desktop (running).
echo   Installs to: %USERPROFILE%\streamclone
echo.
echo   This will:
echo     1. Download the latest release
echo     2. Create config and secrets
echo     3. Pull Docker images and start the stack (~3-5 min)
echo     4. Add a Streamclone shortcut and open the directory
echo.
echo   Windows may show "Unknown Publisher" - click Run. We are not code-signed yet.
echo   Some antivirus tools flag new unsigned installers - see docs/install-desktop.md
echo.
echo   If setup fails but the app already works, run Check Streamclone.cmd first.
echo.
powershell -NoProfile -ExecutionPolicy Bypass -Command "& { $ErrorActionPreference='Stop'; $repo='Aron-Chu/streamclone'; $headers=@{'User-Agent'='streamclone-bootstrap'}; $sha=(Invoke-RestMethod -Uri ('https://api.github.com/repos/' + $repo + '/commits/master') -Headers $headers).sha; $f=Join-Path $env:TEMP 'streamclone-bootstrap.ps1'; $lib=Join-Path $env:TEMP 'streamclone-bootstrap-lib'; $u=('https://raw.githubusercontent.com/' + $repo + '/' + $sha + '/scripts/bootstrap-windows-install.ps1'); if (Test-Path $lib) { Remove-Item -LiteralPath $lib -Recurse -Force -ErrorAction SilentlyContinue }; Invoke-WebRequest -Uri $u -OutFile $f -Headers $headers -UseBasicParsing; & $f; exit $LASTEXITCODE }"
if errorlevel 1 (
  echo.
  echo Setup failed. Run Check Streamclone.cmd in %%USERPROFILE%%\streamclone for details.
  pause
  exit /b 1
)
echo.
echo Setup complete. Use the Streamclone shortcut on your Desktop next time.
pause
