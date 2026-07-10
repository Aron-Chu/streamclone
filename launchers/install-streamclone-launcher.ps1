#Requires -Version 5.1
# Shared logic for double-click Install / Start / Stop launchers.
param(
    [ValidateSet('install', 'start', 'stop', 'uninstall', 'manage', 'repair', 'check', 'update')]
    [string]$Action,
    [string]$LauncherRoot = $PSScriptRoot
)

$ErrorActionPreference = 'Stop'
if ($LauncherRoot) {
    $LauncherRoot = $LauncherRoot.TrimEnd('\', '/')
}

$libDir = Join-Path (Split-Path -Parent $PSScriptRoot) 'scripts\lib'
if (-not (Test-Path $libDir)) {
    $libDir = Join-Path (Split-Path -Parent (Split-Path -Parent $PSScriptRoot)) 'scripts\lib'
}
. (Join-Path $libDir 'install-upgrade.ps1')
. (Join-Path $libDir 'env.ps1')

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

function Test-StreamcloneWebOk {
    param([string]$Url = '')
    if ([string]::IsNullOrWhiteSpace($Url)) { $Url = Get-StreamcloneAppUrl }
    return Test-StreamcloneWebReachable -Url $Url
}

function Write-StreamcloneVersionNotice {
    param([string]$Root)
    $versions = Get-StreamcloneInstallVersions -Root $Root -FetchLatest
    $parts = @()
    if ($versions.bundleVersion) { $parts += "Bundle $($versions.bundleVersion)" }
    if ($versions.imageTag) { $parts += "Images $($versions.imageTag)" }
    if ($versions.latestRelease) { $parts += "Latest $($versions.latestRelease)" }
    if ($parts.Count -gt 0) {
        Write-Host ("Version: " + ($parts -join ' | ')) -ForegroundColor DarkGray
    }
    if (Test-StreamcloneUpgradeNeeded -Root $Root) {
        Write-Host 'Update available - run Manage Streamclone -> Update (or Install to sync).' -ForegroundColor Yellow
    }
}

function Invoke-StreamclonePreflight {
    param([string]$Root)
    $preflight = Join-Path $Root 'scripts\preflight-deps.ps1'
    if (-not (Test-Path $preflight)) {
        $preflight = Join-Path (Split-Path -Parent $PSScriptRoot) 'scripts\preflight-deps.ps1'
    }
    if (-not (Test-Path $preflight)) {
        Write-Warning 'Preflight script missing - continuing without checks.'
        return $true
    }
    Write-Host 'Checking prerequisites (Docker Desktop)...' -ForegroundColor Cyan
    & $preflight -InstallHints -TryStartDocker
    return ($LASTEXITCODE -eq 0)
}

function Invoke-StreamcloneInstall {
    $root = Get-StreamcloneRoot -LauncherRoot $LauncherRoot
    $setupPs1 = Join-Path $root 'scripts\setup.ps1'
    $shortcutPs1 = Join-Path $root 'scripts\install-desktop-shortcut.ps1'
    $checkPs1 = Join-Path $root 'scripts\check-streamclone.ps1'
    $hasEnv = Test-Path (Join-Path $root '.env')

    if (Test-Path $setupPs1) {
        Write-StreamcloneVersionNotice -Root $root

        if (Test-StreamcloneWebOk -and $hasEnv) {
            if (Test-StreamcloneUpgradeNeeded -Root $root) {
                Write-Host 'Upgrade needed - syncing images before start...' -ForegroundColor Yellow
                if (-not (Invoke-StreamclonePreflight -Root $root)) { exit 1 }
                Invoke-StreamcloneUpgrade -Root $root
            } else {
                Write-Host ("Streamclone is already running at {0}" -f (Get-StreamcloneAppUrl)) -ForegroundColor Green
                Write-Host 'Refreshing install scripts and Desktop shortcut.' -ForegroundColor Yellow
            }
            Update-StreamcloneBootstrapOverlayFromMaster -Dir $root
            if (Test-Path $shortcutPs1) {
                Write-Host 'Step 4/4: Adding Desktop shortcut...' -ForegroundColor Cyan
                & $shortcutPs1 -InstallDir $root
            }
            $startPs1 = Join-Path $root 'scripts\start-streamclone.ps1'
            if (Test-Path $startPs1) {
                Write-Host ''
                Write-Host 'Opening Streamclone in your browser...' -ForegroundColor Green
                & powershell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $root 'scripts\ensure-setup-control.ps1') -Root $root -RequireProxy
                $setupControlExitCode = if ($null -ne $LASTEXITCODE) { [int]$LASTEXITCODE } else { 0 }
                if ($setupControlExitCode -ne 0) { exit $setupControlExitCode }
                Start-Process (Get-StreamcloneAppUrl)
            }
            return
        }

        if (-not (Invoke-StreamclonePreflight -Root $root)) {
            Write-Host ''
            Write-Host 'Prerequisites not met. Run Check Streamclone.cmd for details.' -ForegroundColor Red
            if (Test-Path $checkPs1) {
                & $checkPs1 -InstallDir $root
            }
            exit 1
        }

        if (-not $hasEnv) {
            Write-Host 'Step 2/4: Creating config and secrets...' -ForegroundColor Cyan
            $setupArgs = @{ Profile = 'core'; NonInteractive = $true }
            $releaseEnv = Join-Path $root 'deploy\env\release-bundle.env'
            if ((Test-Path (Join-Path $root 'VERSION')) -or (Test-Path $releaseEnv)) {
                $setupArgs['UseImages'] = $true
                Write-Host 'Step 3/4: Pulling Docker images and starting stack (3-8 min on first install)...' -ForegroundColor Cyan
            } else {
                Write-Host 'Step 3/4: Building Docker images and starting stack (may take 10-20 min)...' -ForegroundColor Cyan
            }
            try {
                & $setupPs1 @setupArgs
                if ($LASTEXITCODE -ne 0) {
                    throw "setup.ps1 exited with code $LASTEXITCODE"
                }
            } catch {
                Write-Host ''
                Write-Host "Setup error: $($_.Exception.Message)" -ForegroundColor Red
                $startPs1 = Join-Path $root 'scripts\start-streamclone.ps1'
                if ((Test-Path (Join-Path $root '.env')) -and (Test-Path $startPs1)) {
                    Write-Host 'Config exists - trying Start Streamclone as recovery...' -ForegroundColor Yellow
                    & $startPs1
                    if ($LASTEXITCODE -eq 0 -and (Test-StreamcloneWebOk)) {
                        Write-Host 'Recovery start succeeded.' -ForegroundColor Green
                    } else {
                        Write-Host 'Recovery start did not complete. Run Check Streamclone.cmd in your install folder.' -ForegroundColor Yellow
                        if (Test-Path $checkPs1) {
                            Write-Host ''
                            & $checkPs1 -InstallDir $root
                        }
                        exit 1
                    }
                } else {
                    Write-Host 'Run Check Streamclone.cmd in your install folder for a full diagnostic.' -ForegroundColor Yellow
                    if (Test-Path $checkPs1) {
                        Write-Host ''
                        & $checkPs1 -InstallDir $root
                    }
                    exit 1
                }
            }
        } else {
            if (Test-StreamcloneUpgradeNeeded -Root $root) {
                Write-Host 'Upgrade needed - syncing IMAGE_TAG and pulling images...' -ForegroundColor Yellow
                Invoke-StreamcloneUpgrade -Root $root
            } else {
                Write-Host 'Already configured - starting stack...' -ForegroundColor Yellow
                $startPs1 = Join-Path $root 'scripts\start-streamclone.ps1'
                if (Test-Path $startPs1) {
                    & $startPs1
                    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
                }
            }
        }
        if (Test-Path $shortcutPs1) {
            Write-Host 'Step 4/4: Adding Desktop shortcut...' -ForegroundColor Cyan
            & $shortcutPs1 -InstallDir $root
        }
        $startPs1 = Join-Path $root 'scripts\start-streamclone.ps1'
        if (Test-Path $startPs1) {
            Write-Host ''
            Write-Host 'Opening Streamclone in your browser...' -ForegroundColor Green
            if (-not (Test-StreamcloneWebOk)) {
                & $startPs1 -NoBrowser
                if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
            } else {
                & powershell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $root 'scripts\ensure-setup-control.ps1') -Root $root -RequireProxy
                $setupControlExitCode = if ($null -ne $LASTEXITCODE) { [int]$LASTEXITCODE } else { 0 }
                if ($setupControlExitCode -ne 0) { exit $setupControlExitCode }
            }
            Start-Process (Get-StreamcloneWelcomeUrl)
        }
        return
    }

    Write-Host 'Step 1/4: Downloading latest release...' -ForegroundColor Cyan
    $tempBootstrap = Join-Path $env:TEMP 'streamclone-bootstrap.ps1'
    $headers = @{ 'User-Agent' = 'streamclone-bootstrap' }
    $repo = 'Aron-Chu/streamclone'
    $sha = (Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/commits/master" -Headers $headers).sha
    $bootstrapUrl = "https://raw.githubusercontent.com/$repo/$sha/scripts/bootstrap-windows-install.ps1"
    Invoke-WebRequest -Uri $bootstrapUrl -OutFile $tempBootstrap -Headers $headers -UseBasicParsing
    & $tempBootstrap -InstallDir (Join-Path $env:USERPROFILE 'streamclone')
}

$root = Get-StreamcloneRoot -LauncherRoot $LauncherRoot

switch ($Action) {
    'install' { Invoke-StreamcloneInstall }
    'check' {
        $checkPs1 = Join-Path $root 'scripts\check-streamclone.ps1'
        if (-not (Test-Path $checkPs1)) {
            throw "Check script missing at $root"
        }
        & $checkPs1 -InstallDir $root
        exit $LASTEXITCODE
    }
    'start' {
        $startPs1 = Join-Path $root 'scripts\start-streamclone.ps1'
        if (-not (Test-Path $startPs1)) {
            throw "Streamclone not installed at $root. Run Install Streamclone first."
        }
        Write-Host 'Starting Docker stack and opening browser...' -ForegroundColor Cyan
        & $startPs1
        exit $LASTEXITCODE
    }
    'stop' {
        $stopPs1 = Join-Path $root 'scripts\stop-streamclone.ps1'
        if (-not (Test-Path $stopPs1)) {
            throw "Streamclone not installed at $root."
        }
        Write-Host 'Stopping Docker stack (config and data are kept)...' -ForegroundColor Cyan
        & $stopPs1
        exit $LASTEXITCODE
    }
    'uninstall' {
        $uninstallPs1 = Join-Path $root 'scripts\uninstall-streamclone.ps1'
        if (-not (Test-Path $uninstallPs1)) {
            throw "Streamclone not installed at $root."
        }
        & $uninstallPs1 -InstallDir $root
        exit $LASTEXITCODE
    }
    'manage' {
        $managerPs1 = Join-Path $root 'scripts\streamclone-manager.ps1'
        if (-not (Test-Path $managerPs1)) {
            throw "Manager script missing at $root."
        }
        & $managerPs1 -Action menu -InstallDir $root
    }
    'repair' {
        $managerPs1 = Join-Path $root 'scripts\streamclone-manager.ps1'
        if (-not (Test-Path $managerPs1)) {
            throw "Manager script missing at $root."
        }
        & $managerPs1 -Action repair -InstallDir $root
        exit $LASTEXITCODE
    }
    'update' {
        $managerPs1 = Join-Path $root 'scripts\streamclone-manager.ps1'
        if (-not (Test-Path $managerPs1)) {
            throw "Manager script missing at $root."
        }
        & $managerPs1 -Action update -InstallDir $root
        exit $LASTEXITCODE
    }
}
