#Requires -Version 5.1
# Start BearHost Grafana SSH tunnel in background (port 3001).
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'wsl-path.ps1')
$paths = Get-RepoWslPath -StartPath $PSScriptRoot
wsl bash -lc "cd '$($paths.WslRepo)' && bash scripts/bearhost-grafana-tunnel-start.sh"
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
