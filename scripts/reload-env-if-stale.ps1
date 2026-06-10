# Recreate env-sensitive services when container TWITCH_OAUTH_* differs from .env.
# docker compose restart does NOT reload env_file — only --force-recreate does.
param(
    [string]$EnvFile = '.env',
    [string[]]$Services = @('chat', 'metadata', 'analytics', 'emote'),
    [string]$ComposeFile = 'deploy/docker-compose.yml',
    [string]$ComposeTunnelFile = 'deploy/docker-compose.local-tunnel.yml'
)

$ErrorActionPreference = 'Stop'

function Read-EnvValue {
    param([string]$Path, [string]$Key)
    if (-not (Test-Path $Path)) { return '' }
    foreach ($line in Get-Content $Path) {
        if ($line -match "^$([regex]::Escape($Key))=(.*)$") {
            return $matches[1].Trim()
        }
    }
    return ''
}

function Get-ContainerEnvValue {
    param([string]$Container, [string]$Key)
    if (-not $Container) { return '' }
    $state = docker inspect -f '{{.State.Status}}' $Container 2>$null
    if ($LASTEXITCODE -ne 0 -or $state -ne 'running') { return $null }
    $lines = docker inspect $Container --format '{{range .Config.Env}}{{println .}}{{end}}' 2>$null
    if ($LASTEXITCODE -ne 0) { return $null }
    $prefix = "$Key="
    foreach ($line in $lines) {
        if ($line.StartsWith($prefix)) {
            return $line.Substring($prefix.Length)
        }
    }
    return ''
}

$desiredId = Read-EnvValue -Path $EnvFile -Key 'TWITCH_OAUTH_CLIENT_ID'
if ([string]::IsNullOrWhiteSpace($desiredId)) {
    Write-Host "reload-env-if-stale: TWITCH_OAUTH_CLIENT_ID not in $EnvFile - skip (run make twitch-sync)"
    exit 0
}

$project = 'streamclone'
$stale = @()
$checks = @(
    @{ Service = 'emote'; Container = "${project}-emote-1" },
    @{ Service = 'metadata'; Container = "${project}-metadata-1" },
    @{ Service = 'analytics'; Container = "${project}-analytics-1" },
    @{ Service = 'chat'; Container = "${project}-chat-1" }
)

foreach ($check in $checks) {
    if ($check.Service -notin $Services) { continue }
    $actual = Get-ContainerEnvValue -Container $check.Container -Key 'TWITCH_OAUTH_CLIENT_ID'
    if ($null -eq $actual) { continue }
    if ($actual -ne $desiredId) {
        $stale += $check.Service
    }
}

if ($stale.Count -eq 0) {
    Write-Host "reload-env-if-stale: container OAuth env matches .env"
    exit 0
}

Write-Host "reload-env-if-stale: stale OAuth in [$($stale -join ', ')] - force-recreating: $($Services -join ' ')"
docker compose --env-file $EnvFile -f $ComposeFile -f $ComposeTunnelFile up -d --no-deps --force-recreate @Services
if ($LASTEXITCODE -ne 0) {
    Write-Host "reload-env-if-stale: docker compose recreate failed (exit $LASTEXITCODE)" -ForegroundColor Red
    exit $LASTEXITCODE
}
Write-Host "reload-env-if-stale: recreated $($Services -join ' ')"
exit 0
