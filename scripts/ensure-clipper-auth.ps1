# Validate clipper Twitch token, auto-refresh when possible, recreate clipper if env drifted.
# docker compose restart does NOT reload env_file — only --force-recreate does.
param(
    [string]$EnvFile = '.env',
    [string]$ComposeFile = 'deploy/docker-compose.yml',
    [string]$ComposeTunnelFile = 'deploy/docker-compose.local-tunnel.yml',
    [string]$CliConfig = "$env:APPDATA\twitch-cli\.twitch-cli.env",
    [string]$LogPath = 'debug-4e0301.log',
    [string]$SessionId = '4e0301',
    [switch]$SkipRecreate
)

$ErrorActionPreference = 'Stop'

function Write-AgentLog {
    param(
        [string]$Location,
        [string]$Message,
        [hashtable]$Data,
        [string]$HypothesisId = 'H1'
    )
    $entry = @{
        sessionId   = $SessionId
        timestamp   = [int64]([DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds())
        location    = $Location
        message     = $Message
        data        = $Data
        hypothesisId = $HypothesisId
        runId       = 'ensure-clipper-auth'
    } | ConvertTo-Json -Compress
    try {
        Add-Content -Path $LogPath -Value $entry -Encoding utf8
    } catch {
        # ignore log write failures
    }
}

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

function Get-ContainerEnvValue {
    param([string]$Container, [string]$Key)
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

function Test-ClipperRunning {
    $state = docker inspect -f '{{.State.Status}}' 'streamclone-clipper-1' 2>$null
    return ($LASTEXITCODE -eq 0 -and $state -eq 'running')
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
Write-AgentLog -Location 'ensure-clipper-auth.ps1:start' -Message 'clipper token check' -Data @{
    before_ok = $before.ok
    token_len = if ($token) { $token.Length } else { 0 }
    has_refresh_in_env = -not [string]::IsNullOrWhiteSpace($envValues['CLIPPER_TWITCH_REFRESH_TOKEN'])
    has_refresh_in_cli = -not [string]::IsNullOrWhiteSpace($cli['REFRESHTOKEN'])
    clipper_running = (Test-ClipperRunning)
} -HypothesisId 'H1-H4'

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
            Write-AgentLog -Location 'ensure-clipper-auth.ps1:refresh' -Message 'clipper token refreshed' -Data @{
                after_ok = $before.ok
                expires_in = $before.expires_in
            } -HypothesisId 'H4'
            Write-Host "ensure-clipper-auth: refreshed clipper token (expires_in=$($before.expires_in)s)"
        } else {
            Write-AgentLog -Location 'ensure-clipper-auth.ps1:refresh' -Message 'refresh failed' -Data @{
                status = $refreshed.status
                error = $refreshed.error
            } -HypothesisId 'H4'
            Write-Host "ensure-clipper-auth: auto-refresh failed - run make twitch-local-auth and approve Twitch login" -ForegroundColor Yellow
        }
    } else {
        Write-Host "ensure-clipper-auth: token invalid and no refresh credentials - run make twitch-local-auth" -ForegroundColor Yellow
    }
}

if (-not $before.ok) {
    Write-AgentLog -Location 'ensure-clipper-auth.ps1:exit' -Message 'token still invalid' -Data @{ ok = $false } -HypothesisId 'H1'
    exit 1
}

if ($SkipRecreate -or -not (Test-ClipperRunning)) {
    Write-Host "ensure-clipper-auth: token valid in $EnvFile"
    exit 0
}

$containerToken = Get-ContainerEnvValue -Container 'streamclone-clipper-1' -Key 'CLIPPER_TWITCH_USER_ACCESS_TOKEN'
$containerValid = (Test-TwitchToken -Token $containerToken).ok
$needsRecreate = $tokenChanged -or ($containerToken -ne $token) -or (-not $containerValid)

Write-AgentLog -Location 'ensure-clipper-auth.ps1:container' -Message 'container drift check' -Data @{
    token_changed_in_env = $tokenChanged
    container_matches_env = ($containerToken -eq $token)
    container_valid = $containerValid
    needs_recreate = $needsRecreate
} -HypothesisId 'H2'

if (-not $needsRecreate) {
    Write-Host "ensure-clipper-auth: clipper container token matches valid .env"
    exit 0
}

Write-Host "ensure-clipper-auth: recreating clipper to load updated token..."
docker compose --env-file $EnvFile -f $ComposeFile -f $ComposeTunnelFile --profile clipper `
    up -d --no-deps --force-recreate clipper
if ($LASTEXITCODE -ne 0) {
    Write-Host "ensure-clipper-auth: clipper recreate failed (exit $LASTEXITCODE)" -ForegroundColor Red
    exit $LASTEXITCODE
}

$afterContainer = Get-ContainerEnvValue -Container 'streamclone-clipper-1' -Key 'CLIPPER_TWITCH_USER_ACCESS_TOKEN'
$afterValid = (Test-TwitchToken -Token $afterContainer).ok
Write-AgentLog -Location 'ensure-clipper-auth.ps1:done' -Message 'clipper recreated' -Data @{
    container_valid = $afterValid
    container_matches_env = ($afterContainer -eq $token)
} -HypothesisId 'H2'

Write-Host "ensure-clipper-auth: clipper recreated (container_valid=$afterValid)"
exit $(if ($afterValid) { 0 } else { 1 })
