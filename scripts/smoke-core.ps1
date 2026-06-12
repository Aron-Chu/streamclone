#Requires -Version 5.1
$ErrorActionPreference = 'Stop'
$Root = Split-Path $PSScriptRoot -Parent
Set-Location $Root

$runUi = $args -contains '--ui'

. (Join-Path $PSScriptRoot 'lib\wait-stack.ps1')
if (-not (Wait-StreamcloneTieredReadiness -Root $Root -SkipHLS)) {
    Write-Host 'smoke-core: tiered readiness failed - is the stack up? Run scripts/bootstrap.ps1 or make up.' -ForegroundColor Red
    exit 1
}

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
