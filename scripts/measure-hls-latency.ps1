# Measure separated HLS latency layers for Streamclone live relays.
# Usage:
#   .\scripts\measure-hls-latency.ps1
#   .\scripts\measure-hls-latency.ps1 -Channels mrekk,sodapoppin,thebausffs -LatencyMode fast
#   .\scripts\measure-hls-latency.ps1 -NoReoriginControl
param(
    [string]$BaseUrl = "http://localhost:8090",
    [string[]]$Channels = @('mrekk', 'sodapoppin', 'thebausffs'),
    [string]$LatencyMode = 'fast',
    [string]$Quality = 'best',
    [switch]$NoReoriginControl,
    [string]$OutputDir = '',
    [int]$ManifestPollMs = 150,
    [int]$ManifestTimeoutSec = 25
)

# Allow `-Channels mrekk,sodapoppin` when the shell collapses to one argument.
if ($Channels.Count -eq 1 -and $Channels[0] -match ',') {
    $Channels = $Channels[0].Split(',') | ForEach-Object { $_.Trim() } | Where-Object { $_ }
}

$ErrorActionPreference = 'Continue'
$env:STREAMCLONE_NO_BROWSER = '1'

function Get-PlaylistSummary {
    param([string]$Url)
    try {
        $resp = Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 5
        if ($resp.StatusCode -ne 200) {
            return @{ status = $resp.StatusCode; body = '' }
        }
        return @{ status = 200; body = $resp.Content }
    } catch {
        $code = 0
        if ($_.Exception.Response) { $code = [int]$_.Exception.Response.StatusCode }
        return @{ status = $code; body = ''; error = $_.Exception.Message }
    }
}

function Parse-PlaylistTags {
    param([string]$Body)
    $target = $null
    $partTarget = $null
    $mediaSeq = $null
    $hasParts = $Body -match '#EXT-X-PART'
    foreach ($line in ($Body -split "`n")) {
        $line = $line.Trim()
        if ($line -match '^#EXT-X-TARGETDURATION:(.+)$') { $target = $Matches[1] }
        if ($line -match '^#EXT-X-PART-INF:PART-TARGET=(.+)$') { $partTarget = $Matches[1] }
        if ($line -match '^#EXT-X-MEDIA-SEQUENCE:(.+)$') { $mediaSeq = $Matches[1] }
    }
    return @{
        targetDuration = $target
        partTarget = $partTarget
        mediaSequence = $mediaSeq
        hasParts = $hasParts
    }
}

function Wait-ManifestReady {
    param([string]$Url, [int]$TimeoutSec)
    $deadline = (Get-Date).AddSeconds($TimeoutSec)
    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    while ((Get-Date) -lt $deadline) {
        $probe = Get-PlaylistSummary -Url $Url
        if ($probe.status -eq 200 -and $probe.body.Length -gt 0) {
            $sw.Stop()
            return @{ ready = $true; ms = $sw.ElapsedMilliseconds; probe = $probe }
        }
        Start-Sleep -Milliseconds $ManifestPollMs
    }
    $sw.Stop()
    return @{ ready = $false; ms = $sw.ElapsedMilliseconds; probe = $null }
}

function Invoke-StreamStop {
    param([string]$Channel)
    $body = @{ channel = $Channel } | ConvertTo-Json
    try {
        Invoke-RestMethod -Uri "$BaseUrl/v1/stream/stop" -Method Post -Body $body -ContentType 'application/json' -TimeoutSec 10 | Out-Null
    } catch { }
}

function Measure-RelayPath {
    param([string]$Channel)
    $enc = [uri]::EscapeDataString($Channel)
    $manifestUrl = "$BaseUrl/live/$enc/index.m3u8"
    $result = [ordered]@{
        channel = $Channel
        path = 'streamclone-relay'
        latencyMode = $LatencyMode
        quality = $Quality
        start = $null
        diagnostics = $null
        manifest = $null
        errors = @()
    }

    Invoke-StreamStop -Channel $Channel
    Start-Sleep -Seconds 2

    $startBody = @{
        channel = $Channel
        quality = $Quality
        latency_mode = $LatencyMode
    } | ConvertTo-Json
    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    try {
        $start = Invoke-RestMethod -Uri "$BaseUrl/v1/stream/start" -Method Post -Body $startBody -ContentType 'application/json' -TimeoutSec 90
        $sw.Stop()
        $result.start = @{
            elapsedMs = $sw.ElapsedMilliseconds
            startupMs = $start.startupMs
            workerBackend = $start.workerBackend
            fallbackAttempted = $start.fallbackAttempted
            startupBreakdown = $start.startupBreakdown
            sessionId = $start.session_id
        }
    } catch {
        $sw.Stop()
        $result.errors += "start failed: $($_.Exception.Message)"
        return $result
    }

    $manifestWait = Wait-ManifestReady -Url $manifestUrl -TimeoutSec $ManifestTimeoutSec
    if ($manifestWait.ready) {
        $tags = Parse-PlaylistTags -Body $manifestWait.probe.body
        $segmentSec = 2
        if ($tags.targetDuration) {
            $parsed = [double]::TryParse(($tags.targetDuration -replace 's$',''), [ref]$segmentSec)
            if (-not $parsed) { $segmentSec = 2 }
        }
        $relayEdgeSec = switch ($LatencyMode) {
            'instant' { 1 }
            'fast' { 2 }
            default { 3 }
        }
        $browserTargetSec = switch ($LatencyMode) {
            'instant' { 2 * $segmentSec }
            'fast' { 2 * $segmentSec }
            default { 4 * $segmentSec }
        }
        $result.manifest = @{
            url = $manifestUrl
            readyMs = $manifestWait.ms
            status = 200
            tags = $tags
            transport = if ($tags.hasParts -or $tags.partTarget) { 'll-hls' } else { 'hls-mpegts' }
            estimatedRelayEdgeSec = [math]::Round($relayEdgeSec * $segmentSec, 2)
            estimatedBrowserTargetSec = [math]::Round($browserTargetSec, 2)
        }
    } else {
        $result.errors += "manifest not ready within ${ManifestTimeoutSec}s"
    }

    try {
        $diag = Invoke-RestMethod -Uri "$BaseUrl/v1/stream/diagnostics?channel=$enc" -TimeoutSec 10
        $result.diagnostics = @{
            workerBackend = $diag.workerBackend
            fallbackAttempts = $diag.fallbackAttempts
            lastStartError = $diag.lastStartError
            activeTransport = $diag.activeTransport
            measuredDelaySec = $diag.measuredDelaySec
            liveEdge = $diag.liveEdge
            hlsProbe = $diag.hlsProbe
            startupBreakdown = $diag.startupBreakdown
        }
    } catch {
        $result.errors += "diagnostics failed: $($_.Exception.Message)"
    }

    return $result
}

function Measure-NoReoriginControl {
    param([string]$Channel)
    $result = [ordered]@{
        channel = $Channel
        path = 'streamlink-direct'
        note = 'No ffmpeg/RTMP/MediaMTX re-origin; Streamlink low-latency HLS only'
        available = $false
        streamlinkVersion = $null
        mpvVersion = $null
        errors = @()
    }

    $sl = Get-Command streamlink -ErrorAction SilentlyContinue
    if (-not $sl) {
        $result.errors += 'streamlink not on PATH - install for no-reorigin control'
        return $result
    }
    try {
        $ver = & streamlink --version 2>&1 | Select-Object -First 1
        $result.streamlinkVersion = "$ver".Trim()
        $result.available = $true
    } catch {
        $result.errors += "streamlink --version failed: $($_.Exception.Message)"
    }

    $mpv = Get-Command mpv -ErrorAction SilentlyContinue
    if ($mpv) {
        try {
            $mpvVer = & mpv --version 2>&1 | Select-Object -First 1
            $result.mpvVersion = "$mpvVer".Trim()
            $result.mpvProfile = '--profile=low-latency --cache=no'
        } catch { }
    } else {
        $result.errors += 'mpv not on PATH - use Twitch browser tab or mpv for glass-to-glass source baseline'
    }

    $result.streamlinkCmd = @(
        'streamlink',
        '--twitch-disable-ads',
        '--twitch-low-latency',
        '--hls-live-edge', '2',
        '--stream-url',
        "twitch.tv/$Channel",
        'best'
    ) -join ' '

    return $result
}

$runDate = Get-Date -Format 'yyyy-MM-dd'
$runStamp = Get-Date -Format 'yyyyMMdd-HHmm'
if (-not $OutputDir) {
    $OutputDir = Join-Path (Split-Path $PSScriptRoot -Parent) 'docs\benchmarks'
}
if (-not (Test-Path $OutputDir)) {
    New-Item -ItemType Directory -Path $OutputDir -Force | Out-Null
}

$report = [ordered]@{
    runDate = $runDate
    runStamp = $runStamp
    baseUrl = $BaseUrl
    channels = $Channels
    latencyMode = $LatencyMode
    relay = @()
    noReorigin = @()
}

Write-Host "HLS latency measurement @ $BaseUrl"
Write-Host "Channels: $($Channels -join ', ') | latency=$LatencyMode"
Write-Host ""

foreach ($ch in $Channels) {
    Write-Host "=== $ch (relay) ==="
    $relay = Measure-RelayPath -Channel $ch
    $report.relay += $relay
    if ($relay.start) {
        $bd = $relay.start.startupBreakdown
        Write-Host ("  start {0}ms backend={1} fallback={2}" -f $relay.start.startupMs, $relay.start.workerBackend, $relay.start.fallbackAttempted)
        if ($bd) {
            Write-Host ("    upstream={0}ms spawn={1}ms probeBudget={2}ms hlsReady={3}ms" -f $bd.upstreamFetchMs, $bd.workerSpawnMs, $bd.hlsProbeBudgetMs, $bd.hlsReadyMs)
        }
    }
    if ($relay.manifest) {
        Write-Host ("  manifest {0} transport={1} target~{2}s browser~{3}s" -f $relay.manifest.readyMs, $relay.manifest.transport, $relay.manifest.estimatedRelayEdgeSec, $relay.manifest.estimatedBrowserTargetSec)
    }
    if ($relay.diagnostics) {
        Write-Host ("  diagnostics backend={0} transport={1} measuredDelay={2}s" -f $relay.diagnostics.workerBackend, $relay.diagnostics.activeTransport, $relay.diagnostics.measuredDelaySec)
        if ($relay.diagnostics.lastStartError) {
            Write-Host "  lastStartError: $($relay.diagnostics.lastStartError)"
        }
    }
    foreach ($err in $relay.errors) { Write-Host "  ERR: $err" }
    Write-Host ""
}

if ($NoReoriginControl) {
    foreach ($ch in $Channels) {
        Write-Host "=== $ch (no-reorigin control) ==="
        $ctrl = Measure-NoReoriginControl -Channel $ch
        $report.noReorigin += $ctrl
        if ($ctrl.available) {
            Write-Host "  streamlink: $($ctrl.streamlinkVersion)"
            Write-Host "  cmd: $($ctrl.streamlinkCmd)"
            if ($ctrl.mpvVersion) { Write-Host "  mpv: $($ctrl.mpvVersion)" }
        }
        foreach ($err in $ctrl.errors) { Write-Host "  NOTE: $err" }
        Write-Host ""
    }
}

$jsonPath = Join-Path $OutputDir "hls-latency-$runStamp.json"
$report | ConvertTo-Json -Depth 8 | Set-Content -Path $jsonPath -Encoding UTF8
Write-Host "JSON artifact: $jsonPath"
Write-Host "Append summary to docs/benchmarks/hls-latency-run-$runDate.md"
