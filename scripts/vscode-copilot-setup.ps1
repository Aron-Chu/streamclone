# Enable VS Code Copilot with Streamclone MCP + custom agents (Windows).
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "wsl-path.ps1")
. (Join-Path $PSScriptRoot "vscode-write-mcp.ps1")
$paths = Get-RepoWslPath
$repoWin = $paths.Repo
$repoWsl = $paths.WslRepo

Write-Host "Streamclone VS Code Copilot setup"
Write-Host "repo: $repoWin"

Install-StreamcloneVsCodeMcp -RepoWinPath $repoWin -RepoWslPath $repoWsl

wsl bash -lc "bash '$repoWsl/scripts/vscode-copilot-sync-agents.sh'"
wsl bash -lc "bash '$repoWsl/scripts/codex-sync-skills.sh'"

Write-Host ""
Write-Host "Running MCP preflight..."
& (Join-Path $PSScriptRoot "mcp-preflight.ps1")

$userMcp = Join-Path $env:APPDATA "Code\User\mcp.json"
Write-Host ""
Write-Host "VS Code Copilot setup - next steps:"
Write-Host "  1. Open streamclone-pulse-extension.code-workspace (recommended) or twitch-7tv-clone folder."
Write-Host "  2. Command Palette: Developer: Reload Window"
Write-Host "  3. Command Palette: MCP: List Servers (streamcloneCodegraph, stack, data, playwright)"
Write-Host "  4. Trust and start each server when prompted."
Write-Host "  5. Copilot Chat agent picker: backend-safety-reviewer, ops-diagnostics-reviewer, frontend-ux-reviewer."
Write-Host ""
Write-Host "If Workspace servers still missing:"
Write-Host "  - Command Palette: MCP: Open User Configuration"
Write-Host "  - User MCP file: $userMcp"
Write-Host "  - Set chat.mcp.access to all in workspace or user settings"
