# Install / verify Figma MCP Bridge for Streamclone (Windows).
$ErrorActionPreference = "Stop"
$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$PluginManifest = Join-Path $Root "tools\figma-mcp-bridge-plugin\manifest.json"
$McpJson = Join-Path $Root ".cursor\mcp.json"
$CodexConfig = Join-Path $env:USERPROFILE ".codex\config.toml"

Write-Host "Streamclone - Figma MCP Bridge setup"
Write-Host "Repo: $Root"

if (-not (Test-Path $PluginManifest)) {
  Write-Host "FAIL: plugin manifest missing at $PluginManifest"
  Write-Host "Copy from release zip plugin folder into tools\figma-mcp-bridge-plugin"
  exit 1
}
Write-Host "ok plugin manifest: $PluginManifest"

$desired = @{
  mcpServers = @{
    "figma-bridge" = @{
      command = "npx"
      args    = @("-y", "@gethopp/figma-mcp-bridge@0.0.15")
    }
  }
}
$desiredJson = ($desired | ConvertTo-Json -Depth 6)

if (Test-Path $McpJson) {
  $existing = Get-Content $McpJson -Raw | ConvertFrom-Json
  if (-not $existing.mcpServers.'figma-bridge') {
    if (-not $existing.mcpServers) { $existing | Add-Member -NotePropertyName mcpServers -NotePropertyValue ([pscustomobject]@{}) }
    $existing.mcpServers | Add-Member -NotePropertyName 'figma-bridge' -NotePropertyValue $desired.mcpServers.'figma-bridge' -Force
    $existing | ConvertTo-Json -Depth 6 | Set-Content -Encoding utf8 $McpJson
    Write-Host "ok merged figma-bridge into existing .cursor/mcp.json"
  } else {
    Write-Host "ok figma-bridge already in .cursor/mcp.json"
  }
} else {
  $desiredJson | Set-Content -Encoding utf8 $McpJson
  Write-Host "ok wrote .cursor/mcp.json"
}

if (Test-Path $CodexConfig) {
  $codexText = Get-Content $CodexConfig -Raw
  if ($codexText -notmatch '(?m)^\[mcp_servers\.figma-bridge\]') {
    @'

[mcp_servers.figma-bridge]
command = "cmd.exe"
args = ["/c", "npx", "-y", "@gethopp/figma-mcp-bridge@0.0.15"]
startup_timeout_sec = 60
'@ | Add-Content -Encoding utf8 $CodexConfig
    Write-Host "ok added figma-bridge to Codex config: $CodexConfig"
  } else {
    Write-Host "ok figma-bridge already in Codex config: $CodexConfig"
  }
} else {
  Write-Host "warn Codex config not found at $CodexConfig; Cursor MCP was configured"
}

Write-Host ""
Write-Host "MCP handshake (stdio)..."
node (Join-Path $Root "scripts\verify-figma-bridge-mcp.cjs")
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host ""
Write-Host "=== Figma plugin (manual one-time step) ==="
Write-Host "1. Open Figma desktop - Pulse Wire file (YDMQSWrJHyA7g5D5pIfruH)"
Write-Host "2. Plugins - Development - Import plugin from manifest..."
Write-Host "3. Select: $PluginManifest"
Write-Host "4. Run plugin: Plugins - Development - Figma MCP Bridge"
Write-Host "5. Cursor - Settings - MCP - reload servers (or restart Cursor)"
Write-Host ""
Write-Host 'Then ask the agent: use figma-bridge list_files'
