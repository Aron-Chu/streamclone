# Export Pulse extension Figma frames to docs/pulse-extension/figma/ via figma-bridge MCP.
# Prereq: Figma desktop open on "Streamclone Pulse — Chrome Extension" + Figma MCP Bridge plugin running.
$ErrorActionPreference = "Stop"
$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$OutDir = Join-Path $Root "docs\pulse-extension\figma"
$PulseRepo = Join-Path (Split-Path $Root -Parent) "streamclone-pulse"
$PulseFigma = Join-Path $PulseRepo "docs\pulse-extension\figma"

New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

Write-Host "Streamclone - export Pulse extension Figma PNGs"
Write-Host "Output: $OutDir"
Write-Host ""
Write-Host "Ensure Figma desktop has the Pulse extension file open and Figma MCP Bridge is running."
Write-Host "Run setup-figma-bridge.ps1 first if Codex/Cursor MCP is not configured."
Write-Host ""

node (Join-Path $Root "scripts\export-pulse-extension-figma.cjs")
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

if (Test-Path $PulseFigma) {
  Copy-Item -Path (Join-Path $OutDir "*") -Destination $PulseFigma -Force
  Write-Host "ok copied PNGs to $PulseFigma"
} else {
  Write-Host "warn streamclone-pulse not found at $PulseRepo — skipped copy"
}

Write-Host "ok export complete"
