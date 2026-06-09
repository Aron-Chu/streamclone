# Diagnose TwitchTracker scraper - direct vs proxy, meta#ecs presence.
# On Windows, localhost:8000 may hit wslrelay (stale scraper). Use -UseDocker to test the live container.
param(
    [string]$ScraperUrl = "http://localhost:8000/v2/scrape",
    [string]$TestUrl = "https://twitchtracker.com/jynxzi/streams/318832886110",
    [switch]$UseDocker
)

function Test-Scrape {
    param([bool]$UseProxy)
    $body = @{
        url = $TestUrl
        formats = @("rawHtml")
        useProxy = $UseProxy
        timeout = 60000
    } | ConvertTo-Json

    Write-Host "`n--- useProxy=$UseProxy ---"
    try {
        $resp = Invoke-RestMethod -Uri $ScraperUrl -Method Post -Body $body -ContentType "application/json" -TimeoutSec 90
        if (-not $resp.success) {
            Write-Host "[FAIL] $($resp.error)"
            return
        }
        $html = $resp.data.rawHtml
        if (-not $html) { $html = $resp.data.html }
        $hasEcs = $html -match 'id="ecs"'
        $isCf = ($html -match 'just a moment') -or ($html -match 'cf_chl_opt') -or ($html -match 'performing security verification')
        Write-Host "success=true  len=$($html.Length)  meta_ecs=$hasEcs  cloudflare=$isCf  usedProxy=$($resp.data.usedProxy)"
        if (-not $hasEcs) {
            Write-Host "Snippet: $($html.Substring(0, [Math]::Min(400, $html.Length)))"
        }
    } catch {
        Write-Host "[ERROR] $_"
    }
}

if ($UseDocker) {
    Write-Host "Scraper diagnostic via Docker (bypasses wslrelay on localhost:8000)"
    $repoRoot = Split-Path $PSScriptRoot -Parent
    $script = Join-Path $repoRoot "scripts\scrape-test-inline.py"
    docker cp $script streamclone-scraper:/tmp/diagnose-scrape.py | Out-Null
    foreach ($proxy in @("false", "true")) {
        Write-Host "`n--- useProxy=$proxy (docker) ---"
        docker exec -e USE_PROXY=$proxy -e SCRAPE_TIMEOUT_MS=60000 streamclone-scraper python /tmp/diagnose-scrape.py
        if ($LASTEXITCODE -ne 0) {
            Write-Host "[result] scrape failed or missing meta#ecs (Cloudflare likely still blocking)"
        }
    }
    exit 0
}

Write-Host "Scraper diagnostic: $TestUrl"
Write-Host "Tip: if results look stale, re-run with -UseDocker"
Test-Scrape -UseProxy $false
Test-Scrape -UseProxy $true
