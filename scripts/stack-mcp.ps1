# Manual terminal launch only. Cursor MCP uses wsl.exe via .cursor/mcp.json.
$ErrorActionPreference = "Stop"
$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
& wsl.exe --cd $Root bash scripts/stack-mcp.sh
