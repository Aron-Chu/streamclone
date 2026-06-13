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
powershell -NoProfile -ExecutionPolicy Bypass -Command "& { $ErrorActionPreference='Stop'; $repo='Aron-Chu/streamclone'; $headers=@{'User-Agent'='streamclone-bootstrap'}; $sha=(Invoke-RestMethod -Uri ('https://api.github.com/repos/' + $repo + '/commits/master') -Headers $headers).sha; $f=Join-Path $env:TEMP 'streamclone-bootstrap.ps1'; $lib=Join-Path $env:TEMP 'streamclone-bootstrap-lib'; $u=('https://raw.githubusercontent.com/' + $repo + '/' + $sha + '/scripts/bootstrap-windows-install.ps1'); $log=Join-Path $env:TEMP 'debug-ccdd9b.log'; $scLog=Join-Path (Join-Path $env:USERPROFILE 'streamclone') 'debug-ccdd9b.log'; function Write-InstallDebug($msg,$data){ $entry=@{sessionId='ccdd9b';runId='pre-fix';hypothesisId='A';location='Install.cmd';message=$msg;data=$data;timestamp=[DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()}|ConvertTo-Json -Compress; Add-Content -LiteralPath $log -Value ($entry + [Environment]::NewLine) -Encoding utf8; if (Test-Path (Split-Path $scLog -Parent)) { Add-Content -LiteralPath $scLog -Value ($entry + [Environment]::NewLine) -Encoding utf8 } }; Write-InstallDebug 'bootstrap fetch start' @{sha=$sha;url=$u}; if (Test-Path $lib) { Remove-Item -LiteralPath $lib -Recurse -Force -ErrorAction SilentlyContinue }; Invoke-WebRequest -Uri $u -OutFile $f -Headers $headers -UseBasicParsing; $content=Get-Content -LiteralPath $f -Raw; Write-InstallDebug 'bootstrap downloaded' @{hasLibDirFix=($content -match 'StreamcloneBootstrapLibDir');hasResolveEnv=($content -match 'Resolve-StreamcloneBootstrapEnvScript')}; & $f; exit $LASTEXITCODE }"
if errorlevel 1 (
  echo.
  echo Setup failed. Run Check Streamclone.cmd in %%USERPROFILE%%\streamclone for details.
  pause
  exit /b 1
)
echo.
echo Setup complete. Use the Streamclone shortcut on your Desktop next time.
pause
