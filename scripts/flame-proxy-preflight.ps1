# Validate Flame/FlashProxy API key, balance, and active residential plans.
param(
    [double]$MinGbRemaining = 0.5,
    [switch]$Strict
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
Set-Location $repoRoot
. (Join-Path $PSScriptRoot 'lib\env.ps1')
. (Join-Path $PSScriptRoot 'lib\flame-proxy-api.ps1')

function Read-LocalEnv {
    $merged = @{}
    foreach ($path in @((Join-Path $repoRoot '.env'), (Join-Path $repoRoot '.env.local'))) {
        if (-not (Test-Path $path)) { continue }
        foreach ($entry in (Read-EnvKeyValueFile -Path $path).GetEnumerator()) {
            $merged[$entry.Key] = $entry.Value
        }
    }
    return $merged
}

$envValues = Read-LocalEnv
$cfg = Get-FlameApiConfig -EnvValues $envValues
if ([string]::IsNullOrWhiteSpace($cfg.ApiKey)) {
    Write-Error 'PROXY_API_KEY missing in .env.local'
    exit 1
}

Write-Host "Flame API preflight -> $($cfg.BaseUrl)" -ForegroundColor Cyan

try {
    $balance = Get-FlameUsageSnapshot -EnvValues $envValues
    Write-Host "  Balance: $($balance.balanceFormatted)" -ForegroundColor Green
    Write-Host "  residential GB remaining: $($balance.residentialGb)"
    Write-Host "  residential-lite GB remaining: $($balance.residentialLiteGb)"
    if ($cfg.BaseUrl -match '/sandbox/') {
        Write-Warning '  Using sandbox API — production key may be invalid; static PROXY_FLAME_* creds used for proxy egress'
    }
} catch {
    Write-Warning "GET /balance failed: $_"
    Write-Warning 'Continuing — proxy benchmark can use static PROXY_FLAME_* from .env.local'
    if ($Strict) { exit 1 }
}

try {
    $plans = Get-FlameActivePlans -EnvValues $envValues
    Write-Host "  Active plans: $($plans.Count)" -ForegroundColor Green
    foreach ($plan in $plans) {
        $planId = [string](if ($plan.plan_id) { $plan.plan_id } elseif ($plan.id) { $plan.id } else { $plan.planId })
        $prod = [string](if ($plan.product) { $plan.product } else { '?' })
        $user = [string](if ($plan.proxy_username) { $plan.proxy_username } elseif ($plan.username) { $plan.username } else { '' })
        $status = [string](if ($plan.status) { $plan.status } else { 'active' })
        $masked = if ($user.Length -gt 8) { $user.Substring(0, 8) + '...' } else { $user }
        Write-Host "    - $prod planId=$planId user=$masked status=$status"
    }
} catch {
    Write-Warning "GET /plans failed: $_"
    if ($Strict) { exit 1 }
}

foreach ($product in @('residential', 'residential-lite')) {
    try {
        $proxy = Get-FlameProxyConfig -Product $product -EnvValues $envValues
        $hostOnly = ($proxy.Server -replace '^https?://', '')
        Write-Host "  Resolved $product -> $hostOnly planId=$($proxy.PlanId)" -ForegroundColor Green
    } catch {
        Write-Warning "  Could not resolve $product plan: $_"
        if ($Strict) { exit 1 }
    }
}

$gate = Test-FlameGbRemaining -EnvValues $envValues -MinGbRemaining $MinGbRemaining
if ($gate.skipped) {
    Write-Warning '  GB remaining check skipped (API unavailable or rate limited)'
} elseif (-not $gate.ok) {
    foreach ($issue in $gate.issues) {
        Write-Warning "  $issue"
    }
    if ($Strict) { exit 1 }
} else {
    Write-Host '  GB remaining check: OK' -ForegroundColor Green
}

Write-Host 'Flame API preflight passed.' -ForegroundColor Green
