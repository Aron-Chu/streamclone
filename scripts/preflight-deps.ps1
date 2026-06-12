#Requires -Version 5.1
# Check prerequisites for Streamclone (non-developer friendly).
param(
    [switch]$InstallHints,
    [switch]$Quiet,
    [Alias('JsonSummary')]
    [switch]$Json,
    [string]$ImageTag = '',
    [int]$DockerInfoTimeoutSec = 15
)

$ErrorActionPreference = 'Stop'
$errors = 0
$warnings = 0
$blockedReasons = [System.Collections.Generic.List[string]]::new()
$summary = [ordered]@{
    blocked  = $false
    ok       = $true
    errors   = 0
    warnings = 0
    reason   = ''
    context  = ''
    engine   = ''
    ghcr     = 'skipped'
    checks   = @()
}

. (Join-Path $PSScriptRoot 'lib\env.ps1')

function Write-Check {
    param([string]$Status, [string]$Message)
    $summary.checks += [pscustomobject]@{ status = $Status; message = $Message }
    if ($Quiet -or $Json) { return }
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

function Add-BlockedReason {
    param([string]$Reason)
    if ($blockedReasons -notcontains $Reason) {
        [void]$blockedReasons.Add($Reason)
    }
}

function Test-PortFree {
    param([int]$Port)
    $listener = $null
    try {
        $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, $Port)
        $listener.Start()
        return $true
    } catch {
        return $false
    } finally {
        if ($listener) { $listener.Stop() }
    }
}

function Invoke-DockerInfoWithTimeout {
    param([int]$TimeoutSec = 15)
    return Invoke-EnvDockerCapturedWithTimeout -Arguments @('info') -TimeoutSec $TimeoutSec
}

function Get-DockerContextName {
    $result = Invoke-EnvDockerCapturedWithTimeout -Arguments @('context', 'show') -TimeoutSec 10
    if ($result.TimedOut -or $result.ExitCode -ne 0) { return '' }
    return (($result.Output | Select-Object -First 1) -as [string]).Trim()
}

function Resolve-PreflightImageTag {
    param([string]$Requested)
    if (-not [string]::IsNullOrWhiteSpace($Requested)) { return $Requested.Trim() }
    if (-not [string]::IsNullOrWhiteSpace($env:IMAGE_TAG)) { return $env:IMAGE_TAG.Trim() }
    $root = Split-Path -Parent $PSScriptRoot
    $envPath = Join-Path $root '.env'
    if (Test-Path $envPath) {
        $vals = Read-EnvKeyValueFile -Path $envPath
        if ($vals['IMAGE_TAG']) { return $vals['IMAGE_TAG'].Trim() }
    }
    $tag = Get-EnvReleaseVersionTag
    if ($tag) { return $tag }
    return 'latest'
}

if (-not $Quiet -and -not $Json) {
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
    Add-BlockedReason 'Git is not installed'
    if ($InstallHints) {
        Write-Host '  Install: winget install Git.Git' -ForegroundColor DarkGray
    }
}

# Docker CLI
$dockerExe = Get-EnvDockerExe
$engineRunning = $false
if (-not $dockerExe) {
    Write-Check fail 'Docker is not installed or not on PATH'
    $errors++
    Add-BlockedReason 'Docker is not installed or not on PATH'
    $summary.engine = 'missing'
    if ($InstallHints) {
        Write-Host '  Install Docker Desktop: https://docs.docker.com/desktop/setup/install/windows-install/' -ForegroundColor DarkGray
        Write-Host '  Or try: winget install Docker.DockerDesktop' -ForegroundColor DarkGray
    }
} else {
    Write-Check ok 'Docker CLI found'
    $info = Invoke-DockerInfoWithTimeout -TimeoutSec $DockerInfoTimeoutSec
    if ($info.TimedOut) {
        Write-Check fail "Docker Desktop Linux engine not responding (timed out after ${DockerInfoTimeoutSec}s)"
        $errors++
        Add-BlockedReason 'Docker Desktop Linux engine not responding'
        $summary.engine = 'timeout'
    } elseif ($info.ExitCode -ne 0) {
        Write-Check fail 'Docker is installed but not running - start Docker Desktop'
        $errors++
        Add-BlockedReason 'Docker Desktop is not running'
        $summary.engine = 'stopped'
    } else {
        Write-Check ok 'Docker engine is running'
        $engineRunning = $true
        $summary.engine = 'running'
    }

    if ($engineRunning) {
        try {
            $compose = Invoke-EnvDockerCapturedWithTimeout -Arguments @('compose', 'version') -TimeoutSec 10
            if ($compose.ExitCode -ne 0) { throw 'compose missing' }
            Write-Check ok 'Docker Compose v2 available'
        } catch {
            Write-Check fail 'docker compose is missing - update Docker Desktop'
            $errors++
            Add-BlockedReason 'docker compose is missing'
        }
    }

    if ($engineRunning) {
        $contextName = Get-DockerContextName
        $summary.context = $contextName
        if ($contextName) {
            if (($IsWindows -or $env:OS -match 'Windows') -and $contextName -ne 'desktop-linux') {
                Write-Check warn "Docker context is '$contextName' (expected desktop-linux on Windows)"
                $warnings++
            } else {
                Write-Check ok "Docker context: $contextName"
            }
        }
    }
}

# WSL2 hint (Docker Desktop on Windows) — bounded so a stuck WSL does not hang install
if ($IsWindows -or $env:OS -match 'Windows') {
    $wsl = Get-Command wsl -ErrorAction SilentlyContinue
    if ($wsl) {
        $wslResult = Invoke-EnvCapturedProcess -FilePath $wsl.Source -ArgumentList @('-l', '-v') -TimeoutSec 5
        if ($wslResult.TimedOut) {
            Write-Check warn 'WSL status check timed out (5s) - if Docker is slow, run: wsl --shutdown'
            $warnings++
        } elseif ($wslResult.Output -match 'Stopped') {
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
    Write-Check warn 'Port 8090 is in use — Streamclone may already be running (this is OK)'
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

# GHCR reachability is measured by benchmark-ghcr-pull.ps1. Keep preflight local
# so install/restart benchmarks are not blocked by registry-specific failures.

$summary.errors = $errors
$summary.warnings = $warnings
$summary.blocked = ($errors -gt 0)
$summary.ok = ($errors -eq 0)
$summary.reason = ($blockedReasons -join '; ')

if ($Json) {
    Write-Output ($summary | ConvertTo-Json -Compress -Depth 4)
} elseif (-not $Quiet) {
    Write-Host ''
    if ($errors -gt 0) {
        Write-Host "preflight-deps: FAILED - fix $errors issue(s) above, then re-run." -ForegroundColor Red
    } else {
        Write-Host "preflight-deps: OK - $warnings warning(s). Run scripts/start-streamclone.ps1" -ForegroundColor Green
    }
}

if ($errors -gt 0) { exit 1 }
exit 0
