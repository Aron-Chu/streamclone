# Run Reddit + X (emusks) social probes.
param(
    [string]$Login = 'xqc',
    [string]$OutFile = '',
    [switch]$UseProxy,
    [switch]$RequireXIngest,
    [string]$XIngestUrl = 'http://127.0.0.1:8098',
    [int]$ScrapeTimeoutMs = 90000
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
Set-Location $repoRoot

if ([string]::IsNullOrWhiteSpace($OutFile)) {
    $stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
    $dir = Join-Path $repoRoot 'docs\benchmarks'
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
    $OutFile = Join-Path $dir "social-probe-$stamp.json"
}

$scriptPath = Join-Path $PSScriptRoot 'social-probe.py'
$running = docker ps --format '{{.Names}}' 2>$null | Select-String -Pattern '^streamclone-scraper$' -Quiet

if ($running) {
    docker cp $scriptPath streamclone-scraper:/tmp/social-probe.py | Out-Null
    $envArgs = @(
        '-e', "SOCIAL_PROBE_LOGIN=$Login",
        '-e', "SCRAPE_TIMEOUT_MS=$ScrapeTimeoutMs",
        '-e', "X_INGEST_URL=$XIngestUrl",
        '-e', "USE_PROXY=$($UseProxy.IsPresent.ToString().ToLower())",
        '-e', "REQUIRE_X_INGEST=$($RequireXIngest.IsPresent.ToString().ToLower())",
        '-e', 'SCRAPER_URL=http://127.0.0.1:8000/v2/scrape'
    )
    $raw = docker exec @envArgs streamclone-scraper python /tmp/social-probe.py 2>&1
    $exitCode = $LASTEXITCODE
} else {
    $env:SOCIAL_PROBE_LOGIN = $Login
    $env:SCRAPE_TIMEOUT_MS = "$ScrapeTimeoutMs"
    $env:X_INGEST_URL = $XIngestUrl
    $env:USE_PROXY = $UseProxy.IsPresent.ToString().ToLower()
    $env:REQUIRE_X_INGEST = $RequireXIngest.IsPresent.ToString().ToLower()
    $env:SCRAPER_URL = 'http://127.0.0.1:8000/v2/scrape'
    $python = 'python'
    if (Get-Command py -ErrorAction SilentlyContinue) { $python = 'py -3' }
    $raw = Invoke-Expression "$python `"$scriptPath`"" 2>&1
    $exitCode = $LASTEXITCODE
}

$text = ($raw -join "`n").Trim()
$jsonStart = $text.IndexOf('{')
if ($jsonStart -ge 0) {
    $text.Substring($jsonStart) | Set-Content -LiteralPath $OutFile -Encoding UTF8
    Write-Host "Wrote $OutFile"
} else {
    Write-Host $text
    throw 'social-probe did not emit JSON'
}

foreach ($line in ($text -split "`n")) {
    if ($line -match '^\s*"id"') { Write-Host $line }
    if ($line -match '^\s*"success"') { Write-Host $line -ForegroundColor Cyan }
}

if ($exitCode -ne 0) { exit $exitCode }
