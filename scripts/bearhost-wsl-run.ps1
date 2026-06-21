#Requires -Version 5.1
# Run a repo bash script via WSL (BearHost ops from Windows make).
param(
    [Parameter(Mandatory = $true)]
    [string]$Script,

    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$ScriptArgs
)

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
$argStr = if ($ScriptArgs.Count -gt 0) { ' ' + ($ScriptArgs -join ' ') } else { '' }
$cmd = "${prefix}bash scripts/${Script}${argStr}"

wsl bash -lc "cd '$wslRepo' && $cmd"
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
