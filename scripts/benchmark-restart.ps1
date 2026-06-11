#Requires -Version 5.1
# Cached restart benchmark: Stop -> Start -> wait for directory HTTP 200.
param(
    [string]$InstallDir = '',
    [string]$Url = 'http://localhost:8090/',
    [int]$TimeoutSec = 120
)

$ErrorActionPreference = 'Stop'
if ([string]::IsNullOrWhiteSpace($InstallDir)) {
    $InstallDir = Join-Path $env:USERPROFILE 'streamclone'
}
if (-not (Test-Path $InstallDir)) {
    throw "Install dir not found: $InstallDir"
}

$Root = Split-Path -Parent $PSScriptRoot
$preflightFile = Join-Path $Root 'dist\benchmark-restart-preflight.json'
$previousNoBrowser = $env:STREAMCLONE_NO_BROWSER
$env:STREAMCLONE_NO_BROWSER = '1'

try {
    $preflightRaw = & (Join-Path $PSScriptRoot 'preflight-deps.ps1') -Json 2>&1 | Select-Object -Last 1
    $preflight = $preflightRaw | ConvertFrom-Json
    New-Item -ItemType Directory -Force -Path (Split-Path $preflightFile) | Out-Null
    $preflight | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath $preflightFile -Encoding UTF8
    if ($preflight.blocked) {
        Write-Host "Benchmark blocked: $($preflight.reason)" -ForegroundColor Red
        exit 2
    }

    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    Write-Host 'Stopping Streamclone...' -ForegroundColor Cyan
    & (Join-Path $InstallDir 'scripts\stop-streamclone.ps1')
    if ($LASTEXITCODE -ne 0) { throw 'stop-streamclone.ps1 failed' }
    $stopSec = [math]::Round($sw.Elapsed.TotalSeconds, 1)

    $sw.Restart()
    Write-Host 'Starting Streamclone...' -ForegroundColor Cyan
    & (Join-Path $InstallDir 'scripts\start-streamclone.ps1') -NoBrowser -SkipSetup
    if ($LASTEXITCODE -ne 0) { throw 'start-streamclone.ps1 failed' }
    $startSec = [math]::Round($sw.Elapsed.TotalSeconds, 1)

    $sw.Restart()
    $deadline = (Get-Date).AddSeconds($TimeoutSec)
    $ready = $false
    while ((Get-Date) -lt $deadline) {
        try {
            $resp = Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 5
            if ($resp.StatusCode -ge 200 -and $resp.StatusCode -lt 500) {
                $ready = $true
                break
            }
        } catch { }
        Start-Sleep -Seconds 2
    }
    $readySec = [math]::Round($sw.Elapsed.TotalSeconds, 1)

    Write-Host ''
    Write-Host '=== Streamclone cached restart benchmark ===' -ForegroundColor Cyan
    Write-Host "Stop time:        ${stopSec}s"
    Write-Host "Start time:       ${startSec}s"
    Write-Host "Directory ready:  $(if ($ready) { "${readySec}s (200 OK)" } else { 'FAILED' })"
    Write-Host "Stop->Start total: $([math]::Round($stopSec + $startSec + $readySec, 1))s"

    if (-not $ready) { exit 1 }
} finally {
    $env:STREAMCLONE_NO_BROWSER = $previousNoBrowser
}
