# Wait for streamclone-scraper, probe TwitchTracker, auto-clear Camoufox locks + recreate.
param(
    [switch]$CheckOnly,
    [string]$Url = "https://twitchtracker.com/jynxzi/streams/318832886110",
    [string]$SecondUrl = "https://twitchtracker.com/ishowspeed/streams/318098150359",
    [int]$ScrapeTimeoutMs = 120000,
    [int]$MaxFixAttempts = 2
)

$ErrorActionPreference = "Stop"
$repoRoot = Split-Path $PSScriptRoot -Parent
Set-Location $repoRoot
. (Join-Path $PSScriptRoot 'lib\env.ps1')
. (Join-Path $PSScriptRoot 'lib\stack-progress.ps1')

function Fail([string]$Message) {
    [Console]::Error.WriteLine("scraper-preflight: $Message")
    exit 1
}

function Wait-ScraperContainer {
    Write-Host "Waiting for streamclone-scraper container..."
    for ($i = 1; $i -le 450; $i++) {
        $running = docker ps --filter name=streamclone-scraper --format '{{.Names}}' 2>$null
        if ($running) { return }
        Start-Sleep -Seconds 2
    }
    Fail "Analytics container did not start within 15 minutes. Check .streamclone-start-scraper.log.err"
}

function Invoke-ComposeScraper {
    param([string[]]$ComposeArguments)
    $useImages = (Test-Path (Join-Path $repoRoot 'VERSION'))
    $sourceBuild = Test-ScraperBuildFromSource -Root $repoRoot
    $composeArgs = Get-StreamcloneComposeArgs -Root $repoRoot -Profile 'scraper' -UseImages:$useImages -ScraperSourceBuild:$sourceBuild
    $args = $ComposeArguments
    if ($sourceBuild -and ($args -notcontains '--build')) {
        $args = @('--build') + $args
    }
    Invoke-EnvDocker -Arguments ($composeArgs + $args)
    if ($LASTEXITCODE -ne 0) {
        Fail "docker compose failed (exit $LASTEXITCODE)"
    }
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
    param([string]$ProbeUrl)
    $script = Join-Path $repoRoot "scripts\scrape-test-inline.py"
    docker cp $script streamclone-scraper:/tmp/scrape-probe.py | Out-Null
    $out = docker exec `
        -e "SCRAPE_URL=$ProbeUrl" `
        -e "SCRAPE_TIMEOUT_MS=$ScrapeTimeoutMs" `
        -e USE_PROXY=false `
        streamclone-scraper python /tmp/scrape-probe.py 2>&1
    $rc = $LASTEXITCODE
    Write-Host $out
    return @{ Output = ($out -join "`n"); ExitCode = $rc }
}

function Invoke-SequentialScrapeProbe {
    $urls = @($Url)
    if ($SecondUrl -and $SecondUrl -ne $Url) {
        $urls += $SecondUrl
    }

    $combined = @()
    for ($i = 0; $i -lt $urls.Count; $i++) {
        $probeUrl = $urls[$i]
        Write-Host "Probing TwitchTracker scrape $($i + 1)/$($urls.Count) ($probeUrl)..."
        $probe = Invoke-ScrapeProbe -ProbeUrl $probeUrl
        $combined += $probe.Output
        if ($probe.ExitCode -ne 0) {
            return @{ Output = ($combined -join "`n"); ExitCode = $probe.ExitCode }
        }
    }

    return @{ Output = ($combined -join "`n"); ExitCode = 0 }
}

function Clear-ProfileLocks {
    Write-Host "Clearing Camoufox profile locks in scraper volume..."
    docker exec streamclone-scraper python -c "from profile_sync import clear_firefox_profile_locks; print('removed:', clear_firefox_profile_locks('/data/camoufox-profile'))"
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

Wait-ScraperContainer

Wait-ScraperHealth

$attempt = 0
while ($true) {
    $probe = Invoke-SequentialScrapeProbe
    if ($probe.ExitCode -eq 0) {
        Write-Host "scraper-preflight: Camoufox sequential scrape ok (meta#ecs or chart data present)"
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
