# Remote laptopworker stack control from Windows (Tailscale SSH).
# Works in Windows PowerShell 5.1 (powershell.exe) and PowerShell 7 (pwsh).
#
# From repo root:
#   powershell -ExecutionPolicy Bypass -File scripts\laptopworker-remote.ps1 status
# Or double-click / PATH:
#   scripts\laptopworker-remote.cmd status
param(
    [Parameter(Position = 0)]
    [ValidateSet('start', 'stop', 'restart', 'status', 'smoke', 'logs', 'update', 'install-service')]
    [string]$Command = 'status',

    [string]$SshHost = 'aron@laptopworker',
    [string]$RemoteDir = '~/streamclone'
)

$ErrorActionPreference = 'Stop'
$remote = "cd $RemoteDir && bash scripts/laptopworker-stack.sh $Command"
Write-Host "ssh $SshHost -> laptopworker-stack.sh $Command" -ForegroundColor Cyan
& ssh -o BatchMode=yes $SshHost $remote
exit $LASTEXITCODE
