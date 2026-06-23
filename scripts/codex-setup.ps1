# Generate project-local Codex config (MCP paths) and sync skills for Codex.
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "wsl-path.ps1")
$paths = Get-RepoWslPath
$repoWin = $paths.Repo
$repoWsl = $paths.WslRepo

$codexDir = Join-Path $repoWin ".codex"
$example = Join-Path $codexDir "config.toml.example"
$target = Join-Path $codexDir "config.toml"

if (-not (Test-Path $example)) {
    throw "missing $example - run from streamclone repo root"
}

$content = Get-Content $example -Raw
$content = $content.Replace("__REPO_WIN__", $repoWin.Replace('\', '\\'))
$content = $content.Replace("__REPO_WSL__", $repoWsl)
Set-Content -Path $target -Value $content -NoNewline -Encoding utf8
Write-Host "wrote $target (repo Win=$repoWin Wsl=$repoWsl)"

wsl bash -lc "bash '$repoWsl/scripts/codex-sync-skills.sh'"

Write-Host ""
Write-Host "Codex setup - next steps:"
Write-Host "  1. Trust this repo in Codex (required for project .codex/ to load)."
Write-Host "     In Codex: add trusted project path, or ~/.codex/config.toml trusted_projects."
Write-Host "  2. Reload Codex / run /mcp to verify streamclone-* servers."
Write-Host "  3. Optional: merge docs/codex/global-config.toml.example into ~/.codex/config.toml"
Write-Host "  4. make mcp-setup  (if codegraph tools are empty)"
Write-Host "  See docs/CODEX.md"
