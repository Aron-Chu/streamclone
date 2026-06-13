#Requires -Version 5.1
# Finish Streamclone uninstall when Docker was offline during Uninstall Streamclone.cmd.
param(
    [string]$StateFile = ''
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'lib\env.ps1')
. (Join-Path $PSScriptRoot 'lib\install-upgrade.ps1')

if ([string]::IsNullOrWhiteSpace($StateFile)) {
    $StateFile = Join-Path $env:LOCALAPPDATA 'Streamclone\pending-docker-uninstall.json'
}

function Test-StreamcloneDockerEngineRunning {
    $result = Invoke-EnvDockerCapturedWithTimeout -Arguments @('info') -TimeoutSec 10
    return (-not $result.TimedOut -and $result.ExitCode -eq 0)
}

function Wait-StreamcloneDockerEngine {
    param([int]$WaitSec = 120)
    $preflight = Join-Path $PSScriptRoot 'preflight-deps.ps1'
    if (-not (Test-Path $preflight)) { return (Test-StreamcloneDockerEngineRunning) }
    $raw = & $preflight -Quiet -Json -TryStartDocker 2>&1 | Select-Object -Last 1
    try {
        $summary = $raw | ConvertFrom-Json
        if ($summary.dockerEngineRunning) { return $true }
    } catch { }
    $deadline = (Get-Date).AddSeconds($WaitSec)
    while ((Get-Date) -lt $deadline) {
        if (Test-StreamcloneDockerEngineRunning) { return $true }
        Start-Sleep -Seconds 3
    }
    return $false
}

if (-not (Test-Path $StateFile)) {
    Write-Host 'No pending Docker cleanup found (already finished or never scheduled).' -ForegroundColor Yellow
    exit 0
}

$state = Get-Content -LiteralPath $StateFile -Raw | ConvertFrom-Json
$root = [string]$state.installDir
if (-not (Test-Path (Join-Path $root 'deploy\docker-compose.yml'))) {
    Write-Host "Install folder missing: $root" -ForegroundColor Red
    Remove-Item -LiteralPath $StateFile -Force -ErrorAction SilentlyContinue
    exit 1
}

Write-Host 'Streamclone - Finish Docker cleanup' -ForegroundColor Cyan
Write-Host "Install folder: $root"
Write-Host ''

if (-not (Test-StreamcloneDockerEngineRunning)) {
    Write-Host 'Docker Desktop is not running. Starting/waiting...' -ForegroundColor Yellow
    if (-not (Wait-StreamcloneDockerEngine)) {
        Write-Host 'Docker is still unavailable. Start Docker Desktop manually and run this script again.' -ForegroundColor Red
        exit 1
    }
}

$uninstall = Join-Path $PSScriptRoot 'uninstall-streamclone.ps1'
if (-not (Test-Path $uninstall)) {
    throw "Missing $uninstall"
}

$args = @{
    InstallDir         = $root
    NonInteractive     = $true
    SkipImagePrompt    = $true
    KeepInstallDir     = $false
}
if ($state.removeImages -eq $true) { $args['PruneImages'] = $true }
if ($state.removeBaseImages -eq $true) { $args['PruneBaseImages'] = $true }
if ($state.keepVolumes -eq $true) { $args['KeepVolumes'] = $true }

& $uninstall @args
$code = $LASTEXITCODE
if ($code -ne 0) { exit $code }

Remove-Item -LiteralPath $StateFile -Force -ErrorAction SilentlyContinue
$desktopCmd = Join-Path ([Environment]::GetFolderPath('Desktop')) 'Finish Streamclone Docker cleanup.cmd'
if (Test-Path $desktopCmd) {
    Remove-Item -LiteralPath $desktopCmd -Force -ErrorAction SilentlyContinue
}

Write-Host ''
Write-Host 'Docker cleanup complete.' -ForegroundColor Green
