#Requires -Version 5.1
# Check prerequisites for Streamclone (non-developer friendly).
param(
    [switch]$InstallHints,
    [switch]$Quiet
)

$ErrorActionPreference = 'Stop'
$errors = 0
$warnings = 0

function Write-Check {
    param([string]$Status, [string]$Message)
    if ($Quiet) { return }
    $color = switch ($Status) {
        'ok' { 'Green' }
        'warn' { 'Yellow' }
        'fail' { 'Red' }
        default { 'Gray' }
    }
    $icon = switch ($Status) {
        'ok' { '[ok]' }
        'warn' { '[!!]' }
        'fail' { '[X]' }
        default { '[--]' }
    }
    Write-Host "$icon $Message" -ForegroundColor $color
}

function Test-PortFree {
    param([int]$Port)
    $inUse = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue
    return -not $inUse
}

if (-not $Quiet) {
    Write-Host ''
    Write-Host 'Streamclone - dependency check' -ForegroundColor Cyan
    Write-Host '------------------------------'
}

# Git (needed for clone/install path)
if (Get-Command git -ErrorAction SilentlyContinue) {
    Write-Check ok 'Git is installed'
} else {
    Write-Check fail 'Git is not installed'
    $errors++
    if ($InstallHints) {
        Write-Host '  Install: winget install Git.Git' -ForegroundColor DarkGray
    }
}

# Docker CLI
if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    Write-Check fail 'Docker is not installed or not on PATH'
    $errors++
    if ($InstallHints) {
        Write-Host '  Install Docker Desktop: https://docs.docker.com/desktop/setup/install/windows-install/' -ForegroundColor DarkGray
        Write-Host '  Or try: winget install Docker.DockerDesktop' -ForegroundColor DarkGray
    }
} else {
    Write-Check ok 'Docker CLI found'
    try {
        docker info 2>&1 | Out-Null
        if ($LASTEXITCODE -ne 0) { throw 'docker info failed' }
        Write-Check ok 'Docker engine is running'
    } catch {
        Write-Check fail 'Docker is installed but not running - start Docker Desktop'
        $errors++
    }
    try {
        docker compose version 2>&1 | Out-Null
        if ($LASTEXITCODE -ne 0) { throw 'compose missing' }
        Write-Check ok 'Docker Compose v2 available'
    } catch {
        Write-Check fail 'docker compose is missing - update Docker Desktop'
        $errors++
    }
}

# WSL2 hint (Docker Desktop on Windows)
if ($IsWindows -or $env:OS -match 'Windows') {
    $wsl = Get-Command wsl -ErrorAction SilentlyContinue
    if ($wsl) {
        $wslList = wsl -l -v 2>$null
        if ($wslList -match 'Stopped') {
            Write-Check warn 'One or more WSL distros are stopped - if localhost acts stale, run: wsl --shutdown'
            $warnings++
        } else {
            Write-Check ok 'WSL available'
        }
    }
}

# Port 8090
if (Test-PortFree -Port 8090) {
    Write-Check ok 'Port 8090 is free (Streamclone proxy)'
} else {
    Write-Check warn 'Port 8090 is already in use - another app or old Streamclone stack may be running'
    $warnings++
}

# Optional Twitch CLI
if (Get-Command twitch -ErrorAction SilentlyContinue) {
    Write-Check ok 'Twitch CLI found (optional - Clip Studio / chat login)'
} else {
    Write-Check warn 'Twitch CLI not found - core viewing works without it; clips need it later'
    $warnings++
    if ($InstallHints) {
        Write-Host '  Install: https://github.com/twitchdev/twitch-cli#installation' -ForegroundColor DarkGray
    }
}

# Disk space (rough)
try {
    $root = Split-Path -Parent $PSScriptRoot
    $drive = (Get-Item $root).PSDrive.Name
    $freeGb = [math]::Round((Get-PSDrive $drive).Free / 1GB, 1)
    if ($freeGb -lt 10) {
        Write-Check warn "Low disk space on drive $drive (~$freeGb GB free; recommend 10+ GB for images)"
        $warnings++
    } else {
        Write-Check ok "Disk space ok (~$freeGb GB free on drive $drive)"
    }
} catch {
    Write-Check warn 'Could not check disk space'
    $warnings++
}

if (-not $Quiet) {
    Write-Host ''
    if ($errors -gt 0) {
        Write-Host "preflight-deps: FAILED - fix $errors issue(s) above, then re-run." -ForegroundColor Red
        exit 1
    }
    Write-Host "preflight-deps: OK - $warnings warning(s). Run scripts/start-streamclone.ps1" -ForegroundColor Green
}
exit 0
