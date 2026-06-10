# Wait for streamclone-scraper, probe TwitchTracker, auto-clear Camoufox locks + recreate.
param(
    [switch]$CheckOnly,
    [string]$Url = "https://twitchtracker.com/jynxzi/streams/318832886110",
    [int]$ScrapeTimeoutMs = 120000,
    [int]$MaxFixAttempts = 2
)

$ErrorActionPreference = "Stop"
$repoRoot = Split-Path $PSScriptRoot -Parent
Set-Location $repoRoot

function Fail([string]$Message) {
    Write-Error "scraper-preflight: $Message"
    exit 1
}

function Invoke-ComposeScraper {
    param([string[]]$Args)
    docker compose --env-file .env `
        -f deploy/docker-compose.yml `
        -f deploy/docker-compose.local-tunnel.yml `
        --profile scraper @Args
}

function Wait-ScraperHealth {
    Write-Host "Waiting for scraper /health..."
    for ($i = 1; $i -le 45; $i++) {
        docker exec streamclone-scraper curl -sf http://127.0.0.1:8000/health 2>$null | Out-Null
        if ($LASTEXITCODE -eq 0) {
            Write-Host "  scraper healthy"
            return
        }
        Start-Sleep -Seconds 2
    }
    Fail "scraper health check timed out - is streamclone-scraper running?"
}

function Invoke-ScrapeProbe {
    $script = Join-Path $repoRoot "scripts\scrape-test-inline.py"
    docker cp $script streamclone-scraper:/tmp/scrape-probe.py | Out-Null
    $out = docker exec `
        -e "SCRAPE_URL=$Url" `
        -e "SCRAPE_TIMEOUT_MS=$ScrapeTimeoutMs" `
        -e USE_PROXY=false `
        streamclone-scraper python /tmp/scrape-probe.py 2>&1
    $rc = $LASTEXITCODE
    Write-Host $out
    return @{ Output = ($out -join "`n"); ExitCode = $rc }
}

function Clear-ProfileLocks {
    Write-Host "Clearing Camoufox profile locks in scraper volume..."
    $py = 'from profile_sync import clear_firefox_profile_locks; print("removed:", clear_firefox_profile_locks("/data/camoufox-profile"))'
    docker exec streamclone-scraper python -c $py
}

function Recreate-Scraper {
    Write-Host "Recreating streamclone-scraper..."
    Invoke-ComposeScraper @("up", "-d", "--no-deps", "--force-recreate", "scraper")
    Wait-ScraperHealth
}

function Show-RecoveryHints {
    Write-Host ""
    Write-Host "Scraper preflight failed. Camoufox could not fetch TwitchTracker minute data."
    Write-Host "Try: make scraper-reload | make scraper-warm | make scraper-check"
    Write-Host ""
}

$running = docker ps --filter name=streamclone-scraper --format '{{.Names}}' 2>$null
if (-not $running) {
    Fail "streamclone-scraper is not running - start with: make up-scraper"
}

Wait-ScraperHealth

$attempt = 0
while ($true) {
    Write-Host "Probing TwitchTracker scrape ($Url)..."
    $probe = Invoke-ScrapeProbe
    if ($probe.ExitCode -eq 0) {
        Write-Host "scraper-preflight: Camoufox scrape ok (meta#ecs or chart data present)"
        exit 0
    }

    if ($CheckOnly) {
        Show-RecoveryHints
        Fail "scrape probe failed (--check-only)"
    }

    if ($attempt -ge $MaxFixAttempts) {
        Show-RecoveryHints
        Fail "scrape probe failed after $MaxFixAttempts recovery attempts"
    }

    $text = $probe.Output
    if ($text -match 'browser has been closed|firefox is already running|parentlock|profile.*lock') {
        Write-Host "Detected Camoufox profile/browser issue - auto-recovering (attempt $($attempt + 1)/$MaxFixAttempts)..."
        Clear-ProfileLocks
        Recreate-Scraper
        $attempt++
        continue
    }

    if ($text -match 'cloudflare|just a moment|403') {
        Show-RecoveryHints
        Fail "Cloudflare blocked the scrape - warm the Camoufox profile (see above)"
    }

    Write-Host "Unhandled scrape failure - recreating scraper once..."
    Recreate-Scraper
    $attempt++
}
