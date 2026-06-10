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

if ($envValues['TWITCH_DEV_TOKEN_IMPORT_ENABLED'] -ne 'true') {
    Add-ValidateWarning -Message 'TWITCH_DEV_TOKEN_IMPORT_ENABLED is not true' -Hint 'Run make setup (core profile sets this for in-app local token import)'
}

$oauthId = $envValues['TWITCH_OAUTH_CLIENT_ID']
$oauthSecret = $envValues['TWITCH_OAUTH_CLIENT_SECRET']
if ([string]::IsNullOrWhiteSpace($oauthId) -or [string]::IsNullOrWhiteSpace($oauthSecret)) {
    if ($Profile -in @('scraper', 'full')) {
        Add-ValidateError -Message 'TWITCH_OAUTH_CLIENT_ID/SECRET missing from .env' -Fix 'Run twitch configure then make twitch-sync (analytics emote seeding and Helix need these)'
    } else {
        Add-ValidateWarning -Message 'TWITCH_OAUTH_CLIENT_ID/SECRET missing from .env' -Hint 'Analytics emote charts and Helix will fail until you run: make twitch-sync'
    }
}

if ($Profile -in @('scraper', 'full')) {
    Test-Required -Key 'SCRAPER_API_URL' -Fix 'Profile scraper sets SCRAPER_API_URL=http://scraper:8000/v2/scrape'
    Test-NotPlaceholder -Key 'SCRAPER_API_KEY' -Placeholder 'local-dev-key' -Fix 'Run make setup or scripts/validate-env.ps1 -Fix'
    $sibling = Get-EnvScraperSiblingPath
    if (-not (Test-Path (Join-Path $sibling '.git')) -and -not (Test-Path (Join-Path $sibling 'Dockerfile'))) {
        Add-ValidateError -Message "streamclone-scraper sibling missing at $sibling" -Fix "git clone https://github.com/Aron-Chu/streamclone-scraper.git $sibling"
    }
}

if ($Profile -in @('clipper', 'full')) {
    if ([string]::IsNullOrWhiteSpace($envValues['CLIPPER_WEBHOOK_TOKEN'])) {
        Add-ValidateError -Message 'CLIPPER_WEBHOOK_TOKEN is empty' -Fix 'Run make setup or scripts/validate-env.ps1 -Fix'
    }
    if ([string]::IsNullOrWhiteSpace($envValues['VITE_CLIPPER_TOKEN']) -or $envValues['VITE_CLIPPER_TOKEN'] -ne $envValues['CLIPPER_WEBHOOK_TOKEN']) {
        Add-ValidateWarning -Message 'VITE_CLIPPER_TOKEN should match CLIPPER_WEBHOOK_TOKEN' -Hint 'Run make setup or scripts/validate-env.ps1 -Fix'
    }
    if ([string]::IsNullOrWhiteSpace($envValues['CLIPPER_TWITCH_USER_ACCESS_TOKEN'])) {
        Add-ValidateWarning -Message 'CLIPPER_TWITCH_USER_ACCESS_TOKEN is empty' -Hint 'Run make twitch-local-auth after stack is up'
    }
    if ([string]::IsNullOrWhiteSpace($envValues['CLIPPER_TWITCH_CLIENT_ID']) -and [string]::IsNullOrWhiteSpace($envValues['TWITCH_OAUTH_CLIENT_ID'])) {
        Add-ValidateWarning -Message 'No Twitch OAuth client id for clipper' -Hint 'Run twitch configure then make twitch-sync'
    }
}

Write-Host ''
if ($errors -gt 0) {
    Write-Host "validate-env: FAILED — $errors error(s), $warnings warning(s)"
    exit 1
}
Write-Host "validate-env: OK — $warnings warning(s)"
