# Benchmark streamclone-scraper via HTTP API (TwitchTracker, concurrency, mixed workloads).
param(
    [string]$ScraperUrl = "http://127.0.0.1:8000/v2/scrape",
    [string]$HealthUrl = "http://127.0.0.1:8000/health",
    [string]$Workload = "twitchtracker",
    [string]$Matrix = "A",
    [string]$Concurrency = "1,2,4",
    [int]$Repeats = 1,
    [int]$TimeoutMs = 90000,
    [int]$MaxAgeMs = 0,
    [string]$Output = "",
    [switch]$UseDocker
)

$repoRoot = Split-Path $PSScriptRoot -Parent
$scraperRoot = Join-Path (Split-Path $repoRoot -Parent) "streamclone-scraper"
if (-not (Test-Path $scraperRoot)) {
    $scraperRoot = Join-Path $repoRoot "..\streamclone-scraper"
}

$benchScript = Join-Path $scraperRoot "benchmark_scrape.py"
if (-not (Test-Path $benchScript)) {
    Write-Error "benchmark_scrape.py not found at $benchScript"
    exit 1
}

if ($UseDocker) {
    Write-Host "Running benchmark inside streamclone-scraper container (avoids wslrelay on localhost:8000)"
    $outName = if ($Output) { Split-Path $Output -Leaf } else { "benchmark-scrape-docker.json" }
    $outDir = if ($Output) { Split-Path $Output -Parent } else { $repoRoot }
    if (-not $outDir) { $outDir = $repoRoot }
    New-Item -ItemType Directory -Force -Path $outDir | Out-Null
    $containerOut = "/tmp/$outName"

    docker cp $benchScript streamclone-scraper:/tmp/benchmark_scrape.py | Out-Null
    docker exec `
        -e SCRAPER_URL=http://127.0.0.1:8000/v2/scrape `
        -e SCRAPER_HEALTH_URL=http://127.0.0.1:8000/health `
        streamclone-scraper `
        python /tmp/benchmark_scrape.py `
            --workload $Workload `
            --matrix $Matrix `
            --concurrency $Concurrency `
            --repeats $Repeats `
            --timeout-ms $TimeoutMs `
            --max-age-ms $MaxAgeMs `
            --output $containerOut

    if ($LASTEXITCODE -ne 0) {
        Write-Host "[WARN] Benchmark completed with failures (Cloudflare or scraper not rebuilt)"
    }
    docker cp "streamclone-scraper:${containerOut}" (Join-Path $outDir $outName) | Out-Null
    Write-Host "Report: $(Join-Path $outDir $outName)"
    exit $LASTEXITCODE
}

$python = Get-Command python -ErrorAction SilentlyContinue
if (-not $python) {
    Write-Error "Python not on PATH. Use -UseDocker or install Python."
    exit 1
}

$argsList = @(
    $benchScript,
    "--scraper-url", $ScraperUrl,
    "--health-url", $HealthUrl,
    "--workload", $Workload,
    "--matrix", $Matrix,
    "--concurrency", $Concurrency,
    "--repeats", $Repeats,
    "--timeout-ms", $TimeoutMs,
    "--max-age-ms", $MaxAgeMs
)
if ($Output) {
    $argsList += @("--output", $Output)
}

& $python.Source @argsList
exit $LASTEXITCODE
