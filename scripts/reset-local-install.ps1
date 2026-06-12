#Requires -Version 5.1
# Contributor helper: reset data for a clean re-test without deleting the install folder.
param(
    [string]$InstallDir = (Split-Path -Parent $PSScriptRoot),
    [switch]$RemoveDesktopShortcuts,
    [switch]$PruneImages
)

$ErrorActionPreference = 'Stop'
$uninstall = Join-Path $PSScriptRoot 'uninstall-streamclone.ps1'
if (-not (Test-Path $uninstall)) {
    throw "Missing $uninstall"
}

$args = @{
    InstallDir         = $InstallDir
    NonInteractive     = $true
    KeepInstallDir     = $true
}
if ($PruneImages) { $args['PruneImages'] = $true }

& $uninstall @args

if ($RemoveDesktopShortcuts) {
    $desktop = [Environment]::GetFolderPath('Desktop')
    foreach ($name in @(
        'Streamclone.lnk',
        'Start Streamclone.lnk',
        'Stop Streamclone.lnk',
        'Manage Streamclone.lnk',
        'Check Streamclone.lnk',
        'Uninstall Streamclone.lnk'
    )) {
        $path = Join-Path $desktop $name
        if (Test-Path $path) {
            Remove-Item $path -Force
            Write-Host "Removed Desktop\$name"
        }
    }
}

Write-Host "Reset complete (install folder kept): $InstallDir" -ForegroundColor Green
