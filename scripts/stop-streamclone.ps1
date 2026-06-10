#Requires -Version 5.1
$ErrorActionPreference = 'Stop'
Set-Location (Split-Path -Parent $PSScriptRoot)

Write-Host 'Stopping Streamclone...' -ForegroundColor Cyan
docker compose --env-file .env `
    -f deploy/docker-compose.yml `
    -f deploy/docker-compose.local-tunnel.yml `
    --profile scraper --profile clipper `
    down --remove-orphans --timeout 30 2>$null

docker compose --env-file .env `
    -f deploy/docker-compose.yml `
    -f deploy/docker-compose.local-tunnel.yml `
    down --remove-orphans --timeout 30

Write-Host 'Stopped.' -ForegroundColor Green
