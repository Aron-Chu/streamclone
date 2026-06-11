#Requires -Version 5.1
param(
    [string]$Url = 'http://localhost:8090/',
    [int]$TimeoutSec = 300,
    [int]$IntervalSec = 3
)

$ErrorActionPreference = 'Stop'
$deadline = (Get-Date).AddSeconds($TimeoutSec)
$attempt = 0

Write-Host "Waiting for Streamclone at $Url (up to ${TimeoutSec}s)..." -ForegroundColor Cyan

while ((Get-Date) -lt $deadline) {
    $attempt++
    try {
        $resp = Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 5
        if ($resp.StatusCode -ge 200 -and $resp.StatusCode -lt 500) {
            Write-Host "  Streamclone is ready (attempt $attempt)" -ForegroundColor Green
            return
        }
    } catch {
        # keep polling
    }
    if ($attempt % 5 -eq 0) {
        Write-Host "  still starting... (attempt $attempt)" -ForegroundColor DarkGray
    }
    Start-Sleep -Seconds $IntervalSec
}

Write-Host ''
Write-Host 'Streamclone did not become ready in time.' -ForegroundColor Red
Write-Host 'Try: docker compose --env-file .env -f deploy/docker-compose.yml -f deploy/docker-compose.local-tunnel.yml ps'
Write-Host 'See: docs/install-desktop.md'
exit 1
