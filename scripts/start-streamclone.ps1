#Requires -Version 5.1
# One-command start for non-developers: check deps, setup if needed, start stack, open browser.
param(
    [ValidateSet('core', 'scraper', 'clipper', 'full')]
    [string]$Profile = '',
    [switch]$UseImages,
    [switch]$NoBrowser,
    [switch]$SkipSetup
)

$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent $PSScriptRoot
Set-Location $Root

function Test-StreamcloneUseImagesDefault {
    param([string]$Base)
    if (Test-Path (Join-Path $Base 'VERSION')) { return $true }
    if ($env:STREAMCLONE_USE_IMAGES -eq '1') { return $true }
    $envPath = Join-Path $Base '.env'
    if (Test-Path $envPath) {
        . (Join-Path $PSScriptRoot 'lib\env.ps1')
        $vals = Read-EnvKeyValueFile -Path $envPath
        if ($vals['STREAMCLONE_USE_IMAGES'] -eq '1') { return $true }
    }
    return $false
}

& (Join-Path $PSScriptRoot 'preflight-deps.ps1') -InstallHints
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

$profileFile = Join-Path $Root '.streamclone-profile'
if ([string]::IsNullOrWhiteSpace($Profile) -and (Test-Path $profileFile)) {
    $Profile = (Get-Content $profileFile -Raw).Trim()
}
if ([string]::IsNullOrWhiteSpace($Profile)) {
    $Profile = 'core'
}

$pullImages = $UseImages.IsPresent
if (-not $UseImages.IsPresent) {
    $pullImages = Test-StreamcloneUseImagesDefault -Base $Root
}

$envFile = Join-Path $Root '.env'
if (-not $SkipSetup -and -not (Test-Path $envFile)) {
    Write-Host ''
    Write-Host "First run - running setup (profile: $Profile)..." -ForegroundColor Cyan
    $setupArgs = @{ Profile = $Profile; NonInteractive = $true }
    if ($pullImages) { $setupArgs['UseImages'] = $true }
    & (Join-Path $PSScriptRoot 'setup.ps1') @setupArgs
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
} elseif (-not (Test-Path $envFile)) {
    throw "Missing .env - run: powershell -File scripts/setup.ps1"
} else {
    Write-Host "Starting Streamclone (profile: $Profile)..." -ForegroundColor Cyan
    if ($pullImages) {
        Write-Host 'Using pre-built GHCR images (release bundle or STREAMCLONE_USE_IMAGES=1).'
    }
    . (Join-Path $PSScriptRoot 'lib\env.ps1')
    Repair-StreamcloneCaddyfileLocalTunnel -Root $Root | Out-Null
    Ensure-StreamcloneInstallId -EnvFile $envFile | Out-Null
    Ensure-LocalhostDevTokenImport -EnvFile $envFile | Out-Null
    $composeArgs = @(
        'compose', '--env-file', '.env',
        '-f', 'deploy/docker-compose.yml',
        '-f', 'deploy/docker-compose.local-tunnel.yml'
    )
    if ($pullImages) {
        $composeArgs += '-f', 'deploy/docker-compose.release.yml'
    }
    foreach ($p in (Get-EnvComposeProfiles -Profile $Profile)) {
        $composeArgs += '--profile', $p
    }
    $upArgs = @('up', '-d', '--remove-orphans')
    if ($pullImages) { $upArgs += '--pull', 'missing' } else { $upArgs += '--build' }
    $code = Invoke-EnvDocker -Arguments ($composeArgs + $upArgs)
    if ($code -ne 0) { exit $code }

    & powershell -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot 'reload-env-if-stale.ps1') -EnvFile $envFile 2>$null
    if ($Profile -in @('clipper', 'full')) {
        & powershell -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot 'ensure-clipper-auth.ps1') -EnvFile $envFile 2>$null
    }
}

& powershell -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot 'validate-env.ps1') -Profile $Profile -EnvFile $envFile 2>&1 | Out-Host
$null = $LASTEXITCODE

& (Join-Path $PSScriptRoot 'lib\wait-stack.ps1') -SkipHLS
if (-not $?) { exit 1 }

. (Join-Path $PSScriptRoot 'lib\env.ps1')
& powershell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot 'ensure-localhost-relays.ps1') -Ports 8090
if (-not (Test-StreamcloneWebReachable -Url 'http://localhost:8090/' -TimeoutSec 3) -and (Test-StreamcloneWebReachable -Url (Get-StreamcloneAppUrl))) {
    Write-Host ''
    Write-Host 'Note: http://localhost:8090 fails on this PC (WSL port relay). Use http://127.0.0.1:8090/' -ForegroundColor Yellow
}

if (Test-Path (Join-Path $Root 'VERSION')) {
    . (Join-Path $PSScriptRoot 'lib\install-upgrade.ps1')
    try {
        Update-StreamcloneBootstrapOverlayFromMaster -Dir $Root
    } catch {
        Write-Host "  script refresh skipped: $($_.Exception.Message)" -ForegroundColor DarkYellow
    }
}

$controlPidFile = Join-Path $Root '.streamclone-setup-control.pid'
$controlScript = Join-Path $PSScriptRoot 'setup-control.ps1'
& powershell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot 'ensure-setup-control.ps1') -Root $Root -RequireProxy
if (-not $?) { exit 1 }


Write-Host ''
Write-Host ("Streamclone is running at {0}" -f (Get-StreamcloneAppUrl)) -ForegroundColor Green
Write-Host 'Stop:  powershell -File scripts/stop-streamclone.ps1'
if ($Profile -in @('clipper', 'full')) {
    Write-Host ("Clips: open {0} and click Sign in (optional) (one-time Twitch login)" -f (Get-StreamcloneAppUrl))
    Write-Host '      or: powershell -File scripts/twitch-auth.ps1 -Action local-auth  (Twitch CLI)'
}

if (-not $NoBrowser) {
    Start-Process (Get-StreamcloneAppUrl)
}
