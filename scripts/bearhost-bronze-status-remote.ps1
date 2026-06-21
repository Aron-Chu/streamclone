#Requires -Version 5.1
# Run bronze/VOD status on BearHost VPS (SSH from Windows via WSL).
#
# Usage:
#   powershell -ExecutionPolicy Bypass -File scripts/bearhost-bronze-status-remote.ps1

$ErrorActionPreference = 'Stop'

. (Join-Path $PSScriptRoot 'wsl-path.ps1')
$paths = Get-RepoWslPath -StartPath $PSScriptRoot
$wslRepo = $paths.WslRepo

$envPass = @()
foreach ($name in @('BEARHOST_HOST', 'BEARHOST_USER', 'BEARHOST_SSH_KEY', 'BEARHOST_REMOTE_APP')) {
    $val = [Environment]::GetEnvironmentVariable($name)
    if (-not [string]::IsNullOrWhiteSpace($val)) {
        $envPass += "$name=$(($val -replace "'", "'\\''"))"
    }
}

$prefix = if ($envPass.Count -gt 0) { ($envPass -join ' ') + ' ' } else { '' }
$cmd = "${prefix}bash scripts/bearhost-bronze-status-remote.sh"

Write-Host "==> bearhost bronze status (remote VPS) via WSL"
wsl bash -lc "cd '$wslRepo' && $cmd"
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
