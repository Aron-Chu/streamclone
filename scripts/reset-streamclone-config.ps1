#Requires -Version 5.1
# Reset Streamclone config only — keeps install folder and Docker volumes.
param(
    [string]$InstallDir = '',
    [switch]$NonInteractive
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'lib\env.ps1')
. (Join-Path $PSScriptRoot 'lib\stack-progress.ps1')

function Get-ResetInstallRoot {
    param([string]$Hint)
    $candidates = @()
    if ($Hint) { $candidates += $Hint.TrimEnd('\', '/') }
    $candidates += (Join-Path $env:USERPROFILE 'streamclone')
    $candidates += (Split-Path -Parent $PSScriptRoot)
    foreach ($candidate in $candidates) {
        if (Test-Path (Join-Path $candidate 'scripts\start-streamclone.ps1')) {
            return (Resolve-Path -LiteralPath $candidate).Path
        }
    }
    throw 'Streamclone install not found.'
}

$root = Get-ResetInstallRoot -Hint $InstallDir

if (-not $NonInteractive) {
    Write-Host ''
    Write-Host 'Reset Streamclone configuration' -ForegroundColor Cyan
    Write-Host '================================' -ForegroundColor Cyan
    Write-Host "Install folder: $root"
    Write-Host ''
    Write-Host 'This will:'
    Write-Host '  - Stop Docker containers (volumes kept)'
    Write-Host '  - Remove .env, profile, and setup-control state'
    Write-Host '  - Keep the install folder and database/MinIO data'
    Write-Host ''
    Write-Host 'Next Start or Install will re-run setup.'
    Write-Host ''
    $ans = Read-Host 'Continue? [y/N]'
    if ($ans -notmatch '^[Yy]') {
        Write-Host 'Cancelled.' -ForegroundColor Yellow
        exit 2
    }
}

$stopScript = Join-Path $PSScriptRoot 'stop-streamclone.ps1'
if (Test-Path $stopScript) {
    & $stopScript
}

$envFile = Join-Path $root '.env'
if (Test-Path $envFile) {
    $profile = Get-StreamcloneProfileFromRoot -Root $root
    $composeArgs = Get-StreamcloneComposeArgs -Root $root -Profile $profile
    Write-Host 'Removing containers (keeping volumes)...' -ForegroundColor Cyan
    Invoke-EnvDockerCaptured -Arguments ($composeArgs + @('down', '--remove-orphans', '--timeout', '30')) | Out-Null
}

$controlPidFile = Join-Path $root '.streamclone-setup-control.pid'
if (Test-Path $controlPidFile) {
    $controlPid = (Get-Content $controlPidFile -Raw).Trim()
    if ($controlPid -match '^\d+$') {
        Stop-Process -Id ([int]$controlPid) -Force -ErrorAction SilentlyContinue
    }
    Remove-Item $controlPidFile -Force -ErrorAction SilentlyContinue
}

Get-CimInstance Win32_Process -Filter "Name='powershell.exe'" -ErrorAction SilentlyContinue |
    Where-Object { $_.CommandLine -like '*setup-control.ps1*' -and $_.CommandLine -notlike '*ensure-setup-control.ps1*' } |
    ForEach-Object {
        try { Stop-Process -Id ([int]$_.ProcessId) -Force -ErrorAction SilentlyContinue } catch { }
    }

foreach ($name in @('.env', '.streamclone-profile')) {
    $path = Join-Path $root $name
    if (Test-Path $path) {
        Remove-Item $path -Force
        Write-Host "Removed $name"
    }
}

Write-Host ''
Write-Host 'Config reset complete. Run Start Streamclone or Install to set up again.' -ForegroundColor Green
