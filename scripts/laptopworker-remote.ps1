# Remote laptopworker stack control from Windows (Tailscale SSH).
param(
    [Parameter(Position = 0)]
    [ValidateSet(
        'start', 'stop', 'restart', 'status', 'smoke', 'logs', 'update', 'install-service',
        'ufw-tailnet', 'enable-linger', 'boot-check', 'setup', 'setup-verify'
    )]
    [string]$Command = 'status',

    [string]$SshHost = 'aron@laptopworker',
    [string]$RemoteDir = '~/streamclone'
)

$ErrorActionPreference = 'Stop'
$remote = "cd $RemoteDir && bash scripts/laptopworker-stack.sh $Command"
$interactive = @('setup', 'install-service') -contains $Command

Write-Host "ssh $SshHost -> laptopworker-stack.sh $Command" -ForegroundColor Cyan
if ($interactive) {
    Write-Host "One-time setup - enter laptop sudo password when prompted (at your Windows desk)." -ForegroundColor Yellow
    & ssh -t $SshHost $remote
} else {
    & ssh -o BatchMode=yes $SshHost $remote
}
exit $LASTEXITCODE
