#Requires -Version 5.1
param(
    [string]$InstallDir = (Join-Path $env:USERPROFILE 'streamclone')
)

$ErrorActionPreference = 'Stop'
$Root = if ($InstallDir) { $InstallDir } else { Split-Path -Parent $PSScriptRoot }
$repoLaunchers = Join-Path (Split-Path -Parent $PSScriptRoot) 'launchers'
$targetLaunchers = Join-Path $Root 'launchers'

if (-not (Test-Path (Join-Path $Root 'scripts\start-streamclone.ps1'))) {
    throw "Install dir missing scripts: $Root"
}

New-Item -ItemType Directory -Path $targetLaunchers -Force | Out-Null
foreach ($name in @(
    'install-streamclone-launcher.ps1',
    'Install Streamclone.cmd',
    'Check Streamclone.cmd',
    'Start Streamclone.cmd',
    'Stop Streamclone.cmd',
    'Manage Streamclone.cmd',
    'Uninstall Streamclone.cmd'
)) {
    $src = Join-Path $repoLaunchers $name
    if (-not (Test-Path $src)) { $src = Join-Path $PSScriptRoot $name }
    if (Test-Path $src) {
        $dest = Join-Path $targetLaunchers $name
        if ((Resolve-Path -LiteralPath $src).Path -ne (Resolve-Path -LiteralPath $dest -ErrorAction SilentlyContinue).Path) {
            Copy-Item -Path $src -Destination $dest -Force
        }
    }
}

foreach ($name in @('Start Streamclone.cmd', 'Stop Streamclone.cmd', 'Manage Streamclone.cmd', 'Uninstall Streamclone.cmd', 'Check Streamclone.cmd')) {
    $src = Join-Path (Split-Path -Parent $PSScriptRoot) $name
    if (-not (Test-Path $src)) { $src = Join-Path $repoLaunchers $name }
    if (Test-Path $src) {
        $dest = Join-Path $Root $name
        if ((Resolve-Path -LiteralPath $src).Path -ne (Resolve-Path -LiteralPath $dest -ErrorAction SilentlyContinue).Path) {
            Copy-Item -Path $src -Destination $dest -Force
        }
    }
}

$legacyShortcutNames = @(
    'Start Streamclone.lnk',
    'Stop Streamclone.lnk',
    'Manage Streamclone.lnk',
    'Check Streamclone.lnk',
    'Uninstall Streamclone.lnk'
)

try {
    $shell = New-Object -ComObject WScript.Shell
    $desktop = [Environment]::GetFolderPath('Desktop')
    foreach ($legacy in $legacyShortcutNames) {
        $legacyPath = Join-Path $desktop $legacy
        if (Test-Path $legacyPath) {
            Remove-Item -LiteralPath $legacyPath -Force
            Write-Host "Removed legacy Desktop\$legacy"
        }
    }

    $iconPath = Join-Path $Root 'deploy\installer\icon.ico'
    $lnk = Join-Path $desktop 'Streamclone.lnk'
    $sc = $shell.CreateShortcut($lnk)
    $sc.TargetPath = Join-Path $Root 'Manage Streamclone.cmd'
    $sc.WorkingDirectory = $Root
    $sc.Description = 'Start, stop, and manage Streamclone'
    if (Test-Path $iconPath) {
        $sc.IconLocation = "$iconPath,0"
    }
    $sc.Save()
    Write-Host "Desktop shortcut created: $lnk"
} catch {
    Write-Host "Could not create .lnk shortcut (use Manage Streamclone.cmd in $Root): $_" -ForegroundColor Yellow
}

Write-Host "Launchers: $targetLaunchers"
