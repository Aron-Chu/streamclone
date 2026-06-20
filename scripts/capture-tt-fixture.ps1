# Capture a TwitchTracker stream detail HTML fixture for parser tests.
param(
    [Parameter(Mandatory = $true)]
    [string]$Login,
    [Parameter(Mandatory = $true)]
    [string]$StreamId,
    [string]$ScraperUrl = "http://127.0.0.1:8000/v2/scrape",
    [string]$ApiKey = "local-dev-key",
    [string]$OutputDir = "",
    [int]$TimeoutMs = 120000
)

$repoRoot = Split-Path $PSScriptRoot -Parent
if (-not $OutputDir) {
    $OutputDir = Join-Path $repoRoot "docs\benchmarks\tt-fixtures"
}
New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null

$pageUrl = "https://twitchtracker.com/$Login/streams/$StreamId"
$outName = "$Login-$StreamId.html"
$outPath = Join-Path $OutputDir $outName

Write-Host "Capturing TwitchTracker fixture: $pageUrl"
Write-Host "Output: $outPath"

$body = @{
    url = $pageUrl
    apiKey = $ApiKey
    useProxy = $false
    maxAgeMs = 0
    timeoutMs = $TimeoutMs
} | ConvertTo-Json

try {
    $resp = Invoke-RestMethod -Method Post -Uri $ScraperUrl -Body $body -ContentType "application/json" -TimeoutSec ([math]::Ceiling($TimeoutMs / 1000) + 30)
} catch {
    Write-Error "Scraper request failed: $_"
    Write-Host "Ensure scraper is up: make scraper-preflight"
    exit 1
}

$html = $null
if ($resp -is [string]) {
    $html = $resp
} elseif ($resp.html) {
    $html = $resp.html
} elseif ($resp.body) {
    $html = $resp.body
} elseif ($resp.content) {
    $html = $resp.content
}

if (-not $html -or $html.Length -lt 500) {
    Write-Error "Scraper returned empty or short HTML"
    exit 1
}

if ($html -notmatch 'id="ecs"') {
    Write-Warning "HTML may be incomplete (meta#ecs not found)"
}

Set-Content -Path $outPath -Value $html -Encoding utf8NoBOM
Write-Host "Saved $($html.Length) bytes to $outPath"
Write-Host "Run parser tests: go test ./internal/analytics/... -run TestParseTwitchTrackerFixtures -count=1"
