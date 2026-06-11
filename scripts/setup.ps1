#Requires -Version 5.1
param(
    [ValidateSet('core', 'scraper', 'clipper', 'full')]
    [string]$Profile = 'core',
    [switch]$NonInteractive,
    [switch]$UseImages,
    [switch]$NoUp,
    [switch]$NoSmoke,
    [switch]$SkipPreflight,
    [switch]$SkipTwitch,
    [switch]$SkipScraperClone
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'lib\env.ps1')

Set-Location (Get-EnvRepoRoot)

if ($env:SETUP_MODE -eq 'release') {
    $UseImages = $true
}

Write-Host 'Streamclone setup'
Write-Host '─────────────────'

if (-not $NonInteractive) {
    Write-Host @'

[1] Core only          — watch + chat + emotes (no Twitch login required)
[2] + Analytics charts — needs streamclone-scraper sibling repo
[3] + Clip Studio      — needs Twitch device-code login
[4] Full stack         — scraper + clipper

'@
    $choice = Read-Host 'Choose profile [1-4, default 1]'
    switch ($choice) {
        '2' { $Profile = 'scraper' }
        '3' { $Profile = 'clipper' }
        '4' { $Profile = 'full' }
        default { $Profile = 'core' }
    }
}

Write-Host "Profile: $Profile"

    if (-not $SkipPreflight) {
        Test-EnvPreflightDocker
        Write-Host 'Docker: ok'
    } else {
        Write-Host 'Docker: preflight already checked'
    }

$envFile = Join-Path (Get-Location) '.env'
Invoke-EnvSynthesize -Profile $Profile -OutFile $envFile
Set-Content -Path (Join-Path (Get-Location) '.streamclone-profile') -Value $Profile -NoNewline
$varCount = (Get-Content $envFile | Where-Object { $_ -match '^[A-Z]' }).Count
Write-Host ".env: synthesized ($varCount keys, secrets generated)"

if (-not $SkipTwitch) {
    $twitch = Get-Command twitch -ErrorAction SilentlyContinue
    if ($twitch) {
        Write-Host 'Twitch CLI: found'
        $cliConfig = Get-TwitchCliConfigPath
        if ($cliConfig) {
            $sync = $NonInteractive
            if (-not $sync) {
                $ans = Read-Host 'Sync OAuth app creds from twitch CLI to .env? [Y/n]'
                $sync = ($ans -eq '' -or $ans -match '^[yY]')
            }
            if ($sync) {
                Sync-TwitchCliToEnv -EnvFile $envFile
                Write-Host '  synced TWITCH_OAUTH_CLIENT_ID/SECRET'
            }
        } else {
            Write-Host '  twitch configure not run yet — skip or run: twitch configure'
            if (-not $NonInteractive -and ($Profile -eq 'clipper' -or $Profile -eq 'full')) {
                $cfg = Read-Host 'Run twitch configure now? [y/N]'
                if ($cfg -match '^[yY]') { & twitch configure }
            }
        }
    } else {
        Write-Host 'Twitch CLI: not found — https://github.com/twitchdev/twitch-cli'
        Write-Host '  Clip Studio / chat OAuth need it; core viewing works without login.'
    }
}

$needsScraper = $Profile -in @('scraper', 'full')
if ($needsScraper -and -not $SkipScraperClone) {
    $sibling = Get-EnvScraperSiblingPath
    if ((Test-Path (Join-Path $sibling '.git')) -or (Test-Path (Join-Path $sibling 'Dockerfile'))) {
        Write-Host "Scraper repo: ok ($sibling)"
    } else {
        Write-Host "Scraper repo: missing at $sibling"
        $clone = $false
        if (-not $NonInteractive) {
            $ans = Read-Host 'Clone https://github.com/Aron-Chu/streamclone-scraper? [Y/n]'
            $clone = ($ans -eq '' -or $ans -match '^[yY]')
        }
        if ($clone) {
            git clone https://github.com/Aron-Chu/streamclone-scraper.git $sibling
            Write-Host "  cloned to $sibling"
        } else {
            Write-Host '  scraper profile disabled until sibling repo exists.'
        }
    }
}

$composeArgs = @(
    'compose', '--env-file', '.env',
    '-f', 'deploy/docker-compose.yml',
    '-f', 'deploy/docker-compose.local-tunnel.yml'
)
if ($UseImages) {
    $composeArgs += '-f', 'deploy/docker-compose.release.yml'
}
foreach ($p in (Get-EnvComposeProfiles -Profile $Profile)) {
    $composeArgs += '--profile', $p
}

if (-not $NoUp) {
    Repair-FrontendDockerEntrypointLf
    if ($UseImages) {
        Write-Host 'Pulling Docker images...'
        $code = Invoke-EnvDocker -Arguments ($composeArgs + @('pull'))
        if ($code -ne 0) { exit $code }
        $upArgs = @('up', '-d', '--remove-orphans', '--pull', 'missing')
    } else {
        $upArgs = @('up', '-d', '--remove-orphans', '--build')
    }
    Write-Host "Starting stack (profile: $Profile)..."
    $code = Invoke-EnvDocker -Arguments ($composeArgs + $upArgs)
    if ($code -ne 0) { exit $code }
    & powershell -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot 'reload-env-if-stale.ps1') -EnvFile $envFile 2>$null
    if ($Profile -in @('clipper', 'full')) {
        & powershell -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot 'ensure-clipper-auth.ps1') -EnvFile $envFile 2>$null
    }
    Write-Host ''
    Write-Host 'Streamclone: http://localhost:8090/'
}

if (-not $NoSmoke -and -not $NoUp) {
    Write-Host 'Running smoke checks...'
    & powershell -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot 'smoke-core.ps1')

    if ($needsScraper -and (Test-Path (Get-EnvScraperSiblingPath))) {
        Write-Host 'Checking scraper health...'
        $ok = $false
        for ($i = 1; $i -le 30; $i++) {
            try {
                Invoke-WebRequest -Uri 'http://localhost:8000/health' -UseBasicParsing -TimeoutSec 3 | Out-Null
                Write-Host '  scraper ok'
                $ok = $true
                break
            } catch { Start-Sleep -Seconds 2 }
        }
        if (-not $ok) { Write-Warning '  scraper health check timed out' }
    }

    if ($Profile -in @('clipper', 'full')) {
        Write-Host 'Checking clipper via proxy...'
        $ok = $false
        for ($i = 1; $i -le 30; $i++) {
            foreach ($url in @('http://localhost:8090/clipper/health', 'http://localhost:8095/health')) {
                try {
                    Invoke-WebRequest -Uri $url -UseBasicParsing -TimeoutSec 3 | Out-Null
                    Write-Host "  clipper ok ($url)"
                    $ok = $true
                    break
                } catch { }
            }
            if ($ok) { break }
            Start-Sleep -Seconds 2
        }
        if (-not $ok) { Write-Warning '  clipper health check timed out' }
    }
}

if ($Profile -in @('clipper', 'full')) {
    Write-Host ''
    Write-Host "Optional: run 'make twitch-local-auth' for clip creation scopes."
}

Write-Host 'Setup complete.'
