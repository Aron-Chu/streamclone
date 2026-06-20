# Benchmark Network Activity Monitor API paths and Prometheus instant queries.
# Usage: .\scripts\benchmark-network-monitor.ps1 -Runs 5 -OutFile runtime\benchmark-network-20260617-1925.json
param(
    [string]$BaseUrl = "http://localhost:8090",
    [string]$MetadataUrl = "http://127.0.0.1:8081",
    [string]$AnalyticsUrl = "http://127.0.0.1:8086",
    [string]$PrometheusUrl = "http://127.0.0.1:9090",
    [string]$SetupControlUrl = "http://127.0.0.1:9191",
    [string]$SetupControlProxyUrl = "http://localhost:8090/v1/setup-control",
    [int]$Runs = 5,
    [string]$OutFile = ""
)

$ErrorActionPreference = 'Continue'
$env:STREAMCLONE_NO_BROWSER = '1'

function Get-Percentile {
    param([int[]]$Sorted, [double]$P)
    if ($Sorted.Count -eq 0) { return $null }
    $idx = [Math]::Min($Sorted.Count - 1, [Math]::Ceiling(($Sorted.Count - 1) * $P))
    return $Sorted[$idx]
}

function Measure-Endpoint {
    param(
        [string]$Label,
        [string]$Url,
        [switch]$Optional,
        [scriptblock]$Inspect
    )
    $times = @()
    $sizes = @()
    $lastBody = $null
    $lastStatus = $null
    for ($i = 0; $i -lt $Runs; $i++) {
        $sw = [System.Diagnostics.Stopwatch]::StartNew()
        try {
            $resp = Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 120
            $sw.Stop()
            $times += $sw.ElapsedMilliseconds
            $body = $resp.Content
            $sizes += [Math]::Round($body.Length / 1024, 1)
            $lastBody = $body
            $lastStatus = $resp.StatusCode
        } catch {
            $sw.Stop()
            if ($Optional) {
                Write-Host "  [$Label] run $($i+1): skipped ($($_.Exception.Message))"
                continue
            }
            Write-Host "  [$Label] run $($i+1): FAIL $($_.Exception.Message) url=$Url"
            return $null
        }
    }
    if ($times.Count -eq 0) { return $null }
    $sorted = $times | Sort-Object
    $p50 = Get-Percentile -Sorted $sorted -P 0.5
    $p95 = Get-Percentile -Sorted $sorted -P 0.95
    $avgKb = [Math]::Round(($sizes | Measure-Object -Average).Average, 1)
    Write-Host ("  {0,-32} p50={1,5}ms p95={2,5}ms avgKB={3,6}" -f $Label, $p50, $p95, $avgKb)

    $extra = @{}
    if ($Inspect -and $lastBody) {
        try {
            $json = $lastBody | ConvertFrom-Json
            $extra = & $Inspect $json
        } catch { }
    }

    return [ordered]@{
        label = $Label
        url = $Url
        runs = $times.Count
        p50Ms = $p50
        p95Ms = $p95
        avgKb = $avgKb
        status = $lastStatus
        extra = $extra
    }
}

$promQueries = @(
    @{ key = 'httpRequestsPerSec'; query = 'sum(rate(http_requests_total[1m])) by (service)' },
    @{ key = 'chatConnections'; query = 'sum(chat_connections)' },
    @{ key = 'streamListeners'; query = 'sum(stream_listeners)' },
    @{ key = 'chatMessagesOutPerSec'; query = 'sum(rate(chat_messages_out_total[1m]))' },
    @{ key = 'upstreamP95Sec'; query = 'histogram_quantile(0.95, sum(rate(upstream_request_duration_seconds_bucket[5m])) by (le))' },
    @{ key = 'streamListenersByChannel'; query = 'sum(stream_listeners) by (channel)' },
    @{ key = 'analyticsBytesByChannelOp'; query = 'sum(rate(analytics_sync_bytes_total[1m])) by (channel, op)' },
    @{ key = 'analyticsSyncActive'; query = 'sum(analytics_sync_active) by (channel, phase)' }
)

function Measure-PromQuery {
    param([string]$Key, [string]$Query)
    $enc = [uri]::EscapeDataString($Query)
    $url = "$PrometheusUrl/api/v1/query?query=$enc"
    $result = Measure-Endpoint -Label "prom:$Key" -Url $url -Optional
    if ($result) {
        $result.key = $Key
        $result.query = $Query
    }
    return $result
}

Write-Host "Network monitor benchmark ($Runs runs)"
Write-Host "  proxy=$BaseUrl metadata=$MetadataUrl analytics=$AnalyticsUrl prom=$PrometheusUrl"
Write-Host ""

$results = [ordered]@{
    generatedAt = (Get-Date).ToUniversalTime().ToString('o')
    runs = $Runs
    endpoints = @()
    promQueries = @()
    fieldChecks = @{}
    environment = @{}
}

# Baseline snapshot for field checks (single fetch)
try {
    $baseline = Invoke-RestMethod -Uri "$BaseUrl/v1/ops/network" -TimeoutSec 120
    $results.fieldChecks = [ordered]@{
        pulseReady = [bool]$baseline.pulseReady
        trackedCount = @($baseline.trackingSnapshot.tracked).Count
        hasTracked = (@($baseline.trackingSnapshot.tracked).Count -gt 0)
        activeStreamCount = @($baseline.activeStreams).Count
        hasActiveStreams = (@($baseline.activeStreams).Count -gt 0)
        syncJobCount = @($baseline.activeAnalyticsSyncs.jobs).Count
        hasPromAnalyticsBytes = $null -ne $baseline.prometheus.analyticsBytesByChannelOp
        relayChannels = @($baseline.activeStreams | ForEach-Object { $_.channel })
        syncChannels = @($baseline.activeAnalyticsSyncs.jobs | ForEach-Object { $_.channel })
    }
    Write-Host "Field checks: pulseReady=$($results.fieldChecks.pulseReady) tracked=$($results.fieldChecks.trackedCount) relays=$($results.fieldChecks.activeStreamCount) syncs=$($results.fieldChecks.syncJobCount)"
    Write-Host ""
} catch {
    Write-Host "WARN: baseline ops/network fetch failed: $($_.Exception.Message)"
}

$opsInspect = {
    param($json)
    return [ordered]@{
        pulseReady = [bool]$json.pulseReady
        tracked = @($json.trackingSnapshot.tracked).Count
        activeStreams = @($json.activeStreams).Count
        syncJobs = @($json.activeAnalyticsSyncs.jobs).Count
        hasPromAnalyticsBytes = $null -ne $json.prometheus.analyticsBytesByChannelOp
    }
}

foreach ($item in @(
    @{ label = 'ops/network (proxy)'; url = "$BaseUrl/v1/ops/network"; inspect = $opsInspect },
    @{ label = 'ops/network (metadata)'; url = "$MetadataUrl/v1/ops/network"; inspect = $opsInspect },
    @{ label = 'sync/active'; url = "$AnalyticsUrl/v1/analytics/sync/active"; inspect = $null },
    @{ label = 'tracking/snapshot'; url = "$AnalyticsUrl/v1/analytics/tracking/snapshot"; inspect = $null },
    @{ label = 'host diagnostics/network'; url = "$SetupControlUrl/diagnostics/network"; inspect = $null },
    @{ label = 'host diagnostics/network (proxy)'; url = "$SetupControlProxyUrl/diagnostics/network"; inspect = $null }
)) {
    $params = @{
        Label = $item.label
        Url = $item.url
    }
    if ($item.label -eq 'host diagnostics/network (proxy)') {
        $params.Optional = $true
    }
    if ($item.inspect) {
        $params.Inspect = $item.inspect
    }
    $row = Measure-Endpoint @params
    if ($row) { $results.endpoints += $row }
}

Write-Host ""
Write-Host "Prometheus instant queries:"
foreach ($pq in $promQueries) {
    $row = Measure-PromQuery -Key $pq.key -Query $pq.query
    if ($row) { $results.promQueries += $row }
}

# Environment metadata
try {
    $gitSha = (git -C (Split-Path $PSScriptRoot -Parent) rev-parse --short HEAD 2>$null)
    if (-not $gitSha) { $gitSha = 'unknown' }
} catch { $gitSha = 'unknown' }

$results.environment = [ordered]@{
    gitSha = $gitSha
    baseUrl = $BaseUrl
    os = [Environment]::OSVersion.VersionString
    timestampLocal = (Get-Date).ToString('yyyy-MM-dd HH:mm:ss')
}

if (-not $OutFile) {
    $stamp = Get-Date -Format 'yyyyMMdd-HHmm'
    $runtimeDir = Join-Path (Split-Path $PSScriptRoot -Parent) 'runtime'
    if (-not (Test-Path $runtimeDir)) { New-Item -ItemType Directory -Path $runtimeDir | Out-Null }
    $OutFile = Join-Path $runtimeDir "benchmark-network-$stamp.json"
}

$jsonOut = $results | ConvertTo-Json -Depth 8
Set-Content -Path $OutFile -Value $jsonOut -Encoding UTF8
Write-Host ""
Write-Host "Saved: $OutFile"
