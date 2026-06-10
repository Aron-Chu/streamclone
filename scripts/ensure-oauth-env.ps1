# Sync TWITCH_OAUTH_* from twitch-cli into .env when missing.
# Exit 0 = ok (already present or synced); exit 1 = cannot sync (no CLI config).
param(
    [string]$EnvFile = '.env',
    [string]$CliConfig = "$env:APPDATA\twitch-cli\.twitch-cli.env"
)

$ErrorActionPreference = 'Stop'

function Read-KeyValueFile {
    param([string]$Path)
    $values = @{}
    if (-not (Test-Path $Path)) { return $values }
    foreach ($line in Get-Content $Path) {
        if ($line -match '^(?<key>[A-Z0-9_]+)=(?<value>.*)$') {
            $values[$matches['key']] = $matches['value']
        }
    }
    return $values
}

function Set-EnvValue {
    param([string[]]$Lines, [string]$Key, [string]$Value)
    $prefix = "$Key="
    for ($index = 0; $index -lt $Lines.Length; $index++) {
        if ($Lines[$index].StartsWith($prefix)) {
            $Lines[$index] = $prefix + $Value
            return ,$Lines
        }
    }
    return @($Lines + ($prefix + $Value))
}

if (-not (Test-Path $EnvFile)) {
    Write-Host "ensure-oauth-env: missing $EnvFile (run make setup first)"
    exit 1
}

$envValues = Read-KeyValueFile -Path $EnvFile
if (-not [string]::IsNullOrWhiteSpace($envValues['TWITCH_OAUTH_CLIENT_ID']) -and
    -not [string]::IsNullOrWhiteSpace($envValues['TWITCH_OAUTH_CLIENT_SECRET'])) {
    Write-Host "ensure-oauth-env: TWITCH_OAUTH_* already in $EnvFile"
    exit 0
}

if (-not (Test-Path $CliConfig)) {
    Write-Host "ensure-oauth-env: TWITCH_OAUTH_* missing and twitch-cli config not found at $CliConfig"
    Write-Host "  run: twitch configure   then: make twitch-sync"
    exit 1
}

$cli = Read-KeyValueFile -Path $CliConfig
$clientId = $cli['CLIENTID']
$clientSecret = $cli['CLIENTSECRET']
if ([string]::IsNullOrWhiteSpace($clientId) -or [string]::IsNullOrWhiteSpace($clientSecret)) {
    Write-Host "ensure-oauth-env: twitch-cli config missing CLIENTID/CLIENTSECRET — run twitch configure"
    exit 1
}

$lines = [string[]](Get-Content $EnvFile)
$lines = Set-EnvValue -Lines $lines -Key 'TWITCH_OAUTH_CLIENT_ID' -Value $clientId
$lines = Set-EnvValue -Lines $lines -Key 'TWITCH_OAUTH_CLIENT_SECRET' -Value $clientSecret
Set-Content -Path $EnvFile -Value $lines

Write-Host "ensure-oauth-env: synced TWITCH_OAUTH_CLIENT_ID/SECRET from twitch-cli into $EnvFile"
exit 0
