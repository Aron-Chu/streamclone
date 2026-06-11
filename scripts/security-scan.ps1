#Requires -Version 5.1
# Local security checks aligned with CI (gitleaks + env validation).
$ErrorActionPreference = 'Stop'
Set-Location (Split-Path $PSScriptRoot -Parent)

Write-Host '==> gitleaks'
if (Get-Command gitleaks -ErrorAction SilentlyContinue) {
    gitleaks detect --source . --config .gitleaks.toml --verbose --redact
} elseif (Get-Command pre-commit -ErrorAction SilentlyContinue) {
    pre-commit run gitleaks --all-files
} elseif (Get-Command wsl -ErrorAction SilentlyContinue) {
    wsl -e bash -lc "cd /mnt/c/Users/Aron/twitch-7tv-clone && pre-commit run gitleaks --all-files"
} else {
    Write-Error 'Install gitleaks, pre-commit, or WSL (make install-hooks)'
}

Write-Host '==> validate-env'
& (Join-Path $PSScriptRoot 'validate-env.ps1')

Write-Host 'security-scan ok'
