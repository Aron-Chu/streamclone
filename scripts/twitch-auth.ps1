param(
    [ValidateSet('version', 'configure', 'token', 'sync-env', 'local-auth')]
    [string]$Action = 'token',
    [string]$Scopes = 'chat:read chat:edit user:read:follows clips:edit',
    [string]$EnvFile = '.env',
    [string]$CliConfig = "$env:APPDATA\twitch-cli\.twitch-cli.env",
    [string]$ChatHttp = 'http://localhost:8090',
    [switch]$NoOpen
)

function Read-KeyValueFile {
    param([string]$Path)

    $values = @{}
    foreach ($line in Get-Content $Path) {
        if ($line -match '^(?<key>[A-Z0-9_]+)=(?<value>.*)$') {
            $values[$matches['key']] = $matches['value']
        }
    }
    return $values
}

function Set-EnvValue {
    param(
        [string[]]$Lines,
        [string]$Key,
        [string]$Value
    )

    $prefix = "$Key="
    for ($index = 0; $index -lt $Lines.Length; $index++) {
        if ($Lines[$index].StartsWith($prefix)) {
            $Lines[$index] = $prefix + $Value
            return ,$Lines
        }
    }

    return @($Lines + ($prefix + $Value))
}

function Strip-Ansi {
    param([string]$Value)
    return [regex]::Replace($Value, "`e\[[0-9;]*m", '')
}

function Normalize-TokenValue {
    param([string]$Value)
    if ([string]::IsNullOrWhiteSpace($Value)) {
        return ''
    }
    return (Strip-Ansi -Value $Value).Trim().Trim('"', "'", '`', ',', ';')
}

function Extract-Token {
    param(
        [string]$Text,
        [string[]]$Patterns
    )

    foreach ($pattern in $Patterns) {
        $match = [regex]::Match($Text, $pattern)
        if ($match.Success -and $match.Groups.Count -gt 1) {
            $token = Normalize-TokenValue -Value $match.Groups[1].Value
            if (-not [string]::IsNullOrWhiteSpace($token)) {
                return $token
            }
        }
    }
    return ''
}

function Parse-TokenOutput {
    param([object[]]$Lines)

    $text = Strip-Ansi -Value (($Lines | ForEach-Object { "$_" }) -join "`n")
    $linePrefix = '(?:^|\n)\s*(?:\d{4}/\d{2}/\d{2}\s+\d{2}:\d{2}:\d{2}\s+)?'
    $accessToken = Extract-Token -Text $text -Patterns @(
        '(?im)"access_token"\s*:\s*"([^"]+)"',
        "(?im)$linePrefix(?:user\\s+access\\s+token|access[_\\s-]*token)\\s*[:=]\\s*(\\S+)",
        '(?im)user\s+access\s+token\s*:\s*(\S+)',
        '(?im)access[_\s-]*token\s*[:=]\s*(\S+)'
    )
    $refreshToken = Extract-Token -Text $text -Patterns @(
        '(?im)"refresh_token"\s*:\s*"([^"]+)"',
        "(?im)$linePrefix(?:refresh[_\\s-]*token)\\s*[:=]\\s*(\\S+)",
        '(?im)refresh\s+token\s*:\s*(\S+)'
    )

    [pscustomobject]@{
        AccessToken = $accessToken
        RefreshToken = $refreshToken
    }
}

function Parse-DeviceLoginURL {
    param([string]$Line)

    return Extract-Token -Text (Strip-Ansi -Value $Line) -Patterns @(
        '(?im)use this url to log in:\s*(https?://\S+)',
        '(?im)(https://www\.twitch\.tv/activate\?device-code=\S+)'
    )
}

function Parse-TokenFailureMessage {
    param([object[]]$Lines)

    $text = Strip-Ansi -Value (($Lines | ForEach-Object { "$_" }) -join "`n")

    if ($text -match '(?im)device code .* expired') {
        return 'The Twitch device-code login expired before approval. Run local-auth again to open a fresh Twitch approval page.'
    }
    if ($text -match '(?im)(authorization|access)\s+(was\s+)?denied') {
        return 'The Twitch device-code login was denied. Run local-auth again and approve the Twitch prompt.'
    }

    return ''
}

function Join-Url {
    param(
        [string]$Base,
        [string]$Path
    )
    return $Base.TrimEnd('/') + $Path
}

$env:Path = [Environment]::GetEnvironmentVariable('Path', 'Machine') + ';' + [Environment]::GetEnvironmentVariable('Path', 'User')

$command = Get-Command twitch -ErrorAction Stop

switch ($Action) {
    'version' {
        & $command.Source version
    }
    'configure' {
        & $command.Source configure
    }
    'token' {
        & $command.Source token -u --dcf -s $Scopes
    }
    'local-auth' {
        $tokenOutput = @()
        $openedDeviceLogin = $false
        & $command.Source token -u --dcf -s $Scopes 2>&1 | Tee-Object -Variable tokenOutput | ForEach-Object {
            $line = "$_"
            Write-Host $line
            if (-not $NoOpen -and -not $openedDeviceLogin) {
                $deviceLoginURL = Parse-DeviceLoginURL -Line $line
                if (-not [string]::IsNullOrWhiteSpace($deviceLoginURL)) {
                    Start-Process $deviceLoginURL
                    $openedDeviceLogin = $true
                }
            }
        }
        $tokens = Parse-TokenOutput -Lines $tokenOutput
        if ([string]::IsNullOrWhiteSpace($tokens.AccessToken)) {
            $failureMessage = Parse-TokenFailureMessage -Lines $tokenOutput
            if (-not [string]::IsNullOrWhiteSpace($failureMessage)) {
                throw $failureMessage
            }
            throw "Could not find a Twitch access token in the CLI output."
        }

        $body = @{
            access_token = $tokens.AccessToken
        }
        if (-not [string]::IsNullOrWhiteSpace($tokens.RefreshToken)) {
            $body.refresh_token = $tokens.RefreshToken
        }

        $uri = Join-Url -Base $ChatHttp -Path '/v1/auth/dev/prepare'
        try {
            $response = Invoke-RestMethod -Method Post -Uri $uri -ContentType 'application/json' -Body ($body | ConvertTo-Json -Compress) -ErrorAction Stop
        } catch {
            throw "Could not prepare local Streamclone login at $uri. Start the stack with make up, keep TWITCH_DEV_TOKEN_IMPORT_ENABLED=true, and try again. $($_.Exception.Message)"
        }

        if ([string]::IsNullOrWhiteSpace($response.claimUrl)) {
            throw "The auth service did not return a local claim URL."
        }

        # Also sync this token to the .env file for the clipper service
        if (Test-Path $EnvFile) {
            $lines = [string[]](Get-Content $EnvFile)
            $lines = Set-EnvValue -Lines $lines -Key 'CLIPPER_TWITCH_USER_ACCESS_TOKEN' -Value $tokens.AccessToken
            Set-Content -Path $EnvFile -Value $lines
            Write-Host "Synced token to $EnvFile as CLIPPER_TWITCH_USER_ACCESS_TOKEN."
        }

        Write-Host "Prepared local Streamclone login for $($response.user.login)."
        Write-Host "Claim URL: $($response.claimUrl)"
        if (-not $NoOpen) {
            Start-Process $response.claimUrl
        }
    }
    'sync-env' {
        if (-not (Test-Path $CliConfig)) {
            throw "Twitch CLI config not found at $CliConfig. Run twitch configure first."
        }
        if (-not (Test-Path $EnvFile)) {
            throw "Env file not found at $EnvFile. Create it from .env.example first."
        }

        $cli = Read-KeyValueFile -Path $CliConfig
        $clientId = $cli['CLIENTID']
        $clientSecret = $cli['CLIENTSECRET']

        if ([string]::IsNullOrWhiteSpace($clientId) -or [string]::IsNullOrWhiteSpace($clientSecret)) {
            throw "Twitch CLI config is missing CLIENTID or CLIENTSECRET. Run twitch configure first."
        }

        $lines = [string[]](Get-Content $EnvFile)
        $lines = Set-EnvValue -Lines $lines -Key 'TWITCH_OAUTH_CLIENT_ID' -Value $clientId
        $lines = Set-EnvValue -Lines $lines -Key 'TWITCH_OAUTH_CLIENT_SECRET' -Value $clientSecret
        Set-Content -Path $EnvFile -Value $lines

        [pscustomobject]@{
            envFile = (Resolve-Path $EnvFile).Path
            clientId = $clientId
            updated = @('TWITCH_OAUTH_CLIENT_ID', 'TWITCH_OAUTH_CLIENT_SECRET')
        } | ConvertTo-Json
    }
}
