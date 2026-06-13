#Requires -Version 5.1
# Wrapper: run package-release.sh via WSL (preferred) or Git Bash.
param(
    [string]$Version = ''
)

$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent $PSScriptRoot
. (Join-Path $PSScriptRoot 'wsl-path.ps1')

$envBlock = if ($Version) { "VERSION=$Version " } else { '' }
$wslRoot = Get-WslLinuxPath $Root
$inner = "cd '$wslRoot' && ${envBlock}bash scripts/package-release.sh"

$wsl = Get-Command wsl -ErrorAction SilentlyContinue
if ($wsl) {
    wsl bash -lc $inner
    exit $LASTEXITCODE
}

$bash = Get-Command bash -ErrorAction SilentlyContinue
if (-not $bash) {
    throw 'bash is required (WSL or Git for Windows). Install WSL or Git for Windows and retry.'
}
$gitRoot = $Root -replace '\\', '/'
& $bash.Source -lc "cd '$gitRoot' && ${envBlock}bash scripts/package-release.sh"
exit $LASTEXITCODE
