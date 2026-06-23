# DEPRECATED — Clip Studio runs in ReplayForge (../replayforge on host :8095), not Streamclone compose.
# See docs/agents-streamclone-and-replayforge.md. Kept for legacy callers only; do not add new references.
#
# Validate clipper Twitch token in .env and auto-refresh when possible.
# After refresh, restart ReplayForge so it picks up CLIPPER_TWITCH_* from .env.
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

function Test-TwitchToken {
    param([string]$Token)
    if ([string]::IsNullOrWhiteSpace($Token)) {
        return @{ ok = $false; reason = 'missing' }
    }
    try {
        $response = Invoke-RestMethod -Uri 'https://id.twitch.tv/oauth2/validate' `
            -Headers @{ Authorization = "Bearer $($Token.Trim())" } `
            -Method Get -TimeoutSec 10
        return @{
            ok         = $true
            client_id  = $response.client_id
            scopes     = @($response.scopes)
            expires_in = $response.expires_in
        }
    } catch {
        $status = $null
        if ($_.Exception.Response) { $status = [int]$_.Exception.Response.StatusCode }
        return @{ ok = $false; reason = 'invalid'; status = $status }
    }
}

function Invoke-TwitchRefresh {
    param(
        [string]$ClientId,
        [string]$ClientSecret,
        [string]$RefreshToken
    )
    $body = @{
        grant_type    = 'refresh_token'
        refresh_token = $RefreshToken
        client_id     = $ClientId
        client_secret = $ClientSecret
    }
    try {
        return Invoke-RestMethod -Uri 'https://id.twitch.tv/oauth2/token' `
            -Method Post -Body $body -ContentType 'application/x-www-form-urlencoded' -TimeoutSec 15
    } catch {
        return @{ error = 'refresh_failed'; status = [int]$_.Exception.Response.StatusCode }
    }
}

if (-not (Test-Path $EnvFile)) {
    Write-Host "ensure-clipper-auth: missing $EnvFile"
    exit 1
}

$envValues = Read-KeyValueFile -Path $EnvFile
$cli = Read-KeyValueFile -Path $CliConfig
$clientId = $envValues['TWITCH_OAUTH_CLIENT_ID']
if ([string]::IsNullOrWhiteSpace($clientId)) { $clientId = $cli['CLIENTID'] }
$clientSecret = $envValues['TWITCH_OAUTH_CLIENT_SECRET']
if ([string]::IsNullOrWhiteSpace($clientSecret)) { $clientSecret = $cli['CLIENTSECRET'] }

$token = $envValues['CLIPPER_TWITCH_USER_ACCESS_TOKEN']
if ([string]::IsNullOrWhiteSpace($token)) { $token = $cli['ACCESSTOKEN'] }

$before = Test-TwitchToken -Token $token

$tokenChanged = $false
if (-not $before.ok) {
    $refreshToken = $envValues['CLIPPER_TWITCH_REFRESH_TOKEN']
    if ([string]::IsNullOrWhiteSpace($refreshToken)) { $refreshToken = $cli['REFRESHTOKEN'] }

    if (-not [string]::IsNullOrWhiteSpace($clientId) -and
        -not [string]::IsNullOrWhiteSpace($clientSecret) -and
        -not [string]::IsNullOrWhiteSpace($refreshToken)) {
        Write-Host "ensure-clipper-auth: token invalid - attempting refresh via Twitch OAuth..."
        $refreshed = Invoke-TwitchRefresh -ClientId $clientId -ClientSecret $clientSecret -RefreshToken $refreshToken
        $newToken = [string]$refreshed.access_token
        if (-not [string]::IsNullOrWhiteSpace($newToken)) {
            $lines = [string[]](Get-Content $EnvFile)
            $lines = Set-EnvValue -Lines $lines -Key 'CLIPPER_TWITCH_USER_ACCESS_TOKEN' -Value $newToken
            $lines = Set-EnvValue -Lines $lines -Key 'CLIPPER_TWITCH_CLIENT_ID' -Value $clientId
            if ($refreshed.refresh_token) {
                $lines = Set-EnvValue -Lines $lines -Key 'CLIPPER_TWITCH_REFRESH_TOKEN' -Value ([string]$refreshed.refresh_token)
            }
            Set-Content -Path $EnvFile -Value $lines
            $token = $newToken
            $tokenChanged = $true
            $before = Test-TwitchToken -Token $token
            Write-Host "ensure-clipper-auth: refreshed clipper token (expires_in=$($before.expires_in)s)"
        } else {
            Write-Host "ensure-clipper-auth: auto-refresh failed - run make twitch-local-auth and approve Twitch login" -ForegroundColor Yellow
        }
    } else {
        Write-Host "ensure-clipper-auth: token invalid and no refresh credentials - run make twitch-local-auth" -ForegroundColor Yellow
    }
}

function Complete-EnsureClipperAuth {
    param([int]$ExitCode = 0)
    if ($ExitCode -eq 0) {
        & powershell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot 'ensure-frontend-config.ps1') -EnvFile $EnvFile 2>$null | Out-Host
    }
    exit $ExitCode
}

if (-not $before.ok) {
    exit 1
}

Write-Host "ensure-clipper-auth: token valid in $EnvFile"
if ($tokenChanged) {
    Write-Host "ensure-clipper-auth: restart ReplayForge (../replayforge on host :8095) to load the updated token." -ForegroundColor Yellow
}
Complete-EnsureClipperAuth -ExitCode 0
