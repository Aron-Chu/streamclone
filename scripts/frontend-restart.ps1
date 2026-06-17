#Requires -Version 5.1
# Windows helper when `make` is not on PATH. Runs make frontend-restart via WSL, or docker compose directly.
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
    make frontend-restart ENV_FILE=$EnvFile
    exit $LASTEXITCODE
}

if (Get-Command wsl -ErrorAction SilentlyContinue) {
    $makePath = (wsl -e bash -lc 'command -v make' 2>$null).Trim()
    if ($makePath) {
        Invoke-WslMake -Target 'frontend-restart'
        exit $LASTEXITCODE
    }
}

Write-Host 'frontend-restart: make/wsl unavailable - running docker compose steps directly...' -ForegroundColor Yellow

if (-not (Test-Path $EnvFile)) {
    if (Test-Path '.env.dev') { Copy-Item '.env.dev' $EnvFile }
    else { throw "Missing $EnvFile - run scripts/setup.ps1 first." }
}

$compose = @(
    'docker', 'compose', '--env-file', $EnvFile,
    '-f', 'deploy/docker-compose.yml',
    '-f', 'deploy/docker-compose.local-tunnel.yml'
)

& @compose build frontend
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

& @compose up -d --no-deps --force-recreate frontend local-proxy
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

& (Join-Path $PSScriptRoot 'ensure-setup-control.ps1')
& (Join-Path $PSScriptRoot 'ensure-localhost-relays.ps1') -Ports '8090'

Write-Host 'frontend-restart: done - hard-refresh http://localhost:8090' -ForegroundColor Green
