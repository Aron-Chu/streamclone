#Requires -Version 5.1
# Capture README screenshots at 1920x1080 (16:9). Stack must be up at http://localhost:8090
param(
    [switch]$WithSync,
    [switch]$AnalyticsOnly,
    [switch]$ResetPostgres
)

$ErrorActionPreference = 'Stop'
Set-Location (Split-Path $PSScriptRoot -Parent)

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    Write-Error "Docker required - start the stack first (make bootstrap)."
}

$compose = @(
    'compose', '--env-file', '.env',
    '-f', 'deploy/docker-compose.yml',
    '-f', 'deploy/docker-compose.local-tunnel.yml'
)

if ($ResetPostgres) {
    Write-Host "Resetting Postgres (removing pg-data volume)..."
    docker @compose down --remove-orphans -v --timeout 30
    if ($LASTEXITCODE -ne 0) { throw "docker compose down failed with exit $LASTEXITCODE" }
    if ($AnalyticsOnly) {
        Write-Host "Starting core + scraper stack (TwitchTracker GIF needs scraper profile)..."
        docker @compose --profile scraper up -d --build --remove-orphans
    } else {
        docker @compose up -d --build --remove-orphans
    }
    Write-Host "Waiting for analytics health..."
    for ($i = 1; $i -le 90; $i++) {
        try {
            Invoke-WebRequest -Uri 'http://localhost:8086/healthz' -UseBasicParsing -TimeoutSec 5 | Out-Null
            Write-Host "  analytics ok"
            break
        } catch {
            Start-Sleep -Seconds 2
        }
    }
    if ($AnalyticsOnly) {
        for ($i = 1; $i -le 90; $i++) {
            try {
                Invoke-WebRequest -Uri 'http://localhost:8000/health' -UseBasicParsing -TimeoutSec 5 | Out-Null
                Write-Host "  scraper ok"
                break
            } catch {
                Start-Sleep -Seconds 2
            }
        }
    }
}

$skipSync = if ($WithSync -or $AnalyticsOnly) { '0' } else { '1' }
if (Get-Command ffmpeg -ErrorAction SilentlyContinue) {
    $env:DOCS_FFMPEG = 'ffmpeg'
} else {
    $env:DOCS_FFMPEG = 'docker'
    Write-Host "Using Docker ffmpeg for GIF (jrottenberg/ffmpeg)."
}

Remove-Item Env:DOCS_ANALYTICS_STREAM -ErrorAction SilentlyContinue
$env:DOCS_SKIP_SYNC = $skipSync

Push-Location frontend
try {
    npx playwright install chromium 2>$null
    if ($AnalyticsOnly) {
        npx playwright test e2e/docs-media-analytics.spec.ts
    } else {
        npx playwright test e2e/docs-media.spec.ts
    }
} finally {
    Pop-Location
}

Write-Host ""
Write-Host "Saved to docs/images/ - open README preview (Ctrl+Shift+V) to verify."
Write-Host "Analytics load GIFs: scripts/capture-readme-media.ps1 -AnalyticsOnly -ResetPostgres"
Write-Host "Manual override: save Win+Shift+S shots as docs/images/<name>.png at 1920x1080 browser width."
