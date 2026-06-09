# Verify Streamclone MCP prerequisites on Windows (wraps WSL).
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "wsl-path.ps1")
$paths = Get-RepoWslPath
$WslRepo = $paths.WslRepo
wsl bash -lc "bash '$WslRepo/scripts/mcp-preflight.sh'"
