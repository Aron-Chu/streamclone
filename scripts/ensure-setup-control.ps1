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
$resolvedRoot = (Resolve-Path -LiteralPath $Root).Path

$controlScript = Join-Path $resolvedRoot 'scripts\setup-control.ps1'
if (-not (Test-Path $controlScript)) {
    $controlScript = Join-Path $PSScriptRoot 'setup-control.ps1'
}
$pidFile = Join-Path $resolvedRoot '.streamclone-setup-control.pid'

function Get-SetupControlHealthInfo {
    param([int]$HealthPort = 9191)
    try {
        $resp = Invoke-WebRequest -Uri "http://127.0.0.1:$HealthPort/health" -UseBasicParsing -TimeoutSec 2
        if ($resp.StatusCode -eq 200) {
            return ($resp.Content | ConvertFrom-Json)
        }
    } catch { }
    return $null
}

function Test-SetupControlHealth {
    param([int]$HealthPort = 9191)
    $body = Get-SetupControlHealthInfo -HealthPort $HealthPort
    return ($null -ne $body -and [bool]$body.ok)
}

function Get-SetupControlDaemonPids {
    Get-CimInstance Win32_Process -Filter "Name='powershell.exe'" -ErrorAction SilentlyContinue |
        Where-Object { $_.CommandLine -like '*setup-control.ps1*' } |
        ForEach-Object { [int]$_.ProcessId }
}

function Stop-SetupControlListeners {
    foreach ($procId in (Get-SetupControlDaemonPids)) {
        try { Stop-Process -Id $procId -Force -ErrorAction SilentlyContinue } catch { }
    }
}

function Test-SetupControlEnvStale {
    param([string]$StalePidFile)
    $envFile = Join-Path $resolvedRoot '.env'
    if (-not (Test-Path $envFile)) { return $false }
    if (-not (Test-Path $StalePidFile)) { return $false }
    $raw = (Get-Content $StalePidFile -Raw).Trim()
    if ($raw -notmatch '^\d+$') { return $true }
    $proc = Get-Process -Id ([int]$raw) -ErrorAction SilentlyContinue
    if (-not $proc) { return $true }
    return ((Get-Item $envFile).LastWriteTimeUtc -gt $proc.StartTime.ToUniversalTime())
}

function Test-SetupControlScriptStale {
    param([string]$StalePidFile)
    if (-not (Test-Path $StalePidFile)) { return $false }
    if (-not (Test-Path $controlScript)) { return $false }
    $raw = (Get-Content $StalePidFile -Raw).Trim()
    if ($raw -notmatch '^\d+$') { return $true }
    $proc = Get-Process -Id ([int]$raw) -ErrorAction SilentlyContinue
    if (-not $proc) { return $true }
    return ((Get-Item $controlScript).LastWriteTimeUtc -gt $proc.StartTime.ToUniversalTime())
}

function Test-SetupControlRootMismatch {
    param(
        [int]$HealthPort = 9191,
        [string]$ExpectedPidFile
    )
    $health = Get-SetupControlHealthInfo -HealthPort $HealthPort
    if (-not $health -or -not $health.ok) { return $false }

    $healthRoot = [string]$health.root
    if (-not [string]::IsNullOrWhiteSpace($healthRoot)) {
        try {
            $normalizedHealthRoot = (Resolve-Path -LiteralPath $healthRoot).Path
            if ($normalizedHealthRoot -ne $resolvedRoot) { return $true }
        } catch {
            return $true
        }
    }

    $daemonPids = @(Get-SetupControlDaemonPids)
    if ($daemonPids.Count -eq 0) { return $false }
    if (-not (Test-Path $ExpectedPidFile)) { return $true }

    $raw = (Get-Content $ExpectedPidFile -Raw).Trim()
    if ($raw -notmatch '^\d+$') { return $true }
    return ([int]$raw -notin $daemonPids)
}

function Stop-StaleSetupControl {
    param([string]$StalePidFile)
    Stop-SetupControlListeners
    if (-not (Test-Path $StalePidFile)) { return }
    Remove-Item $StalePidFile -Force -ErrorAction SilentlyContinue
}

$envStale = Test-SetupControlEnvStale -StalePidFile $pidFile
$scriptStale = Test-SetupControlScriptStale -StalePidFile $pidFile
$rootMismatch = Test-SetupControlRootMismatch -HealthPort $Port -ExpectedPidFile $pidFile

if (Test-SetupControlHealth -HealthPort $Port) {
    if (-not $envStale -and -not $scriptStale -and -not $rootMismatch) {
        return
    }
    Stop-StaleSetupControl -StalePidFile $pidFile
}

if (Test-Path $pidFile) {
    Stop-StaleSetupControl -StalePidFile $pidFile
}

if (-not (Test-Path (Join-Path $resolvedRoot '.env'))) {
    Write-Warning 'setup-control not started: missing .env (run setup first).'
    return
}

if (-not (Test-Path $controlScript)) {
    Write-Warning "setup-control not started: missing $controlScript"
    return
}

Start-Process -FilePath 'powershell.exe' `
    -ArgumentList @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-WindowStyle', 'Hidden', '-File', $controlScript, '-Port', "$Port", '-Root', $resolvedRoot) `
    -WorkingDirectory $resolvedRoot | Out-Null

for ($i = 0; $i -lt 10; $i++) {
    Start-Sleep -Milliseconds 300
    if (Test-SetupControlHealth -HealthPort $Port) {
        return
    }
}

Write-Warning 'setup-control did not respond on port 9191. Use Start Streamclone.cmd or run scripts\ensure-setup-control.ps1'
