#Requires -Version 5.1
# Shared logic for double-click Install / Start / Stop launchers.
param(
    [ValidateSet('install', 'start', 'stop')]
    [string]$Action,
    [string]$LauncherRoot = $PSScriptRoot
)

$ErrorActionPreference = 'Stop'
if ($LauncherRoot) {
    $LauncherRoot = $LauncherRoot.TrimEnd('\', '/')
}

function Get-StreamcloneRoot {
    param([string]$LauncherRoot)
    $LauncherRoot = $LauncherRoot.TrimEnd('\', '/')
    $candidates = @(
        $LauncherRoot,
        (Split-Path -Parent $LauncherRoot.TrimEnd('\')),
        (Join-Path $env:USERPROFILE 'streamclone')
    )
    foreach ($candidate in $candidates) {
        if (Test-Path (Join-Path $candidate 'scripts\start-streamclone.ps1')) {
            return $candidate
        }
    }
    return Join-Path $env:USERPROFILE 'streamclone'
}

function Invoke-StreamcloneInstall {
    $root = Get-StreamcloneRoot -LauncherRoot $LauncherRoot
    $setupPs1 = Join-Path $root 'scripts\setup.ps1'
    $shortcutPs1 = Join-Path $root 'scripts\install-desktop-shortcut.ps1'

    if (Test-Path $setupPs1) {
        if (-not (Test-Path (Join-Path $root '.env'))) {
            Write-Host 'Step 2/4: Creating config and secrets...' -ForegroundColor Cyan
            $setupArgs = @{ Profile = 'core'; NonInteractive = $true }
            $releaseEnv = Join-Path $root 'deploy\env\release-bundle.env'
            if ((Test-Path (Join-Path $root 'VERSION')) -or (Test-Path $releaseEnv)) {
                $setupArgs['UseImages'] = $true
                Write-Host 'Step 3/4: Pulling Docker images and starting stack (~3-5 min)...' -ForegroundColor Cyan
            } else {
                Write-Host 'Step 3/4: Building Docker images and starting stack (may take 10-20 min)...' -ForegroundColor Cyan
            }
            & $setupPs1 @setupArgs
        } else {
            Write-Host 'Already installed — refreshing Desktop shortcuts.' -ForegroundColor Yellow
        }
        if (Test-Path $shortcutPs1) {
            Write-Host 'Step 4/4: Creating Desktop shortcuts...' -ForegroundColor Cyan
            & $shortcutPs1 -InstallDir $root
        }
        $startPs1 = Join-Path $root 'scripts\start-streamclone.ps1'
        if (Test-Path $startPs1) {
            Write-Host ''
            Write-Host 'Opening Streamclone in your browser...' -ForegroundColor Green
            & $startPs1
        }
        return
    }

    Write-Host 'Step 1/4: Downloading latest release...' -ForegroundColor Cyan
    $tempInstall = Join-Path $env:TEMP 'streamclone-install.ps1'
    Invoke-WebRequest -Uri 'https://raw.githubusercontent.com/Aron-Chu/streamclone/master/scripts/install.ps1' -OutFile $tempInstall -UseBasicParsing
    & $tempInstall -Release -NonInteractive -DesktopShortcut
}

$root = Get-StreamcloneRoot -LauncherRoot $LauncherRoot

switch ($Action) {
    'install' { Invoke-StreamcloneInstall }
    'start' {
        $startPs1 = Join-Path $root 'scripts\start-streamclone.ps1'
        if (-not (Test-Path $startPs1)) {
            throw "Streamclone not installed at $root. Run Install Streamclone first."
        }
        Write-Host 'Starting Docker stack and opening browser...' -ForegroundColor Cyan
        & $startPs1
    }
    'stop' {
        $stopPs1 = Join-Path $root 'scripts\stop-streamclone.ps1'
        if (-not (Test-Path $stopPs1)) {
            throw "Streamclone not installed at $root."
        }
        & $stopPs1
    }
}
