#Requires -Version 5.1
# Probe ReplayForge API and print install hint when Clip Studio is requested via deprecated clipper profile.
param(
    [string]$HealthUrl = 'http://127.0.0.1:8095/healthz',
    [string]$DocsRelPath = 'docs/agents-streamclone-and-replayforge.md'
)

$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
$docsPath = Join-Path $repoRoot $DocsRelPath

$reachable = $false
try {
    $resp = Invoke-WebRequest -Uri $HealthUrl -UseBasicParsing -TimeoutSec 3
    if ($resp.StatusCode -ge 200 -and $resp.StatusCode -lt 300) {
        $reachable = $true
    }
} catch {
    $reachable = $false
}

if ($reachable) {
    Write-Host 'ReplayForge: running (Clip Studio API at http://127.0.0.1:8095)' -ForegroundColor Green
    exit 0
}

Write-Host ''
Write-Host 'The clipper compose profile is deprecated — Clip Studio runs in ReplayForge.' -ForegroundColor Yellow
Write-Host 'Install and start ReplayForge separately (sibling checkout ../replayforge).'
Write-Host "Integration guide: $DocsRelPath"
if (Test-Path $docsPath) {
    Write-Host "  ($docsPath)" -ForegroundColor DarkGray
}
Write-Host ''
Write-Host "ReplayForge not reachable at $HealthUrl — start ReplayForge before using Clip Studio." -ForegroundColor DarkYellow
exit 0
