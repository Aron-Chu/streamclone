#Requires -Version 5.1
# Run one BearHost Grafana tunnel health check (restart if needed).
# Invoked from Task Scheduler (StreamcloneGrafanaTunnelWatch); must not flash a console.
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'wsl-path.ps1')
$paths = Get-RepoWslPath -StartPath $PSScriptRoot
$cmd = "cd '$($paths.WslRepo)' && bash scripts/bearhost-grafana-tunnel-watch.sh"
$exitCode = Invoke-WslBashLcSilent -Command $cmd
if ($exitCode -ne 0) { exit $exitCode }
