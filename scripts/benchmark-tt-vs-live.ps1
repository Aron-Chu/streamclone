# Compare live-collected minute rollups to TwitchTracker parse for an ended stream.
param(
    [Parameter(Mandatory = $true)]
    [string]$Login,
    [Parameter(Mandatory = $true)]
    [string]$StreamId,
    [string]$BaseUrl = "http://localhost:8090",
    [string]$ScraperUrl = "http://127.0.0.1:8000/v2/scrape",
    [string]$ApiKey = "local-dev-key",
    [string]$Output = "",
    [int]$TimeoutMs = 120000
)

$ErrorActionPreference = "Stop"
$repoRoot = Split-Path $PSScriptRoot -Parent
$workDir = Join-Path $repoRoot "docs\benchmarks\tt-bench-work"
New-Item -ItemType Directory -Force -Path $workDir | Out-Null

if (-not $Output) {
    $Output = Join-Path $repoRoot "docs\benchmarks\tt-vs-live-$StreamId.json"
}
$outParent = Split-Path $Output -Parent
if ($outParent) { New-Item -ItemType Directory -Force -Path $outParent | Out-Null }

Write-Host "Fetching live rollups from $BaseUrl/v1/analytics/streams/$StreamId"
$detail = Invoke-RestMethod -Uri "$BaseUrl/v1/analytics/streams/$StreamId" -TimeoutSec 60
if (-not $detail.rollups -or @($detail.rollups).Count -eq 0) {
    Write-Error "No rollups returned for stream $StreamId"
    exit 1
}

$rollupsPath = Join-Path $workDir "rollups-$StreamId.json"
$detail.rollups | ConvertTo-Json -Depth 20 | Set-Content -Path $rollupsPath -Encoding utf8NoBOM

& (Join-Path $PSScriptRoot "capture-tt-fixture.ps1") -Login $Login -StreamId $StreamId -ScraperUrl $ScraperUrl -ApiKey $ApiKey -OutputDir $workDir -TimeoutMs $TimeoutMs
$htmlPath = Join-Path $workDir "$Login-$StreamId.html"
if (-not (Test-Path $htmlPath)) {
    Write-Error "TT capture failed"
    exit 1
}

function Invoke-TTCompareTest {
    param([string]$WorkRoot)
    $env:TT_BENCHMARK = "1"
    $env:TT_BENCHMARK_STREAM_ID = $StreamId
    $env:TT_BENCHMARK_HTML = (Join-Path $WorkRoot "docs\benchmarks\tt-bench-work\$Login-$StreamId.html")
    $env:TT_BENCHMARK_ROLLUPS_JSON = (Join-Path $WorkRoot "docs\benchmarks\tt-bench-work\rollups-$StreamId.json")
    $env:TT_BENCHMARK_OUTPUT = $Output
    Push-Location $WorkRoot
    try {
        go test ./internal/analytics/... -run TestBenchmarkTTVsLiveCompare -count=1 -v
    } finally {
        Pop-Location
        Remove-Item Env:TT_BENCHMARK, Env:TT_BENCHMARK_STREAM_ID, Env:TT_BENCHMARK_HTML, Env:TT_BENCHMARK_ROLLUPS_JSON, Env:TT_BENCHMARK_OUTPUT -ErrorAction SilentlyContinue
    }
}

$go = Get-Command go -ErrorAction SilentlyContinue
if ($go) {
    Invoke-TTCompareTest -WorkRoot $repoRoot
} else {
    Write-Host "Go not on PATH — using docker golang image"
    docker run --rm `
        -v "${repoRoot}:/src" `
        -w /src `
        -e TT_BENCHMARK=1 `
        -e "TT_BENCHMARK_STREAM_ID=$StreamId" `
        -e "TT_BENCHMARK_HTML=/src/docs/benchmarks/tt-bench-work/$Login-$StreamId.html" `
        -e "TT_BENCHMARK_ROLLUPS_JSON=/src/docs/benchmarks/tt-bench-work/rollups-$StreamId.json" `
        -e "TT_BENCHMARK_OUTPUT=/src/docs/benchmarks/tt-vs-live-$StreamId.json" `
        golang:1.25-bookworm `
        go test ./internal/analytics/... -run TestBenchmarkTTVsLiveCompare -count=1 -v
}

if (Test-Path $Output) {
    Write-Host "Report: $Output"
    Get-Content $Output -Raw
} else {
    Write-Warning "Report file not written (check test output)"
}
