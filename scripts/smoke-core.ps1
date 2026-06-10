#Requires -Version 5.1
$ErrorActionPreference = 'Stop'
Set-Location (Split-Path $PSScriptRoot -Parent)

$runUi = $args -contains '--ui'

function Wait-Url {
    param([string]$Url, [string]$Label)
    Write-Host "Checking $Label ($Url)..."
    for ($i = 1; $i -le 60; $i++) {
        try {
            Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 5 | Out-Null
            Write-Host "  ok"
            return
        } catch {
            Start-Sleep -Seconds 2
        }
    }
    throw "smoke-core: $Label failed - is the stack up? Run scripts/bootstrap.ps1 or make up."
}

Wait-Url "http://localhost:8090/" "Caddy proxy (frontend)"
Wait-Url "http://localhost:8081/healthz" "metadata"
Wait-Url "http://localhost:8082/healthz" "video"
Wait-Url "http://localhost:8083/healthz" "chat"
Wait-Url "http://localhost:8084/healthz" "emote"
Wait-Url "http://localhost:8086/healthz" "analytics"

if ($runUi) {
    Write-Host "Running Playwright smoke-core..."
    Push-Location frontend
    try {
        npm run test:smoke
    } finally {
        Pop-Location
    }
}

Write-Host "smoke-core: all checks passed"
