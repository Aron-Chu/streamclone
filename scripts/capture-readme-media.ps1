#Requires -Version 5.1
# Capture README screenshots at 1920x1080 (16:9). Stack must be up at http://localhost:8090
$ErrorActionPreference = 'Stop'
Set-Location (Split-Path $PSScriptRoot -Parent)

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    Write-Error "Docker required - start the stack first (make bootstrap)."
}

$skipSync = if ($args -contains '--with-sync') { '0' } else { '1' }
if (Get-Command ffmpeg -ErrorAction SilentlyContinue) {
    $env:DOCS_FFMPEG = 'ffmpeg'
} else {
    $env:DOCS_FFMPEG = 'docker'
    Write-Host "Using Docker ffmpeg for GIF (jrottenberg/ffmpeg)."
}

Push-Location frontend
try {
    npx playwright install chromium 2>$null
    $env:DOCS_SKIP_SYNC = $skipSync
    npx playwright test e2e/docs-media.spec.ts
} finally {
    Pop-Location
}

Write-Host ""
Write-Host "Saved to docs/images/ - open README preview (Ctrl+Shift+V) to verify."
Write-Host "Manual override: save Win+Shift+S shots as docs/images/<name>.png at 1920x1080 browser width."
