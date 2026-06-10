#Requires -Version 5.1
$ErrorActionPreference = 'Stop'
Set-Location (Split-Path $PSScriptRoot -Parent)

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    Write-Error "Docker is required. Install Docker Desktop and ensure 'docker' is on PATH."
}

if (-not (Test-Path .env)) {
    Copy-Item .env.dev .env
    $token = -join ((1..48) | ForEach-Object { '{0:x2}' -f (Get-Random -Maximum 256) })
    (Get-Content .env -Raw) -replace '(?m)^CURATOR_API_TOKEN=.*', "CURATOR_API_TOKEN=$token" | Set-Content .env -NoNewline
    Write-Host "Created .env from .env.dev (random CURATOR_API_TOKEN)."
}

Write-Host "Starting core stack (no scraper/clipper profiles)..."
docker compose --env-file .env `
    -f deploy/docker-compose.yml `
    -f deploy/docker-compose.local-tunnel.yml `
    up -d --build --remove-orphans

Write-Host ""
Write-Host "Streamclone is starting at http://localhost:8090"
Write-Host "Run 'powershell -File scripts/smoke-core.ps1' once services are healthy."
