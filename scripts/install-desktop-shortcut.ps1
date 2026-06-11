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
    'Start Streamclone.cmd',
    'Stop Streamclone.cmd',
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

foreach ($name in @('Start Streamclone.cmd', 'Stop Streamclone.cmd', 'Uninstall Streamclone.cmd')) {
    $src = Join-Path (Split-Path -Parent $PSScriptRoot) $name
    if (-not (Test-Path $src)) { $src = Join-Path $repoLaunchers $name }
    if (Test-Path $src) {
        $dest = Join-Path $Root $name
        if ((Resolve-Path -LiteralPath $src).Path -ne (Resolve-Path -LiteralPath $dest -ErrorAction SilentlyContinue).Path) {
            Copy-Item -Path $src -Destination $dest -Force
        }
    }
}

try {
    $shell = New-Object -ComObject WScript.Shell
    $desktop = [Environment]::GetFolderPath('Desktop')
    foreach ($pair in @(
        @{ Name = 'Start Streamclone.lnk'; Target = Join-Path $Root 'Start Streamclone.cmd' }
        @{ Name = 'Stop Streamclone.lnk'; Target = Join-Path $Root 'Stop Streamclone.cmd' }
    )) {
        $lnk = Join-Path $desktop $pair.Name
        $sc = $shell.CreateShortcut($lnk)
        $sc.TargetPath = $pair.Target
        $sc.WorkingDirectory = $Root
        $sc.Description = if ($pair.Name -like 'Start*') {
            'Start Streamclone and open http://localhost:8090/'
        } else {
            'Stop all Streamclone Docker containers'
        }
        $sc.Save()
    }
    Write-Host "Desktop shortcuts created on $desktop"
} catch {
    Write-Host "Could not create .lnk shortcuts (use Start/Stop .cmd in $Root): $_" -ForegroundColor Yellow
}

Write-Host "Launchers: $targetLaunchers"
