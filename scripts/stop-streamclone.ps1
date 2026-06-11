#Requires -Version 5.1
$ErrorActionPreference = 'Stop'
Set-Location (Split-Path -Parent $PSScriptRoot)
. (Join-Path $PSScriptRoot 'lib\env.ps1')

$Root = Split-Path -Parent $PSScriptRoot
$controlPidFile = Join-Path $Root '.streamclone-setup-control.pid'
if (Test-Path $controlPidFile) {
    $controlPid = (Get-Content $controlPidFile -Raw).Trim()
    if ($controlPid -match '^\d+$') {
        Stop-Process -Id ([int]$controlPid) -Force -ErrorAction SilentlyContinue
    }
    Remove-Item $controlPidFile -Force -ErrorAction SilentlyContinue
}

Write-Host 'Stopping Streamclone...' -ForegroundColor Cyan
$prev = $ErrorActionPreference
$ErrorActionPreference = 'Continue'
try {
    $withProfiles = @(
        'compose', '--env-file', '.env',
        '-f', 'deploy/docker-compose.yml',
        '-f', 'deploy/docker-compose.local-tunnel.yml',
        '--profile', 'scraper', '--profile', 'clipper',
        'down', '--remove-orphans', '--timeout', '30'
    )
    $result = Invoke-EnvDockerCaptured -Arguments $withProfiles
    foreach ($line in $result.Output) { Write-Host $line }

    $core = @(
        'compose', '--env-file', '.env',
        '-f', 'deploy/docker-compose.yml',
        '-f', 'deploy/docker-compose.local-tunnel.yml',
        'down', '--remove-orphans', '--timeout', '30'
    )
    $result = Invoke-EnvDockerCaptured -Arguments $core
    foreach ($line in $result.Output) { Write-Host $line }
} finally {
    $ErrorActionPreference = $prev
}

Write-Host 'Stopped.' -ForegroundColor Green
