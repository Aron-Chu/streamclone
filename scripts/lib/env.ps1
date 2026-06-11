# Shared env merge + secret generation for bootstrap/setup/validate (PowerShell).

function Get-EnvRepoRoot {
    return (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
}

function Get-EnvRandomHex {
    param([int]$Bytes = 24)
    $chars = (1..($Bytes * 2) | ForEach-Object { '{0:x}' -f (Get-Random -Maximum 16) }) -join ''
    return $chars
}

function Get-EnvProfileFragment {
    param([ValidateSet('core', 'scraper', 'clipper', 'full')][string]$Profile)
    $root = Get-EnvRepoRoot
    return Join-Path $root "deploy\env\profile-$Profile.env"
}

function Get-EnvComposeProfiles {
    param([ValidateSet('core', 'scraper', 'clipper', 'full')][string]$Profile)
    switch ($Profile) {
        'core' { return @() }
        'scraper' { return @('scraper') }
        'clipper' { return @('clipper') }
        'full' { return @('scraper', 'clipper') }
    }
}

function Read-EnvKeyValueFile {
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

function Merge-EnvFiles {
    param(
        [string]$OutFile,
        [string[]]$Sources
    )
    $merged = [ordered]@{}
    foreach ($src in $Sources) {
        if (-not (Test-Path $src)) { continue }
        foreach ($line in Get-Content $src) {
            if ($line -match '^(?<key>[A-Z0-9_]+)=(?<value>.*)$') {
                $merged[$matches['key']] = $matches['value']
            }
        }
    }
    $lines = foreach ($key in $merged.Keys) { "$key=$($merged[$key])" }
    Set-Content -Path $OutFile -Value $lines
}

function Set-EnvFileValue {
    param(
        [string]$Path,
        [string]$Key,
        [string]$Value
    )
    $prefix = "$Key="
    $lines = @()
    $found = $false
    if (Test-Path $Path) {
        $lines = [string[]](Get-Content $Path)
        for ($i = 0; $i -lt $lines.Length; $i++) {
            if ($lines[$i].StartsWith($prefix)) {
                $lines[$i] = $prefix + $Value
                $found = $true
            }
        }
    }
    if (-not $found) {
        $lines += ($prefix + $Value)
    }
    Set-Content -Path $Path -Value $lines
}

function Test-EnvPlaceholderValue {
    param(
        [string]$Key,
        [string]$Value
    )
    switch ($Key) {
        'CURATOR_API_TOKEN' { return [string]::IsNullOrWhiteSpace($Value) -or $Value -eq 'change-me' }
        'AUTH_COOKIE_SECRET' { return [string]::IsNullOrWhiteSpace($Value) -or $Value -eq 'dev-insecure-cookie-secret' }
        'SCRAPER_API_KEY' { return [string]::IsNullOrWhiteSpace($Value) -or $Value -eq 'local-dev-key' }
        'CLIPPER_WEBHOOK_TOKEN' { return [string]::IsNullOrWhiteSpace($Value) }
        'VITE_CLIPPER_TOKEN' { return [string]::IsNullOrWhiteSpace($Value) }
        default { return $false }
    }
}

function Invoke-EnvGenerateSecrets {
    param([string]$EnvFile)
    $current = Read-EnvKeyValueFile -Path $EnvFile

    if (Test-EnvPlaceholderValue -Key 'CURATOR_API_TOKEN' -Value $current['CURATOR_API_TOKEN']) {
        Set-EnvFileValue -Path $EnvFile -Key 'CURATOR_API_TOKEN' -Value (Get-EnvRandomHex -Bytes 24)
    }
    if (Test-EnvPlaceholderValue -Key 'AUTH_COOKIE_SECRET' -Value $current['AUTH_COOKIE_SECRET']) {
        Set-EnvFileValue -Path $EnvFile -Key 'AUTH_COOKIE_SECRET' -Value (Get-EnvRandomHex -Bytes 32)
    }
    if (Test-EnvPlaceholderValue -Key 'SCRAPER_API_KEY' -Value $current['SCRAPER_API_KEY']) {
        Set-EnvFileValue -Path $EnvFile -Key 'SCRAPER_API_KEY' -Value (Get-EnvRandomHex -Bytes 16)
    }

    $current = Read-EnvKeyValueFile -Path $EnvFile
    if (Test-EnvPlaceholderValue -Key 'CLIPPER_WEBHOOK_TOKEN' -Value $current['CLIPPER_WEBHOOK_TOKEN']) {
        $clipper = Get-EnvRandomHex -Bytes 24
        Set-EnvFileValue -Path $EnvFile -Key 'CLIPPER_WEBHOOK_TOKEN' -Value $clipper
        Set-EnvFileValue -Path $EnvFile -Key 'VITE_CLIPPER_TOKEN' -Value $clipper
    } elseif ([string]::IsNullOrWhiteSpace($current['VITE_CLIPPER_TOKEN']) -and -not [string]::IsNullOrWhiteSpace($current['CLIPPER_WEBHOOK_TOKEN'])) {
        Set-EnvFileValue -Path $EnvFile -Key 'VITE_CLIPPER_TOKEN' -Value $current['CLIPPER_WEBHOOK_TOKEN']
    }
}

function Get-EnvReleaseVersionTag {
    $root = Get-EnvRepoRoot
    $versionFile = Join-Path $root 'VERSION'
    if (-not (Test-Path $versionFile)) { return $null }
    $tag = (Get-Content $versionFile -Raw).Trim()
    if ([string]::IsNullOrWhiteSpace($tag)) { return $null }
    return $tag
}

function Invoke-EnvApplyReleaseImageTag {
    param([string]$EnvFile)
    $current = Read-EnvKeyValueFile -Path $EnvFile
    if (-not [string]::IsNullOrWhiteSpace($current['IMAGE_TAG'])) { return }
    $tag = Get-EnvReleaseVersionTag
    if (-not $tag) { return }
    Set-EnvFileValue -Path $EnvFile -Key 'IMAGE_TAG' -Value $tag
    Set-EnvFileValue -Path $EnvFile -Key 'STREAMCLONE_USE_IMAGES' -Value '1'
}

function Repair-FrontendDockerEntrypointLf {
    $path = Join-Path (Get-EnvRepoRoot) 'frontend\docker-entrypoint.d\40-streamclone-config.sh'
    if (-not (Test-Path $path)) { return }
    $text = [System.IO.File]::ReadAllText($path) -replace "`r`n", "`n" -replace "`r", "`n"
    [System.IO.File]::WriteAllText($path, $text, [System.Text.UTF8Encoding]::new($false))
}

function Invoke-EnvSynthesize {
    param(
        [ValidateSet('core', 'scraper', 'clipper', 'full')][string]$Profile = 'core',
        [string]$OutFile = (Join-Path (Get-EnvRepoRoot) '.env')
    )
    $root = Get-EnvRepoRoot
    $sources = @(
        (Join-Path $root '.env.dev'),
        (Get-EnvProfileFragment -Profile $Profile)
    )
    $releaseBundle = Join-Path $root 'deploy\env\release-bundle.env'
    if (Test-Path $releaseBundle) { $sources += $releaseBundle }
    $local = Join-Path $root '.env.local'
    if (Test-Path $local) { $sources += $local }
    Merge-EnvFiles -OutFile $OutFile -Sources $sources
    Set-EnvFileValue -Path $OutFile -Key 'STREAMCLONE_PROFILE' -Value $Profile
    Invoke-EnvGenerateSecrets -EnvFile $OutFile
    Invoke-EnvApplyReleaseImageTag -EnvFile $OutFile
}

function Get-EnvScraperSiblingPath {
    $root = Get-EnvRepoRoot
    return (Resolve-Path (Join-Path $root '..')).Path + '\streamclone-scraper'
}

function Test-EnvPreflightDocker {
    if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
        throw "Docker is required. Install Docker Desktop and ensure 'docker' is on PATH."
    }
    $null = docker compose version 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw "docker compose is required. Update Docker Desktop."
    }
}

function Get-TwitchCliConfigPath {
    $candidates = @(
        (Join-Path $env:APPDATA 'twitch-cli\.twitch-cli.env'),
        (Join-Path $env:USERPROFILE '.config\twitch-cli\.twitch-cli.env')
    )
    foreach ($path in $candidates) {
        if (Test-Path $path) { return $path }
    }
    return $null
}

function Sync-TwitchCliToEnv {
    param([string]$EnvFile = (Join-Path (Get-EnvRepoRoot) '.env'))
    $cliConfig = Get-TwitchCliConfigPath
    if (-not $cliConfig) {
        throw "Twitch CLI config not found. Run: twitch configure"
    }
    $cli = Read-EnvKeyValueFile -Path $cliConfig
    if ([string]::IsNullOrWhiteSpace($cli['CLIENTID']) -or [string]::IsNullOrWhiteSpace($cli['CLIENTSECRET'])) {
        throw "Twitch CLI config missing CLIENTID or CLIENTSECRET."
    }
    Set-EnvFileValue -Path $EnvFile -Key 'TWITCH_OAUTH_CLIENT_ID' -Value $cli['CLIENTID']
    Set-EnvFileValue -Path $EnvFile -Key 'TWITCH_OAUTH_CLIENT_SECRET' -Value $cli['CLIENTSECRET']
}
