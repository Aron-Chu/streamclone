# Post-edit agent validation on Windows (WSL bash wrapper).
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "wsl-path.ps1")
$paths = Get-RepoWslPath
$WslRepo = $paths.WslRepo
wsl bash -lc "bash '$WslRepo/scripts/agent-smoke.sh'"
