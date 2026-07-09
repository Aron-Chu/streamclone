#Requires -Version 5.1
# Windows helper when `make` is not on PATH. Runs make frontend-refresh via WSL, or docker compose directly.
param(
    [string]$EnvFile = '.env'
)

$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent $PSScriptRoot
Set-Location $Root

function Invoke-WslMake {
    param([string]$Target)
    $linuxRoot = wsl -e wslpath -a $Root
    wsl -e bash -lc "cd '$linuxRoot' && make $Target ENV_FILE=$EnvFile"
}

if (Get-Command make -ErrorAction SilentlyContinue) {
    make frontend-refresh ENV_FILE=$EnvFile
    exit $LASTEXITCODE
}

if (Get-Command wsl -ErrorAction SilentlyContinue) {
    $makePath = (wsl -e bash -lc 'command -v make' 2>$null).Trim()
    if ($makePath) {
        Invoke-WslMake -Target 'frontend-refresh'
        exit $LASTEXITCODE
    }
}

Write-Host 'frontend-refresh: make/wsl unavailable - running docker compose steps directly...' -ForegroundColor Yellow

if (-not (Test-Path $EnvFile)) {
    & bash scripts/env-synthesize.sh core $EnvFile
    if ($LASTEXITCODE -ne 0) { throw "Failed to synthesize $EnvFile" }
}

Push-Location frontend
try {
    npm run build
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
}
finally {
    Pop-Location
}

$compose = @(
    'docker', 'compose', '--env-file', $EnvFile,
    '-f', 'deploy/docker-compose.yml',
    '-f', 'deploy/docker-compose.local-tunnel.yml'
)

& @compose run --rm migrate
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

& @compose build frontend chat
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

& @compose up -d --no-deps --force-recreate frontend chat local-proxy
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

& (Join-Path $PSScriptRoot 'ensure-setup-control.ps1')
& (Join-Path $PSScriptRoot 'ensure-localhost-relays.ps1') -Ports '8090'

Write-Host 'frontend-refresh: done - hard-refresh http://localhost:8090' -ForegroundColor Green
