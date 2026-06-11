#Requires -Version 5.1
param(
    [string]$Dir = (Join-Path $env:USERPROFILE 'streamclone'),
    [ValidateSet('core', 'scraper', 'clipper', 'full')]
    [string]$Profile = 'core',
    [switch]$Release,
    [switch]$UseImages,
    [switch]$NonInteractive,
    [switch]$DesktopShortcut,
    [string]$Version = ''
)

$ErrorActionPreference = 'Stop'

& (Join-Path $PSScriptRoot 'preflight-deps.ps1') -InstallHints
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

if ($Release) {
    if (-not $PSBoundParameters.ContainsKey('UseImages')) { $UseImages = $true }
    if (-not $PSBoundParameters.ContainsKey('NonInteractive')) { $NonInteractive = $true }
    if ($NonInteractive -and -not $PSBoundParameters.ContainsKey('DesktopShortcut')) {
        $DesktopShortcut = $true
    }
    Write-Host 'Step 1/4: Downloading latest release...' -ForegroundColor Cyan
    $dlArgs = @{ InstallDir = $Dir }
    if ($Version) { $dlArgs['Version'] = $Version }
    & (Join-Path $PSScriptRoot 'lib\release-download.ps1') @dlArgs
} elseif (Test-Path (Join-Path $Dir '.git')) {
    Write-Host "Updating existing checkout at $Dir..."
    git -C $Dir pull --ff-only
} else {
    Write-Host "Cloning Streamclone to $Dir..."
    git clone https://github.com/Aron-Chu/streamclone.git $Dir
}

$setupArgs = @{
    Profile = $Profile
}
if ($NonInteractive) { $setupArgs['NonInteractive'] = $true }
if ($UseImages) { $setupArgs['UseImages'] = $true }

if ($Release) {
    Write-Host 'Step 2/4: Creating config and secrets...' -ForegroundColor Cyan
    Write-Host 'Step 3/4: Pulling Docker images and starting stack (~3-5 min)...' -ForegroundColor Cyan
}
& (Join-Path $Dir 'scripts\setup.ps1') @setupArgs
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

if ($DesktopShortcut) {
    Write-Host 'Step 4/4: Creating Desktop shortcuts...' -ForegroundColor Cyan
    & (Join-Path $Dir 'scripts\install-desktop-shortcut.ps1') -InstallDir $Dir
}

Write-Host ''
Write-Host "Installed at: $Dir"

if ($Release) {
    $startPs1 = Join-Path $Dir 'scripts\start-streamclone.ps1'
    if (Test-Path $startPs1) {
        Write-Host ''
        Write-Host 'Opening Streamclone in your browser...' -ForegroundColor Green
        & $startPs1
    }
} else {
    Write-Host 'Open:         http://localhost:8090'
    Write-Host "Start:        powershell -File `"$Dir\scripts\start-streamclone.ps1`""
    Write-Host "Stop stack:   powershell -File `"$Dir\scripts\stop-streamclone.ps1`""
}
