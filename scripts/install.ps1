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

& (Join-Path $Dir 'scripts\setup.ps1') @setupArgs
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

if ($DesktopShortcut) {
    & (Join-Path $Dir 'scripts\install-desktop-shortcut.ps1') -InstallDir $Dir
}

Write-Host ''
Write-Host "Installed at: $Dir"
Write-Host 'Open:         http://localhost:8090'
Write-Host "Start:        powershell -File `"$Dir\scripts\start-streamclone.ps1`""
Write-Host "Stop stack:   powershell -File `"$Dir\scripts\stop-streamclone.ps1`""
