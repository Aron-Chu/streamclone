#Requires -Version 5.1
# Wrapper: run package-release.sh via Git Bash or WSL.
param(
    [string]$Version = ''
)

$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent $PSScriptRoot
$bash = Get-Command bash -ErrorAction SilentlyContinue
if (-not $bash) {
    throw 'bash is required (Git for Windows or WSL). Install Git for Windows and retry.'
}
$envBlock = if ($Version) { "VERSION=$Version " } else { '' }
& $bash.Source -lc "cd '$($Root -replace '\\','/')' && ${envBlock}bash scripts/package-release.sh"
exit $LASTEXITCODE
