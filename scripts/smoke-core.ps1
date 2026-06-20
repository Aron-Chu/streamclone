#Requires -Version 5.1
param(
    [switch]$SkipReadiness
)

$ErrorActionPreference = 'Stop'
$Root = Split-Path $PSScriptRoot -Parent
Set-Location $Root

$runUi = $args -contains '--ui'
if ($args -contains '--skip-readiness') { $SkipReadiness = $true }

. (Join-Path $PSScriptRoot 'lib\wait-stack.ps1')
if (-not $SkipReadiness) {
    if (-not (Wait-StreamcloneTieredReadiness -Root $Root -SkipHLS)) {
        Write-Host 'smoke-core: tiered readiness failed - is the stack up? Run scripts/bootstrap.ps1 or make up.' -ForegroundColor Red
        exit 1
    }
}

if ($runUi) {
    Write-Host "Running Playwright smoke-core..."
    Push-Location frontend
    try {
        npm run test:smoke
        if ($LASTEXITCODE -ne 0) {
            exit $LASTEXITCODE
        }
    } finally {
        Pop-Location
    }
}

Write-Host "smoke-core: all checks passed"
