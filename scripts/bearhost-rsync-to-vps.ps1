#Requires -Version 5.1
# Rsync local Streamclone checkout to BearHost VPS (via WSL rsync + SSH).
#
# Usage:
#   powershell -ExecutionPolicy Bypass -File scripts/bearhost-rsync-to-vps.ps1
#
# Env overrides (optional): BEARHOST_HOST, BEARHOST_USER, BEARHOST_SSH_KEY, etc.
# Passed through to scripts/bearhost-rsync-to-vps.sh in WSL.

$ErrorActionPreference = 'Stop'

. (Join-Path $PSScriptRoot 'wsl-path.ps1')
$paths = Get-RepoWslPath -StartPath $PSScriptRoot
$wslRepo = $paths.WslRepo

$envPass = @()
foreach ($name in @('BEARHOST_HOST', 'BEARHOST_USER', 'BEARHOST_SSH_KEY', 'BEARHOST_REMOTE_APP', 'BEARHOST_REMOTE_SCRAPER')) {
    $val = [Environment]::GetEnvironmentVariable($name)
    if (-not [string]::IsNullOrWhiteSpace($val)) {
        $envPass += "$name=$(($val -replace "'", "'\\''"))"
    }
}

$prefix = if ($envPass.Count -gt 0) { ($envPass -join ' ') + ' ' } else { '' }
$cmd = "${prefix}bash scripts/bearhost-rsync-to-vps.sh"

Write-Host "==> bearhost-rsync via WSL: $wslRepo"
wsl bash -lc "cd '$wslRepo' && $cmd"
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
