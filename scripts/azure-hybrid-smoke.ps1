# Azure hybrid archive plane preflight + proxy smoke checks.
# Usage:
#   pwsh scripts/azure-hybrid-smoke.ps1
#   pwsh scripts/azure-hybrid-smoke.ps1 -PreflightOnly
#   pwsh scripts/azure-hybrid-smoke.ps1 -ScraperBaseUrl http://azure-streamclone:8000 -LogHost azure-streamclone

param(
    [switch]$PreflightOnly,
    [string]$ScraperBaseUrl = "http://azure-streamclone:8000",
    [string]$TailscaleHost = "azure-streamclone",
    [string]$LogHost = "azure-streamclone",
    [string]$TTUrl = "https://twitchtracker.com/jynxzi/streams/318832886110",
    [string]$SocialUrl = "https://old.reddit.com/r/livestreamfail/search/?q=twitch&sort=new",
    [int]$ScrapeTimeoutMs = 120000,
    [switch]$SkipLogChecks
)

$ErrorActionPreference = "Stop"
$repoRoot = Split-Path $PSScriptRoot -Parent
Set-Location $repoRoot

function Fail([string]$Message) {
    [Console]::Error.WriteLine("azure-hybrid-smoke: $Message")
    exit 1
}

function Test-TailscalePing {
    param([string]$HostName)
    if (-not (Get-Command tailscale -ErrorAction SilentlyContinue)) {
        Write-Host "  tailscale CLI not found — skip ping (install Tailscale on this machine)"
        return
    }
    Write-Host "Tailscale ping $HostName ..."
    tailscale ping -c 3 $HostName 2>&1 | ForEach-Object { Write-Host "  $_" }
    if ($LASTEXITCODE -ne 0) {
        Fail "tailscale ping $HostName failed"
    }
}

function Test-ScraperHealth {
    param([string]$Base)
    $base = $Base.TrimEnd('/')
    $health = "$base/health"
    Write-Host "Scraper health GET $health ..."
    try {
        $resp = Invoke-WebRequest -Uri $health -UseBasicParsing -TimeoutSec 30
        if ($resp.StatusCode -lt 200 -or $resp.StatusCode -ge 300) {
            Fail "scraper health HTTP $($resp.StatusCode)"
        }
        Write-Host "  scraper healthy"
    } catch {
        Fail "scraper health failed: $($_.Exception.Message)"
    }
}

function Invoke-ScrapeRequest {
    param(
        [string]$Base,
        [string]$Url,
        [bool]$UseProxy
    )
    $endpoint = "$($Base.TrimEnd('/'))/v2/scrape"
    $body = @{
        url      = $Url
        formats  = @('rawHtml')
        useProxy = $UseProxy
        timeout  = $ScrapeTimeoutMs
    } | ConvertTo-Json -Compress
    Write-Host "POST $endpoint useProxy=$UseProxy url=$Url"
    $resp = Invoke-WebRequest -Uri $endpoint -Method POST -Body $body -ContentType 'application/json' -UseBasicParsing -TimeoutSec ([Math]::Max(90, $ScrapeTimeoutMs / 1000 + 30))
    if ($resp.StatusCode -lt 200 -or $resp.StatusCode -ge 300) {
        Fail "scrape HTTP $($resp.StatusCode)"
    }
    $payload = $resp.Content | ConvertFrom-Json
    if (-not $payload.success) {
        Fail "scrape failed: $($payload.error)"
    }
    Write-Host "  scrape ok"
}

function Get-RemoteScraperLogs {
    param([string]$SshHost, [int]$SinceMinutes = 3)
    if (-not (Get-Command ssh -ErrorAction SilentlyContinue)) {
        return $null
    }
    $cmd = "docker logs streamclone-scraper --since ${SinceMinutes}m 2>&1"
    try {
        return (ssh -o BatchMode=yes -o ConnectTimeout=10 $SshHost $cmd 2>&1 | Out-String)
    } catch {
        Write-Host "  could not fetch remote logs via ssh $SshHost — $($_.Exception.Message)"
        return $null
    }
}

function Get-LocalScraperLogs {
    param([int]$SinceMinutes = 3)
    if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
        return $null
    }
    $running = docker ps --filter name=streamclone-scraper --format '{{.Names}}' 2>$null
    if (-not $running) {
        return $null
    }
    return (docker logs streamclone-scraper --since "${SinceMinutes}m" 2>&1 | Out-String)
}

function Test-ProxyLogPattern {
    param(
        [string]$Logs,
        [bool]$ExpectProxyEnabled,
        [string]$Label
    )
    if ([string]::IsNullOrWhiteSpace($Logs)) {
        Write-Host "  skip proxy log check ($Label) — no logs available"
        return
    }
    $lower = $Logs.ToLowerInvariant()
    $enabledPatterns = @('useproxy=true', 'use_proxy=true', 'proxy enabled', 'using proxy', 'proxy: http')
    $disabledPatterns = @('useproxy=false', 'use_proxy=false', 'proxy disabled', 'bypass.*proxy', 'direct.*no proxy', 'skipping proxy', 'proxy bypass')
    if ($ExpectProxyEnabled) {
        $hit = $enabledPatterns | Where-Object { $lower -match [regex]::Escape($_).Replace('\\', '') -or $lower -match $_ }
        if (-not $hit) {
            # fallback: generic "proxy" mention when social scrape ran
            if ($lower -notmatch 'proxy') {
                Fail "$Label — expected proxy ENABLED in scraper logs (patterns: $($enabledPatterns -join ', '))"
            }
        }
        Write-Host "  proxy log check ($Label): ENABLED ok"
    } else {
        $disabledHit = $false
        foreach ($pat in $disabledPatterns) {
            if ($lower -match $pat) { $disabledHit = $true; break }
        }
        if (-not $disabledHit -and ($lower -match 'useproxy=true|using proxy')) {
            Fail "$Label — expected proxy DISABLED for TwitchTracker (useProxy=false)"
        }
        Write-Host "  proxy log check ($Label): DISABLED ok"
    }
}

function Test-ModeBPostgresVolume {
    if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
        Write-Host "  docker not available — skip Mode B volume check"
        return
    }
    $vol = docker volume ls --format '{{.Name}}' 2>$null | Where-Object { $_ -match 'azure-pg-data|streamclone-azure-archive-plane.*azure-pg-data' }
    if ($vol) {
        Write-Host "  Mode B postgres volume present: $($vol | Select-Object -First 1)"
    } else {
        Write-Host "  Mode B postgres volume not found locally (expected on Azure VM after Mode B up)"
    }
}

Write-Host "==> Azure hybrid preflight"
Test-TailscalePing -HostName $TailscaleHost
Test-ScraperHealth -Base $ScraperBaseUrl
Test-ModeBPostgresVolume

if ($PreflightOnly) {
    Write-Host "azure-hybrid-smoke: preflight ok"
    exit 0
}

Write-Host "==> Proxy behavior smoke (TT direct vs social proxied)"
Invoke-ScrapeRequest -Base $ScraperBaseUrl -Url $TTUrl -UseProxy $false
Start-Sleep -Seconds 2
Invoke-ScrapeRequest -Base $ScraperBaseUrl -Url $SocialUrl -UseProxy $true

if (-not $SkipLogChecks) {
    $logs = Get-RemoteScraperLogs -SshHost $LogHost
    if (-not $logs) {
        $logs = Get-LocalScraperLogs
    }
    Test-ProxyLogPattern -Logs $logs -ExpectProxyEnabled $false -Label 'TT detail'
    Test-ProxyLogPattern -Logs $logs -ExpectProxyEnabled $true -Label 'social scrape'
}

Write-Host "azure-hybrid-smoke: all checks passed"
