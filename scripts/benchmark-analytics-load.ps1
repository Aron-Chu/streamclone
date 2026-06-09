# Benchmark Analytics API load times (insights, streams list, stream detail, games).
# Usage: .\scripts\benchmark-analytics-load.ps1 -Login jynxzi -StreamId 318832886110 -Runs 3
param(
    [string]$BaseUrl = "http://localhost:8090",
    [string]$Login = "jynxzi",
    [string]$StreamId = "",
    [int]$Runs = 3
)

function Measure-Endpoint {
    param(
        [string]$Label,
        [string]$Url,
        [switch]$Optional
    )
    $times = @()
    $sizes = @()
    $rollupCount = $null
    for ($i = 0; $i -lt $Runs; $i++) {
        $sw = [System.Diagnostics.Stopwatch]::StartNew()
        try {
            $resp = Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 120
            $sw.Stop()
            $times += $sw.ElapsedMilliseconds
            $body = $resp.Content
            $sizes += [Math]::Round($body.Length / 1024, 1)
            if ($Label -like "detail*" -and $body -match '"rollups"') {
                try {
                    $json = $body | ConvertFrom-Json
                    $rollupCount = @($json.rollups).Count
                } catch { }
            }
        } catch {
            $sw.Stop()
            if ($Optional) {
                Write-Host "  [$Label] run $($i+1): skipped ($($_.Exception.Message))"
                continue
            }
            Write-Host "  [$Label] run $($i+1): FAIL $($_.Exception.Message) url=$Url"
            return
        }
    }
    if ($times.Count -eq 0) { return }
    $sorted = $times | Sort-Object
    $p50 = $sorted[[Math]::Floor(($sorted.Count - 1) * 0.5)]
    $p95 = $sorted[[Math]::Min($sorted.Count - 1, [Math]::Ceiling(($sorted.Count - 1) * 0.95))]
    $avgKb = [Math]::Round(($sizes | Measure-Object -Average).Average, 1)
    $extra = ""
    if ($null -ne $rollupCount) { $extra = " rollups=$rollupCount" }
    Write-Host ("  {0,-22} p50={1,5}ms p95={2,5}ms avgKB={3,6}{4}" -f $Label, $p50, $p95, $avgKb, $extra)
}

Write-Host "Analytics benchmark: $Login @ $BaseUrl ($Runs runs)"
Write-Host ""

$encLogin = [uri]::EscapeDataString($Login)
Measure-Endpoint -Label "insights?period=all" -Url "$BaseUrl/v1/channels/$encLogin/insights?period=all"
Measure-Endpoint -Label "history?period=all" -Url "$BaseUrl/v1/channels/$encLogin/streams/history?period=all" -Optional
Measure-Endpoint -Label "streams?limit=20" -Url "$BaseUrl/v1/analytics/channels/$encLogin/streams?limit=20"

if (-not $StreamId) {
    try {
        $list = Invoke-RestMethod -Uri "$BaseUrl/v1/analytics/channels/$encLogin/streams?limit=5" -TimeoutSec 30
        if ($list.items -and $list.items.Count -gt 0) {
            $StreamId = $list.items[0].streamId
            Write-Host "Auto-selected streamId: $StreamId"
        }
    } catch {
        Write-Host "Could not auto-select streamId - pass -StreamId"
    }
}

if ($StreamId) {
    $encStream = [uri]::EscapeDataString($StreamId)
    Measure-Endpoint -Label "detail (sparse)" -Url "$BaseUrl/v1/analytics/streams/${encStream}?sparse=true"
    Measure-Endpoint -Label "detail (full)" -Url "$BaseUrl/v1/analytics/streams/${encStream}?sparse=false"
    Measure-Endpoint -Label "games" -Url "$BaseUrl/v1/analytics/streams/$encStream/games" -Optional
} else {
    Write-Host "Skipping stream detail/games - no StreamId"
}

Write-Host ""
Write-Host "Record p50/p95 in .kiro/steering/analytics.md after tuning."
