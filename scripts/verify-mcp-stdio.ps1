# Cursor-like MCP stdio check for all Streamclone servers (Windows host).
$ErrorActionPreference = "Stop"
$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$Init = '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"verify","version":"1.0"}}}'
$Note = '{"jsonrpc":"2.0","method":"notifications/initialized"}'
$List = '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'
$Expected = @{ codegraph = 6; stack = 6; data = 5 }
$Fail = $false

function Test-McpStdio {
  param([string]$Name, [string]$Script)
  $psi = New-Object System.Diagnostics.ProcessStartInfo
  $psi.FileName = "wsl.exe"
  $psi.Arguments = "--cd `"$Root`" bash $Script"
  $psi.UseShellExecute = $false
  $psi.RedirectStandardInput = $true
  $psi.RedirectStandardOutput = $true
  $psi.RedirectStandardError = $true
  $p = [Diagnostics.Process]::Start($psi)
  foreach ($line in @($Init, $Note, $List)) {
    $bytes = [Text.Encoding]::UTF8.GetBytes($line + "`n")
    $p.StandardInput.BaseStream.Write($bytes, 0, $bytes.Length)
  }
  $p.StandardInput.Close()
  $stdout = $p.StandardOutput.ReadToEnd()
  $stderr = $p.StandardError.ReadToEnd()
  if (-not $p.WaitForExit(25000)) {
    $p.Kill()
    Write-Host "FAIL $Name : timed out"
    return $false
  }
  $lines = ($stdout -split "`n" | Where-Object { $_.Trim() -ne "" })
  $tools = 0
  if ($lines.Count -gt 0) {
    try { $tools = (($lines[-1] | ConvertFrom-Json).result.tools).Count } catch { $tools = -1 }
  }
  $want = $Expected[$Name]
  if ($tools -eq $want -and [string]::IsNullOrEmpty($stderr)) {
    Write-Host "ok $Name : $tools tools, stderr empty"
    return $true
  }
  Write-Host "FAIL $Name : tools=$tools (want $want) stderr=[$stderr]"
  return $false
}

Write-Host "Streamclone MCP stdio verify (same path as .cursor/mcp.json)"
foreach ($name in @("codegraph", "stack", "data")) {
  if (-not (Test-McpStdio $name "scripts/$name-mcp.sh")) { $Fail = $true }
}
if ($Fail) { exit 1 }
Write-Host "All MCP servers OK for Cursor."
