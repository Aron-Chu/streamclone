# Regenerate frontend /config.js and reload VITE_CLIPPER_TOKEN when .env drifted.
# docker compose restart does NOT reload env_file — only --force-recreate does.
param(
    [string]$EnvFile = '.env',
    [switch]$SkipRecreate
)

$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent $PSScriptRoot
Set-Location $Root

. (Join-Path $PSScriptRoot 'lib\env.ps1')

$ok = Invoke-EnsureFrontendClipperConfig -EnvFile $EnvFile -SkipRecreate:$SkipRecreate
exit $(if ($ok) { 0 } else { 1 })
