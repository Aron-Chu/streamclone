#Requires -Version 5.1
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'lib\env.ps1')

Set-Location (Get-EnvRepoRoot)
Test-EnvPreflightDocker

$envFile = Join-Path (Get-Location) '.env'
if (-not (Test-Path $envFile)) {
    Invoke-EnvSynthesize -Profile core -OutFile $envFile
    Write-Host 'Created .env from .env.dev + profile-core (secrets generated).'
} else {
    Invoke-EnvGenerateSecrets -EnvFile $envFile
}

Write-Host 'Starting core stack (no scraper/clipper profiles)...'
docker compose --env-file .env `
    -f deploy/docker-compose.yml `
    -f deploy/docker-compose.local-tunnel.yml `
    up -d --build --remove-orphans

Write-Host ''
Write-Host 'Streamclone is starting at http://localhost:8090'
Write-Host "Run 'powershell -File scripts/smoke-core.ps1' once services are healthy."
