#Requires -Version 5.1
# Tiered readiness gates: infra -> apps -> proxy (optional HLS probe).
param(
    [string]$Url = 'http://127.0.0.1:8090/',
    [string]$Root = '',
    [int]$TimeoutSec = 300,
    [int]$IntervalSec = 3,
    [switch]$SkipHLS,
    [string]$HLSChannel = ''
)

$ErrorActionPreference = 'Stop'

. (Join-Path $PSScriptRoot 'stack-progress.ps1')

function Get-WaitStackRoot {
    param([string]$Requested = '')
    if (-not [string]::IsNullOrWhiteSpace($Requested)) {
        return (Resolve-Path -LiteralPath $Requested).Path
    }
    return (Get-EnvRepoRoot)
}

function Wait-StreamclonePollUntil {
    param(
        [string]$Label,
        [scriptblock]$Test,
        [int]$TimeoutSec = 300,
        [int]$IntervalSec = 3
    )
    $deadline = (Get-Date).AddSeconds($TimeoutSec)
    $attempt = 0
    while ((Get-Date) -lt $deadline) {
        $attempt++
        try {
            if (& $Test) {
                Write-Host "  $Label ready (attempt $attempt)" -ForegroundColor Green
                return $true
            }
        } catch { }
        if ($attempt % 5 -eq 0) {
            Write-Host "  waiting for $Label... (attempt $attempt)" -ForegroundColor DarkGray
        }
        Start-Sleep -Seconds $IntervalSec
    }
    Write-Host "  $Label not ready within ${TimeoutSec}s" -ForegroundColor Red
    return $false
}

function Test-StreamcloneInfraServiceReady {
    param(
        [string[]]$ComposeArgs,
        [string]$Service
    )
    $result = Invoke-EnvDockerCaptured -Arguments ($ComposeArgs + @('ps', '--status', 'running', '--format', '{{.Health}}', $Service))
    if ($result.ExitCode -ne 0 -or -not $result.Output) { return $false }
    $health = (($result.Output | Select-Object -First 1) -as [string]).Trim().ToLowerInvariant()
    if ($health -eq 'healthy') { return $true }
    if ([string]::IsNullOrWhiteSpace($health)) {
        $state = Invoke-EnvDockerCaptured -Arguments ($ComposeArgs + @('ps', '--status', 'running', '--format', '{{.State}}', $Service))
        if ($state.ExitCode -eq 0 -and $state.Output) {
            return (($state.Output | Select-Object -First 1).Trim() -eq 'running')
        }
    }
    return $false
}

function Wait-StreamcloneInfraReady {
    param(
        [string]$Root = '',
        [int]$TimeoutSec = 300,
        [int]$IntervalSec = 3
    )
    $resolvedRoot = Get-WaitStackRoot -Requested $Root
    $composeArgs = Get-StreamcloneComposeArgs -Root $resolvedRoot -NoUseImages
    Write-Host 'Tier 1/3: infrastructure (postgres, redis)' -ForegroundColor Cyan
    $ok = Wait-StreamclonePollUntil -Label 'postgres + redis' -TimeoutSec $TimeoutSec -IntervalSec $IntervalSec -Test {
        (Test-StreamcloneInfraServiceReady -ComposeArgs $composeArgs -Service 'postgres') -and
        (Test-StreamcloneInfraServiceReady -ComposeArgs $composeArgs -Service 'redis')
    }
    if (-not $ok) {
        Write-Host 'Infrastructure tier failed. Try: docker compose --env-file .env -f deploy/docker-compose.yml -f deploy/docker-compose.local-tunnel.yml ps' -ForegroundColor Yellow
    }
    return $ok
}

function Wait-StreamcloneAppsReady {
    param(
        [int]$TimeoutSec = 300,
        [int]$IntervalSec = 3
    )
    $checks = @(
        @{ Url = 'http://localhost:8081/healthz'; Label = 'metadata' },
        @{ Url = 'http://localhost:8082/healthz'; Label = 'video' },
        @{ Url = 'http://localhost:8083/healthz'; Label = 'chat' },
        @{ Url = 'http://localhost:8084/healthz'; Label = 'emote' }
    )
    Write-Host 'Tier 2/3: application services' -ForegroundColor Cyan
    $ready = @{}
    $deadline = (Get-Date).AddSeconds($TimeoutSec)
    $attempt = 0
    while ((Get-Date) -lt $deadline) {
        $attempt++
        foreach ($check in $checks) {
            if ($ready.ContainsKey($check.Label)) { continue }
            try {
                $resp = Invoke-WebRequest -Uri $check.Url -UseBasicParsing -TimeoutSec 5
                if ($resp.StatusCode -ge 200 -and $resp.StatusCode -lt 500) {
                    $ready[$check.Label] = $true
                    Write-Host "  $($check.Label) ready (attempt $attempt)" -ForegroundColor Green
                }
            } catch { }
        }
        if ($ready.Count -eq $checks.Count) { return $true }
        if ($attempt % 5 -eq 0) {
            $pending = @($checks | Where-Object { -not $ready.ContainsKey($_.Label) } | ForEach-Object { $_.Label })
            Write-Host "  waiting for apps: $($pending -join ', ') (attempt $attempt)" -ForegroundColor DarkGray
        }
        Start-Sleep -Seconds $IntervalSec
    }
    $pending = @($checks | Where-Object { -not $ready.ContainsKey($_.Label) } | ForEach-Object { $_.Label })
    Write-Host "  application services not ready within ${TimeoutSec}s (pending: $($pending -join ', '))" -ForegroundColor Red
    return $false
}

function Wait-StreamcloneProxyReady {
    param(
        [string]$Url = 'http://127.0.0.1:8090/',
        [string]$Root = '',
        [int]$TimeoutSec = 300,
        [int]$IntervalSec = 3
    )
    Write-Host "Tier 3/3: Caddy proxy ($Url)" -ForegroundColor Cyan
    $proxyOk = Wait-StreamclonePollUntil -Label 'Caddy proxy' -TimeoutSec $TimeoutSec -IntervalSec $IntervalSec -Test {
        $resp = Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 5
        $resp.StatusCode -ge 200 -and $resp.StatusCode -lt 500
    }
    if (-not $proxyOk) { return $false }
    $resolvedRoot = Get-WaitStackRoot -Requested $Root
    if (-not (Test-StreamcloneLocalTunnelHlsCaddyConfig -Root $resolvedRoot)) {
        Write-Host '  HLS proxy config outdated — playback may 401 on main_stream.m3u8. Run Manage Streamclone -> Update.' -ForegroundColor Yellow
    }
    return $true
}

function Wait-StreamcloneHLSReady {
    param(
        [string]$BaseUrl = 'http://127.0.0.1:8090',
        [string]$Channel = '',
        [int]$TimeoutSec = 60
    )
    if ([string]::IsNullOrWhiteSpace($Channel)) {
        $envPath = Join-Path (Get-WaitStackRoot) '.env'
        if (Test-Path $envPath) {
            $vals = Read-EnvKeyValueFile -Path $envPath
            $tracked = [string]$vals['ALWAYS_TRACKED_CHANNELS']
            if (-not [string]::IsNullOrWhiteSpace($tracked)) {
                $Channel = ($tracked -split ',')[0].Trim()
            }
        }
        if ([string]::IsNullOrWhiteSpace($Channel)) { $Channel = 'jynxzi' }
    }

    $enc = [uri]::EscapeDataString($Channel)
    $manifestCandidates = @(
        "$BaseUrl/live/$enc/index.m3u8",
        "$BaseUrl/live/$enc/main_stream.m3u8"
    )

    Write-Host "Optional: HLS probe for channel $Channel" -ForegroundColor Cyan
    $startBody = @{ channel = $Channel } | ConvertTo-Json
    try {
        Invoke-RestMethod -Uri "$BaseUrl/v1/stream/start" -Method Post -Body $startBody -ContentType 'application/json' -TimeoutSec 60 | Out-Null
    } catch {
        Write-Host "  HLS probe: stream start failed ($($_.Exception.Message))" -ForegroundColor Yellow
        return $false
    }

    $deadline = (Get-Date).AddSeconds($TimeoutSec)
    while ((Get-Date) -lt $deadline) {
        foreach ($candidate in $manifestCandidates) {
            try {
                $resp = Invoke-WebRequest -Uri $candidate -UseBasicParsing -TimeoutSec 5
                if ($resp.StatusCode -eq 200) {
                    Write-Host "  HLS manifest ready ($candidate)" -ForegroundColor Green
                    return $true
                }
            } catch { }
        }
        Start-Sleep -Milliseconds 250
    }
    Write-Host '  HLS manifest not ready within timeout (optional tier)' -ForegroundColor Yellow
    return $false
}

function Wait-StreamcloneTieredReadiness {
    param(
        [string]$Root = '',
        [string]$Url = 'http://127.0.0.1:8090/',
        [int]$TimeoutSec = 300,
        [int]$IntervalSec = 3,
        [switch]$SkipHLS,
        [string]$HLSChannel = ''
    )
    $perTier = [math]::Max(30, [math]::Floor($TimeoutSec / 3))
    Write-Host "Waiting for Streamclone tiered readiness (up to ${TimeoutSec}s)..." -ForegroundColor Cyan

    if (-not (Wait-StreamcloneInfraReady -Root $Root -TimeoutSec $perTier -IntervalSec $IntervalSec)) {
        Write-Host 'Required tier failed: infrastructure' -ForegroundColor Red
        return $false
    }
    if (-not (Wait-StreamcloneAppsReady -TimeoutSec $perTier -IntervalSec $IntervalSec)) {
        Write-Host 'Required tier failed: application services' -ForegroundColor Red
        return $false
    }
    if (-not (Wait-StreamcloneProxyReady -Url $Url -Root $Root -TimeoutSec $perTier -IntervalSec $IntervalSec)) {
        Write-Host 'Required tier failed: Caddy proxy' -ForegroundColor Red
        Write-Host 'See: docs/install-desktop.md' -ForegroundColor Yellow
        return $false
    }

    if (-not $SkipHLS) {
        $null = Wait-StreamcloneHLSReady -BaseUrl ($Url.TrimEnd('/')) -Channel $HLSChannel
    }

    Write-Host 'Streamclone tiered readiness: all required tiers passed' -ForegroundColor Green
    return $true
}

if ($MyInvocation.InvocationName -ne '.') {
    $resolvedRoot = Get-WaitStackRoot -Requested $Root
    if (-not (Wait-StreamcloneTieredReadiness -Root $resolvedRoot -Url $Url -TimeoutSec $TimeoutSec -IntervalSec $IntervalSec -SkipHLS:$SkipHLS -HLSChannel $HLSChannel)) {
        exit 1
    }
}
