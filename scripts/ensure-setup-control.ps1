#Requires -Version 5.1
# Ensure the host-side setup-control HTTP helper is running (port 9191).
param(
    [string]$Root = '',
    [int]$Port = 9191
)

$ErrorActionPreference = 'Stop'
if ([string]::IsNullOrWhiteSpace($Root)) {
    $Root = Split-Path -Parent $PSScriptRoot
}

$controlScript = Join-Path $PSScriptRoot 'setup-control.ps1'
$pidFile = Join-Path $Root '.streamclone-setup-control.pid'

function Test-SetupControlHealth {
    param([int]$HealthPort = 9191)
    try {
        $resp = Invoke-WebRequest -Uri "http://127.0.0.1:$HealthPort/health" -UseBasicParsing -TimeoutSec 2
        if ($resp.StatusCode -eq 200) {
            $body = $resp.Content | ConvertFrom-Json
            return [bool]$body.ok
        }
    } catch { }
    return $false
}

function Test-SetupControlEnvStale {
    param([string]$StalePidFile)
    $envFile = Join-Path $Root '.env'
    if (-not (Test-Path $envFile)) { return $false }
    if (-not (Test-Path $StalePidFile)) { return $false }
    $raw = (Get-Content $StalePidFile -Raw).Trim()
    if ($raw -notmatch '^\d+$') { return $true }
    $proc = Get-Process -Id ([int]$raw) -ErrorAction SilentlyContinue
    if (-not $proc) { return $true }
    return ((Get-Item $envFile).LastWriteTimeUtc -gt $proc.StartTime.ToUniversalTime())
}

function Stop-StaleSetupControl {
    param([string]$StalePidFile)
    if (-not (Test-Path $StalePidFile)) { return }
    $raw = (Get-Content $StalePidFile -Raw).Trim()
    if ($raw -match '^\d+$') {
        $proc = Get-Process -Id ([int]$raw) -ErrorAction SilentlyContinue
        if ($proc) {
            try { Stop-Process -Id ([int]$raw) -Force -ErrorAction SilentlyContinue } catch { }
        }
    }
    Remove-Item $StalePidFile -Force -ErrorAction SilentlyContinue
}

if (Test-SetupControlHealth -HealthPort $Port) {
    if (-not (Test-SetupControlEnvStale -StalePidFile $pidFile)) {
        return
    }
    Stop-StaleSetupControl -StalePidFile $pidFile
}

if (Test-Path $pidFile) {
    Stop-StaleSetupControl -StalePidFile $pidFile
}

if (-not (Test-Path (Join-Path $Root '.env'))) {
    Write-Warning 'setup-control not started: missing .env (run setup first).'
    return
}

if (-not (Test-Path $controlScript)) {
    Write-Warning "setup-control not started: missing $controlScript"
    return
}

Start-Process -FilePath 'powershell.exe' `
    -ArgumentList @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-WindowStyle', 'Hidden', '-File', $controlScript, '-Port', "$Port") `
    -WorkingDirectory $Root | Out-Null

for ($i = 0; $i -lt 10; $i++) {
    Start-Sleep -Milliseconds 300
    if (Test-SetupControlHealth -HealthPort $Port) {
        return
    }
}

Write-Warning 'setup-control did not respond on port 9191. Use Start Streamclone.cmd or run scripts\ensure-setup-control.ps1'
