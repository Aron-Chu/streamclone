# Benchmark Pydoll Turnstile handling vs Camoufox/Chromium on TwitchTracker.
param(
    [string]$Url = 'https://twitchtracker.com/jynxzi/streams/318832886110',
    [string[]]$Engines = @('camoufox_headless', 'pydoll_headless', 'pydoll_headful'),
    [int]$Repeat = 1,
    [ValidateSet('sequential', 'burst', 'cooldown')]
    [string]$StressMode = 'sequential',
    [string]$OutFile = '',
    [switch]$UseHostCdp,
    [int]$CdpPort = 9222
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$scraperRoot = Join-Path (Split-Path $repoRoot -Parent) 'streamclone-scraper'
if (-not (Test-Path $scraperRoot)) {
    $scraperRoot = Join-Path $repoRoot '..\streamclone-scraper'
}
if (-not (Test-Path $scraperRoot)) {
    throw "streamclone-scraper not found beside Streamclone repo"
}

Set-Location $repoRoot
. (Join-Path $PSScriptRoot 'lib\env.ps1')

function Test-ScraperContainer {
    docker ps --format '{{.Names}}' 2>$null | Select-String -Pattern '^streamclone-scraper$' -Quiet
}

function Invoke-DockerBenchmark {
    param([string[]]$EngineList)
    $engineArg = ($EngineList -join ',')
    $stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
    $containerOut = "/tmp/scraper-turnstile-$stamp.json"
    $envBlock = @(
        "-e", "SCRAPE_URL=$Url",
        "-e", "SCRAPE_TIMEOUT_MS=120000",
        "-e", "BENCH_STRESS_MODE=$StressMode"
    )
    if ($UseHostCdp) {
        $envBlock += @("-e", "PYDOLL_CDP_URL=http://host.docker.internal:$CdpPort")
        $envBlock += @("-e", "CDP_URL=http://host.docker.internal:$CdpPort")
    }
    $cmd = @(
        'python', 'benchmark_browsers.py',
        '--url', $Url,
        '--engines', $engineArg,
        '--repeat', "$Repeat",
        '--stress-mode', $StressMode,
        '--output', $containerOut
    )
    docker exec @envBlock streamclone-scraper @cmd
    $exitCode = $LASTEXITCODE
    docker cp "streamclone-scraper:$containerOut" $OutFile | Out-Null
    if ($exitCode -ne 0) {
        Write-Warning "benchmark_browsers.py exit $exitCode (report still copied when present)"
    }
}

function Invoke-LocalBenchmark {
    param([string[]]$EngineList)
    $python = Join-Path $scraperRoot '.venv-test\Scripts\python.exe'
    if (-not (Test-Path $python)) {
        $python = 'python'
    }
    $engineArg = ($EngineList -join ',')
    Push-Location $scraperRoot
    try {
        $env:SCRAPE_URL = $Url
        $env:SCRAPE_TIMEOUT_MS = '120000'
        $env:BENCH_STRESS_MODE = $StressMode
        if ($UseHostCdp) {
            $env:PYDOLL_CDP_URL = "http://127.0.0.1:$CdpPort"
            $env:CDP_URL = "http://127.0.0.1:$CdpPort"
        }
        & $python benchmark_browsers.py `
            --url $Url `
            --engines $engineArg `
            --repeat $Repeat `
            --stress-mode $StressMode `
            --output $OutFile
        if ($LASTEXITCODE -ne 0) {
            throw "local benchmark_browsers.py failed (exit $LASTEXITCODE)"
        }
    } finally {
        Pop-Location
    }
}

if ([string]::IsNullOrWhiteSpace($OutFile)) {
    $stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
    $dir = Join-Path $repoRoot 'docs\benchmarks'
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
    $OutFile = Join-Path $dir "scraper-turnstile-$stamp.json"
}

Write-Host "Turnstile benchmark -> $OutFile"
Write-Host "URL: $Url"
Write-Host "Engines: $($Engines -join ', ') repeat=$Repeat stress=$StressMode"

if ($UseHostCdp) {
    $listening = Get-NetTCPConnection -LocalPort $CdpPort -State Listen -ErrorAction SilentlyContinue
    if (-not $listening) {
        Write-Host "Starting host Chrome CDP on port $CdpPort..."
        & (Join-Path $PSScriptRoot 'scraper-cdp.ps1') -Port $CdpPort
        Start-Sleep -Seconds 2
    }
}

if (Test-ScraperContainer) {
    Write-Host 'Running inside streamclone-scraper container...'
    Invoke-DockerBenchmark -EngineList $Engines
} else {
    Write-Host 'Scraper container not running; using one-off docker compose run...'
    $stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
    $runName = "turnstile-bench-$stamp"
    $containerOut = "/tmp/scraper-turnstile-$stamp.json"
    $engineArg = ($Engines -join ',')
    Push-Location (Join-Path $repoRoot 'deploy')
    try {
        docker compose --profile scraper run --name $runName `
            -e "SCRAPE_URL=$Url" `
            -e "SCRAPE_TIMEOUT_MS=120000" `
            -e "BENCH_STRESS_MODE=$StressMode" `
            scraper python benchmark_browsers.py `
            --url $Url `
            --engines $engineArg `
            --repeat $Repeat `
            --stress-mode $StressMode `
            --output $containerOut
        docker cp "${runName}:$containerOut" $OutFile | Out-Null
        docker rm -f $runName 2>$null | Out-Null
    } finally {
        Pop-Location
    }
}

Write-Host "Wrote $OutFile"
