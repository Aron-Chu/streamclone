# Benchmark HLS relay cold start and manifest availability.
# Usage: .\scripts\benchmark-hls-start.ps1 -Channel jynxzi -Runs 2
param(
    [string]$BaseUrl = "http://localhost:8090",
    [string]$Channel = "jynxzi",
    [int]$Runs = 2
)

$env:STREAMCLONE_NO_BROWSER = '1'

function Wait-Manifest {
    param([string]$Url, [int]$TimeoutSec = 20)
    $deadline = (Get-Date).AddSeconds($TimeoutSec)
    while ((Get-Date) -lt $deadline) {
        try {
            $resp = Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 5
            if ($resp.StatusCode -eq 200) {
                return $resp.StatusCode
            }
        } catch { }
        Start-Sleep -Milliseconds 150
    }
    return 0
}

Write-Host "HLS benchmark: $Channel @ $BaseUrl ($Runs runs)"
Write-Host ""

$enc = [uri]::EscapeDataString($Channel)
# MediaMTX serves index.m3u8; main_stream.m3u8 is legacy.
$manifestCandidates = @(
    "$BaseUrl/live/$enc/index.m3u8",
    "$BaseUrl/live/$enc/main_stream.m3u8"
)

for ($i = 0; $i -lt $Runs; $i++) {
    Write-Host "--- Run $($i + 1) ---"
    $stopBody = @{ channel = $Channel } | ConvertTo-Json
    try {
        Invoke-RestMethod -Uri "$BaseUrl/v1/stream/stop" -Method Post -Body $stopBody -ContentType "application/json" -TimeoutSec 10 | Out-Null
    } catch { }
    Start-Sleep -Seconds 2

    $startBody = @{ channel = $Channel } | ConvertTo-Json
    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    try {
        $start = Invoke-RestMethod -Uri "$BaseUrl/v1/stream/start" -Method Post -Body $startBody -ContentType "application/json" -TimeoutSec 60
        $sw.Stop()
        $relayMs = $start.startupMs
        $bd = $start.startupBreakdown
        Write-Host ("  POST /stream/start: {0}ms (reported startupMs={1})" -f $sw.ElapsedMilliseconds, $relayMs)
        if ($bd) {
            Write-Host ("    upstream={0}ms spawn={1}ms hlsReady={2}ms" -f $bd.upstreamFetchMs, $bd.workerSpawnMs, $bd.hlsReadyMs)
        }
        if ($start.session_id) { Write-Host "    session=$($start.session_id) backend=$($start.backend)" }
    } catch {
        $sw.Stop()
        Write-Host "  POST /stream/start FAIL: $($_.Exception.Message)"
        continue
    }

    $mSw = [System.Diagnostics.Stopwatch]::StartNew()
    $code = 0
    $manifestUrl = $null
    foreach ($candidate in $manifestCandidates) {
        $code = Wait-Manifest -Url $candidate -TimeoutSec 25
        if ($code -eq 200) {
            $manifestUrl = $candidate
            break
        }
    }
    $mSw.Stop()
    if ($code -eq 200) {
        Write-Host ("  manifest 200 ({0}): {1}ms after start" -f $manifestUrl, $mSw.ElapsedMilliseconds)
    } else {
        Write-Host "  manifest not ready within timeout (tried: $($manifestCandidates -join ', '))"
    }
    Write-Host ""
}

Write-Host "Compare firstFrameMs in Channel stats panel (browser) with relay startupMs above."
Write-Host "Record baselines in memories/repo/playback-notes.md"
