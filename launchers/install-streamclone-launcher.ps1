#Requires -Version 5.1
# Shared logic for double-click Install / Start / Stop launchers.
param(
    [ValidateSet('install', 'start', 'stop')]
    [string]$Action,
    [string]$LauncherRoot = $PSScriptRoot
)

$ErrorActionPreference = 'Stop'

function Get-StreamcloneRoot {
    param([string]$LauncherRoot)
    $candidates = @(
        $LauncherRoot.TrimEnd('\'),
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
            $setupArgs = @{ Profile = 'core'; NonInteractive = $true }
            $releaseEnv = Join-Path $root 'deploy\env\release-bundle.env'
            if ((Test-Path (Join-Path $root 'VERSION')) -or (Test-Path $releaseEnv)) {
                $setupArgs['UseImages'] = $true
            }
            & $setupPs1 @setupArgs
        }
        if (Test-Path $shortcutPs1) {
            & $shortcutPs1 -InstallDir $root
        }
        return
    }

    $tempInstall = Join-Path $env:TEMP 'streamclone-install.ps1'
    Write-Host 'Downloading installer...'
    Invoke-WebRequest -Uri 'https://raw.githubusercontent.com/Aron-Chu/streamclone/main/scripts/install.ps1' -OutFile $tempInstall -UseBasicParsing
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
