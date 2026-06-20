#Requires -Version 5.1
param(
    [ValidateSet('core', 'scraper', 'clipper', 'full')]
    [string]$Profile = 'core',
    [string]$EnvFile = '',
    [switch]$Fix
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'lib\env.ps1')

if ([string]::IsNullOrWhiteSpace($EnvFile)) {
    $EnvFile = Join-Path (Get-EnvRepoRoot) '.env'
}

$errors = 0
$warnings = 0

function Add-ValidateError {
    param([string]$Message, [string]$Fix)
    Write-Host "ERROR: $Message" -ForegroundColor Red
    Write-Host "  fix: $Fix"
    $script:errors++
}

function Add-ValidateWarning {
    param([string]$Message, [string]$Hint)
    Write-Host "WARN: $Message" -ForegroundColor Yellow
    Write-Host "  hint: $Hint"
    $script:warnings++
}

if (-not (Test-Path $EnvFile)) {
    Add-ValidateError -Message "Missing env file at $EnvFile" -Fix "make setup or scripts/setup.ps1 -Profile $Profile -NonInteractive -NoUp"
    Write-Host ""
    Write-Host "validate-env: $errors error(s), $warnings warning(s)"
    exit 1
}

if ($Fix) {
    Invoke-EnvGenerateSecrets -EnvFile $EnvFile
    Write-Host "Regenerated placeholder secrets in $EnvFile"
}

$envValues = Read-EnvKeyValueFile -Path $EnvFile

Write-Host "validate-env: profile=$Profile file=$EnvFile"

if ($Profile -eq 'clipper') {
    Write-Warning 'Profile clipper is deprecated — compose uses core only; install ReplayForge separately (see docs/agents-streamclone-and-replayforge.md).'
}

function Test-Required {
    param([string]$Key, [string]$Fix)
    if ([string]::IsNullOrWhiteSpace($envValues[$Key])) {
        Add-ValidateError -Message "$Key is empty" -Fix $Fix
    }
}

function Test-NotPlaceholder {
    param([string]$Key, [string]$Placeholder, [string]$Fix)
    $val = $envValues[$Key]
    if ([string]::IsNullOrWhiteSpace($val) -or $val -eq $Placeholder) {
        Add-ValidateError -Message "$Key is missing or still '$Placeholder'" -Fix $Fix
    }
}

Test-Required -Key 'DATABASE_URL' -Fix 'Run make setup to synthesize .env from .env.dev'
Test-Required -Key 'REDIS_URL' -Fix 'Run make setup to synthesize .env from .env.dev'
Test-NotPlaceholder -Key 'CURATOR_API_TOKEN' -Placeholder 'change-me' -Fix 'Run make setup or scripts/validate-env.ps1 -Fix'
Test-Required -Key 'PUBLIC_ORIGIN' -Fix 'Set PUBLIC_ORIGIN=http://localhost:8090 for local dev'
Test-Required -Key 'FRONTEND_ORIGIN' -Fix 'Set FRONTEND_ORIGIN=http://localhost:8090'

$isReleaseInstall = ($envValues['STREAMCLONE_USE_IMAGES'] -eq '1')
$loopbackInstall = Test-EnvLoopbackPublicOrigin -EnvValues $envValues
$nonLoopbackOrigin = (-not $loopbackInstall) -and (
    -not [string]::IsNullOrWhiteSpace($envValues['PUBLIC_ORIGIN']) -or
    -not [string]::IsNullOrWhiteSpace($envValues['FRONTEND_ORIGIN'])
)
if ($envValues['TWITCH_DEV_TOKEN_IMPORT_ENABLED'] -eq 'true' -and $nonLoopbackOrigin) {
    Add-ValidateError -Message 'TWITCH_DEV_TOKEN_IMPORT_ENABLED=true with a non-loopback PUBLIC_ORIGIN/FRONTEND_ORIGIN' -Fix 'Set TWITCH_DEV_TOKEN_IMPORT_ENABLED=false before using a tunnel or public domain'
}
if ($isReleaseInstall) {
    if ($envValues['TWITCH_DEV_TOKEN_IMPORT_ENABLED'] -eq 'true' -and -not $loopbackInstall) {
        Add-ValidateWarning -Message 'TWITCH_DEV_TOKEN_IMPORT_ENABLED=true on a non-loopback release install' -Hint 'Dev-only token import should stay false outside localhost (docs/security.md)'
    } elseif ($envValues['TWITCH_DEV_TOKEN_IMPORT_ENABLED'] -ne 'true' -and $loopbackInstall) {
        Add-ValidateWarning -Message 'TWITCH_DEV_TOKEN_IMPORT_ENABLED is not true on loopback release install' -Hint 'Run scripts/reload-env-if-stale.ps1 or restart Streamclone to enable Sign in (optional)'
    }
} elseif ($envValues['TWITCH_DEV_TOKEN_IMPORT_ENABLED'] -ne 'true') {
    Add-ValidateWarning -Message 'TWITCH_DEV_TOKEN_IMPORT_ENABLED is not true' -Hint 'Run make setup (.env.dev sets this for in-app local token import)'
}

if ($nonLoopbackOrigin) {
    if (-not [string]::IsNullOrWhiteSpace($envValues['SETUP_CONTROL_TOKEN'])) {
        Add-ValidateWarning -Message 'SETUP_CONTROL_TOKEN is exposed to browsers through /config.js on a non-loopback origin' -Hint 'Unset SETUP_CONTROL_TOKEN or restrict tunnel access; setup-control mutations are intended for trusted localhost only'
    }
    if (-not [string]::IsNullOrWhiteSpace($envValues['VITE_CLIPPER_TOKEN'])) {
        Add-ValidateWarning -Message 'VITE_CLIPPER_TOKEN is exposed to browsers through /config.js on a non-loopback origin' -Hint 'Unset VITE_CLIPPER_TOKEN unless every visitor is trusted to call clipper mutation APIs'
    }
    if ($envValues['S3_ACCESS_KEY'] -eq 'minioadmin' -or $envValues['S3_SECRET_KEY'] -eq 'minioadmin') {
        Add-ValidateWarning -Message 'MinIO root credentials are still the local defaults on a non-loopback origin' -Hint 'Rotate S3_ACCESS_KEY/S3_SECRET_KEY before treating this install as production-ready'
    }
}

$oauthId = $envValues['TWITCH_OAUTH_CLIENT_ID']
$oauthSecret = $envValues['TWITCH_OAUTH_CLIENT_SECRET']
if ([string]::IsNullOrWhiteSpace($oauthId) -or [string]::IsNullOrWhiteSpace($oauthSecret)) {
    if ($envValues['TWITCH_DEV_TOKEN_IMPORT_ENABLED'] -eq 'true') {
        Add-ValidateWarning -Message 'Sign in (optional) requires TWITCH_OAUTH_CLIENT_ID/SECRET' -Hint 'Run twitch configure then make twitch-sync, or copy deploy/env/oauth-bundle.env.example to deploy/env/oauth-bundle.env and re-run setup'
    } elseif ($Profile -in @('scraper', 'full')) {
        Add-ValidateWarning -Message 'TWITCH_OAUTH_CLIENT_ID/SECRET missing from .env' -Hint 'Streamclone can still start; Helix VOD lookup/token refresh may be limited until you run twitch configure then make twitch-sync'
    } else {
        Add-ValidateWarning -Message 'TWITCH_OAUTH_CLIENT_ID/SECRET missing from .env' -Hint 'Optional Helix enrichment and token refresh are limited until you run: make twitch-sync'
    }
}

if ($envValues['PULSE_WIRE_ENABLED'] -eq 'true') {
    if ($envValues['REDDIT_COMMERCIAL_OK'] -ne 'true') {
        Add-ValidateWarning -Message 'PULSE_WIRE_ENABLED=true but REDDIT_COMMERCIAL_OK is not true' -Hint 'Reddit ingest stays disabled until you accept commercial API terms (set REDDIT_COMMERCIAL_OK=true in .env.local)'
    }
}

$streamerbansEnabled = ($envValues['STREAMERBANS_INGEST_ENABLED'] -eq 'true')
$xUnofficialOK = ($envValues['X_UNOFFICIAL_OK'] -eq 'true')
$xAuthToken = [string]$envValues['X_AUTH_TOKEN']
$emusksXAuthToken = [string]$envValues['EMUSKS_X_AUTH_TOKEN']
$hasXToken = (-not [string]::IsNullOrWhiteSpace($xAuthToken)) -or (-not [string]::IsNullOrWhiteSpace($emusksXAuthToken))
if ($xUnofficialOK -and -not $hasXToken) {
    Add-ValidateWarning -Message 'X_UNOFFICIAL_OK=true but no X_AUTH_TOKEN or EMUSKS_X_AUTH_TOKEN is set' -Hint 'StreamerBans tier 2 is credential-gated; leave tier 2 unset for HTML fallback, or keep the token in .env.local'
}
if ($hasXToken -and -not $xUnofficialOK) {
    Add-ValidateWarning -Message 'X_AUTH_TOKEN/EMUSKS_X_AUTH_TOKEN is set but X_UNOFFICIAL_OK is not true' -Hint 'Set X_UNOFFICIAL_OK=true only if you accept the unofficial emusks/X path; otherwise remove the token'
}
if (($xUnofficialOK -or $hasXToken) -and -not $streamerbansEnabled) {
    Add-ValidateWarning -Message 'StreamerBans tier-2 variables are set but STREAMERBANS_INGEST_ENABLED is not true' -Hint 'Tier 2 only augments StreamerBans ingest; set STREAMERBANS_INGEST_ENABLED=true or remove the tier-2 variables'
}

if ($Profile -in @('scraper', 'full')) {
    Test-Required -Key 'SCRAPER_API_URL' -Fix 'Profile scraper sets SCRAPER_API_URL=http://scraper:8000/v2/scrape'
    Test-NotPlaceholder -Key 'SCRAPER_API_KEY' -Placeholder 'local-dev-key' -Fix 'Run make setup or scripts/validate-env.ps1 -Fix'
    $scraperUseImages = ($envValues['SCRAPER_USE_IMAGES'] -eq '1')
    if (-not $scraperUseImages) {
        $sibling = Get-EnvScraperSiblingPath
        if (-not (Test-Path (Join-Path $sibling '.git')) -and -not (Test-Path (Join-Path $sibling 'Dockerfile'))) {
            Add-ValidateError -Message "streamclone-scraper sibling missing at $sibling" -Fix "git clone https://github.com/Aron-Chu/streamclone-scraper.git $sibling or set SCRAPER_USE_IMAGES=1"
        }
    }
}

if (-not [string]::IsNullOrWhiteSpace($envValues['CLIPPER_TWITCH_USER_ACCESS_TOKEN'])) {
    $replayForgeOk = $false
    try {
        $resp = Invoke-WebRequest -Uri 'http://127.0.0.1:8095/healthz' -UseBasicParsing -TimeoutSec 3
        if ($resp.StatusCode -ge 200 -and $resp.StatusCode -lt 300) { $replayForgeOk = $true }
    } catch { }
    if (-not $replayForgeOk) {
        Add-ValidateWarning -Message 'CLIPPER_TWITCH_USER_ACCESS_TOKEN is set but ReplayForge is not reachable at http://127.0.0.1:8095/healthz' -Hint 'Install and start ReplayForge separately — see docs/agents-streamclone-and-replayforge.md'
    }
}

Write-Host ''
if ($errors -gt 0) {
    Write-Host "validate-env: FAILED - $errors error(s), $warnings warning(s)"
    exit 1
}
Write-Host "validate-env: OK - $warnings warning(s)"
