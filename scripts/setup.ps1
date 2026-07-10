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
. (Join-Path $PSScriptRoot 'lib\install-upgrade.ps1')

Set-Location (Get-EnvRepoRoot)

if ($env:SETUP_MODE -eq 'release') {
    $UseImages = $true
}

Write-Host 'Streamclone setup'
Write-Host '-----------------'

if (-not $NonInteractive) {
    Write-Host @'

[1] Core only          - watch + chat + emotes (no Twitch login required)
[2] + Analytics charts - uses scraper image or streamclone-scraper sibling repo
[3] Full stack         - core + Analytics scraper

'@
    $choice = Read-Host 'Choose profile [1-3, default 1]'
    switch ($choice) {
        '2' { $Profile = 'scraper' }
        '3' { $Profile = 'full' }
        default { $Profile = 'core' }
    }
}

Write-Host "Profile: $Profile"

if ($Profile -in @('clipper', 'full')) {
}

if (-not $SkipPreflight) {
    try {
        Test-EnvPreflightDocker
        Write-Host 'Docker: ok'
    } catch {
        Write-Host "Docker check failed: $($_.Exception.Message)" -ForegroundColor Red
        Write-Host 'Start Docker Desktop, then run Check Streamclone.cmd in your install folder.' -ForegroundColor Yellow
        exit 1
    }
} else {
    Write-Host 'Docker: preflight skipped'
}

$envFile = Join-Path (Get-Location) '.env'
Invoke-EnvSynthesize -Profile $Profile -OutFile $envFile
Set-Content -Path (Join-Path (Get-Location) '.streamclone-profile') -Value $Profile -NoNewline
$varCount = (Get-Content $envFile | Where-Object { $_ -match '^[A-Z]' }).Count
Write-Host ".env: synthesized ($varCount keys, secrets generated)"

$oauthAfterSynth = Read-EnvKeyValueFile -Path $envFile
if ([string]::IsNullOrWhiteSpace($oauthAfterSynth['TWITCH_OAUTH_CLIENT_ID']) -or
    [string]::IsNullOrWhiteSpace($oauthAfterSynth['TWITCH_OAUTH_CLIENT_SECRET'])) {
    Write-Host 'Note: Twitch OAuth app creds not set — optional Sign in and Helix features need them; Reddit-only Pulse Wire still works.' -ForegroundColor Cyan
}

& powershell -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot 'ensure-oauth-env.ps1') -EnvFile $envFile 2>$null | Out-Host

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
                try {
                    Sync-TwitchCliToEnv -EnvFile $envFile
                    Write-Host '  synced TWITCH_OAUTH_CLIENT_ID/SECRET'
                } catch {
                    Write-Host "  OAuth sync skipped: $($_.Exception.Message)" -ForegroundColor Yellow
                }
            }
        } else {
            Write-Host '  twitch configure not run yet - skip or run: twitch configure'
        }
    } else {
        Write-Host 'Twitch CLI: not found (optional for Clip Studio)'
        Write-Host '  Sign in (optional) still needs TWITCH_OAUTH_CLIENT_ID/SECRET in .env.'
        Write-Host '  Copy deploy/env/oauth-bundle.env.example to oauth-bundle.env, or install twitch-cli and run twitch configure.'
        Write-Host '  Developers: https://github.com/twitchdev/twitch-cli#installation' -ForegroundColor DarkGray
    }
}

$envAfterOAuth = Read-EnvKeyValueFile -Path $envFile
if ($envAfterOAuth['TWITCH_DEV_TOKEN_IMPORT_ENABLED'] -eq 'true' -and
    ([string]::IsNullOrWhiteSpace($envAfterOAuth['TWITCH_OAUTH_CLIENT_ID']) -or
     [string]::IsNullOrWhiteSpace($envAfterOAuth['TWITCH_OAUTH_CLIENT_SECRET']))) {
    Write-Host 'Sign in (optional) will fail until TWITCH_OAUTH_CLIENT_ID/SECRET are set (twitch configure + make twitch-sync, or oauth-bundle.env).' -ForegroundColor Yellow
}

$needsScraper = $Profile -in @('scraper', 'full')
$envAfterSynth = Read-EnvKeyValueFile -Path $envFile
$scraperUseImages = ($envAfterSynth['SCRAPER_USE_IMAGES'] -eq '1')
if ($needsScraper -and $scraperUseImages) {
    Write-Host 'Scraper: GHCR image (SCRAPER_USE_IMAGES=1)'
} elseif ($needsScraper -and -not $SkipScraperClone) {
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

$composeArgs = Get-StreamcloneComposeArgs -Root (Get-Location) -Profile $Profile -UseImages:$UseImages -RelativePaths

if (-not $NoUp) {
    Repair-FrontendDockerEntrypointLf
    if ($UseImages) {
        $pullResult = Invoke-EnvDockerComposePullWithRetry -ComposeArgs $composeArgs -OutputMode friendly
        $pullTail = @($pullResult.Output | Select-Object -Last 40)
        $code = [int]$pullResult.ExitCode
        if ($code -ne 0) {
            $logsDir = Join-Path (Get-Location) 'logs'
            New-Item -ItemType Directory -Force -Path $logsDir | Out-Null
            $pullLog = Join-Path $logsDir 'setup-pull.log'
            @($pullResult.Output) | Set-Content -LiteralPath $pullLog -Encoding UTF8
            Write-Host "docker compose pull failed after retries (exit $code). Check Docker Desktop and network." -ForegroundColor Red
            Write-Host 'Last docker compose lines:' -ForegroundColor Yellow
            $pullTail | Select-Object -Last 15 | ForEach-Object { Write-Host "  $_" -ForegroundColor DarkRed }
            Write-Host "Full pull log: $pullLog" -ForegroundColor Yellow
            $coreStatus = Get-StreamcloneCoreImageStatus -Root (Get-Location)
            Write-Host "$($coreStatus.present)/$($coreStatus.total) core images present - run Start Streamclone to resume." -ForegroundColor Yellow
            Write-Host 'If images partially downloaded, try Start Streamclone.cmd or re-run Install.' -ForegroundColor Yellow
            exit $code
        }
        $upArgs = @('up', '-d', '--remove-orphans', '--pull', 'missing')
    } else {
        $upArgs = @('up', '-d', '--remove-orphans', '--build')
    }
    Write-Host "Starting stack (profile: $Profile)..."
    $code = Invoke-EnvDocker -Arguments ($composeArgs + $upArgs)
    if ($code -ne 0) {
        Write-Host "docker compose up failed (exit $code)." -ForegroundColor Red
        exit $code
    }
    & powershell -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot 'reload-env-if-stale.ps1') -EnvFile $envFile 2>$null
    Write-Host ''
    Write-Host 'Streamclone: http://localhost:8090/'
    & powershell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot 'ensure-setup-control.ps1') -Root (Get-Location)
    $setupControlExitCode = if ($null -ne $LASTEXITCODE) { [int]$LASTEXITCODE } else { 0 }
    if ($setupControlExitCode -ne 0) {
        Write-Host 'Setup failed: setup helper could not start.' -ForegroundColor Red
        exit $setupControlExitCode
    }
    & powershell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot 'ensure-localhost-relays.ps1') -Ports 8090
}

if (-not $NoSmoke -and -not $NoUp) {
    Write-Host 'Waiting for tiered readiness...'
    & powershell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot 'lib\wait-stack.ps1') -Root (Get-Location) -SkipHLS
    $waitExitCode = if ($null -ne $LASTEXITCODE) { [int]$LASTEXITCODE } else { 0 }
    if ($waitExitCode -ne 0) {
        Write-Host 'Setup failed: required readiness tier did not pass.' -ForegroundColor Red
        exit $waitExitCode
    }
    Write-Host 'Running smoke checks (readiness already verified)...'
    & powershell -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot 'smoke-core.ps1') -SkipReadiness
    if ($LASTEXITCODE -ne 0) {
        Write-Host 'Setup failed: smoke checks did not pass.' -ForegroundColor Red
        exit $LASTEXITCODE
    }
    & powershell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot 'ensure-localhost-relays.ps1') -Ports 8090
    & powershell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot 'ensure-setup-control.ps1') -Root (Get-Location) -RequireProxy
    $setupControlProxyExitCode = if ($null -ne $LASTEXITCODE) { [int]$LASTEXITCODE } else { 0 }
    if ($setupControlProxyExitCode -ne 0) {
        Write-Host 'Setup failed: setup helper is not reachable through the app proxy.' -ForegroundColor Red
        exit $setupControlProxyExitCode
    }

    if ($needsScraper -and ($scraperUseImages -or (Test-Path (Get-EnvScraperSiblingPath)))) {
        $preflight = Join-Path $PSScriptRoot 'scraper-preflight.ps1'
        if (Test-Path $preflight) {
            Write-Host 'Running scraper preflight (sequential TwitchTracker probes)...'
            & powershell -NoProfile -ExecutionPolicy Bypass -File $preflight -CheckOnly
            if ($LASTEXITCODE -ne 0) {
                Write-Host 'Setup failed: scraper could not fetch TwitchTracker charts reliably.' -ForegroundColor Red
                Write-Host 'Run scripts\scraper-preflight.ps1 or scripts\warm-camoufox-profile.ps1, then retry setup.' -ForegroundColor Yellow
                exit $LASTEXITCODE
            }
        } else {
            Write-Warning 'scraper-preflight.ps1 missing; falling back to scraper health check only.'
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
    }
}

Write-Host 'Setup complete.'
