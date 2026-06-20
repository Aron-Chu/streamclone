# Flame / FlashProxy reseller API client for benchmark preflight and plan discovery.
# Auth: Authorization: Bearer $PROXY_API_KEY (fp_live_* production, fp_test_* sandbox)

$script:FlameApiDefaultBase = 'https://rapi.flashproxy.com/api/v1'
$script:FlameApiResolvedBase = $null
$script:FlamePlanCache = @{}

function Resolve-FlameApiBaseUrl {
    param([hashtable]$EnvValues = @{})
    if ($script:FlameApiResolvedBase) {
        return $script:FlameApiResolvedBase
    }
    $explicit = [string]$EnvValues['PROXY_API_BASE_URL']
    if ([string]::IsNullOrWhiteSpace($explicit)) {
        $explicit = [string]$env:PROXY_API_BASE_URL
    }
    if (-not [string]::IsNullOrWhiteSpace($explicit)) {
        $script:FlameApiResolvedBase = $explicit.TrimEnd('/')
        return $script:FlameApiResolvedBase
    }
    $apiKey = [string]$EnvValues['PROXY_API_KEY']
    if ([string]::IsNullOrWhiteSpace($apiKey)) { $apiKey = [string]$env:PROXY_API_KEY }
    if ($apiKey.StartsWith('fp_test_')) {
        $script:FlameApiResolvedBase = 'https://rapi.flashproxy.com/sandbox/api/v1'
        return $script:FlameApiResolvedBase
    }
    $script:FlameApiResolvedBase = $script:FlameApiDefaultBase
    return $script:FlameApiResolvedBase
}

function Get-FlameApiConfig {
    param([hashtable]$EnvValues = @{})
    $apiKey = [string]$EnvValues['PROXY_API_KEY']
    if ([string]::IsNullOrWhiteSpace($apiKey)) {
        $apiKey = [string]$env:PROXY_API_KEY
    }
    $baseUrl = Resolve-FlameApiBaseUrl -EnvValues $EnvValues
    return @{
        ApiKey  = $apiKey.Trim()
        BaseUrl = $baseUrl.TrimEnd('/')
    }
}

function Invoke-FlameApi {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [ValidateSet('GET', 'POST', 'PUT', 'DELETE')][string]$Method = 'GET',
        [hashtable]$Query = @{},
        [hashtable]$EnvValues = @{}
    )
    $cfg = Get-FlameApiConfig -EnvValues $EnvValues
    if ([string]::IsNullOrWhiteSpace($cfg.ApiKey)) {
        throw 'PROXY_API_KEY is not set in .env.local'
    }
    $uri = "$($cfg.BaseUrl)$Path"
    if ($Query.Count -gt 0) {
        $qs = ($Query.GetEnumerator() | ForEach-Object {
            "$([uri]::EscapeDataString($_.Key))=$([uri]::EscapeDataString([string]$_.Value))"
        }) -join '&'
        $uri = "$uri`?$qs"
    }
    $headers = @{
        Authorization = "Bearer $($cfg.ApiKey)"
        Accept        = 'application/json'
    }
    $maxAttempts = 4
    for ($attempt = 1; $attempt -le $maxAttempts; $attempt++) {
        try {
            return Invoke-RestMethod -Uri $uri -Method $Method -Headers $headers -TimeoutSec 30
        } catch {
            $status = $null
            $body = $null
            if ($_.Exception.Response) {
                $status = [int]$_.Exception.Response.StatusCode
                try {
                    $reader = New-Object System.IO.StreamReader($_.Exception.Response.GetResponseStream())
                    $body = $reader.ReadToEnd()
                    $reader.Close()
                } catch { }
            }
            if ($status -eq 429 -and $attempt -lt $maxAttempts) {
                $delay = [Math]::Min(60, 5 * $attempt)
                Write-Warning "Flame API rate limited on $Path; retry $attempt/$maxAttempts in ${delay}s"
                Start-Sleep -Seconds $delay
                continue
            }
            throw "Flame API $Method $Path failed (HTTP $status): $($_.Exception.Message) $body"
        }
    }
}

function Get-FlameBalance {
    param([hashtable]$EnvValues = @{})
    return Invoke-FlameApi -Path '/balance' -EnvValues $EnvValues
}

function Get-FlameActivePlans {
    param(
        [string]$Product = '',
        [hashtable]$EnvValues = @{}
    )
    $query = @{ status = 'active'; per_page = '50'; page = '1' }
    if ($Product) { $query['product'] = $Product }
    $resp = Invoke-FlameApi -Path '/plans' -Query $query -EnvValues $EnvValues
    $plans = @()
    if ($resp.data -and $resp.data.plans) {
        $plans = @($resp.data.plans)
    } elseif ($resp.plans) {
        $plans = @($resp.plans)
    } elseif ($resp.data -is [array]) {
        $plans = @($resp.data)
    }
    return $plans
}

function Get-FlamePlanDetail {
    param(
        [Parameter(Mandatory = $true)][string]$PlanId,
        [hashtable]$EnvValues = @{}
    )
    $resp = Invoke-FlameApi -Path "/plans/$PlanId" -EnvValues $EnvValues
    if ($resp.data) { return $resp.data }
    return $resp
}

function Get-FlameUsageSummary {
    param(
        [ValidateSet('today', 'week', 'month', 'all')][string]$Period = 'today',
        [hashtable]$EnvValues = @{}
    )
    return Invoke-FlameApi -Path '/usage/summary' -Query @{ period = $Period } -EnvValues $EnvValues
}

function Get-FlameConnectionInfo {
    param([hashtable]$EnvValues = @{})
    return Invoke-FlameApi -Path '/proxies/connection-info' -EnvValues $EnvValues
}

function ConvertTo-FlameGbRemaining {
    param($Allocation)
    if (-not $Allocation) { return $null }
    if ($null -ne $Allocation.gb_remaining) {
        return [double]$Allocation.gb_remaining
    }
    if ($null -ne $Allocation.bytes_remaining) {
        return [math]::Round([double]$Allocation.bytes_remaining / 1GB, 4)
    }
    return $null
}

function Get-FlameUsageSnapshot {
    param([hashtable]$EnvValues = @{})
    $balance = Get-FlameBalance -EnvValues $EnvValues
    $data = $balance.data
    if (-not $data) { $data = $balance }
    $alloc = $data.allocations
    $snapshot = [ordered]@{
        balanceFormatted = [string]$data.balance_formatted
        residentialGb    = ConvertTo-FlameGbRemaining -Allocation $alloc.residential
        residentialLiteGb = ConvertTo-FlameGbRemaining -Allocation $alloc.'residential-lite'
        mobileGb         = ConvertTo-FlameGbRemaining -Allocation $alloc.mobile
        totalSpentFormatted = [string]$data.total_spent_formatted
    }
    return $snapshot
}

function Resolve-FlamePlanId {
    param(
        [Parameter(Mandatory = $true)][string]$Product,
        [hashtable]$EnvValues = @{}
    )
    $envKey = switch ($Product) {
        'residential' { 'PROXY_FLAME_PLAN_PREMIUM_ID' }
        'residential-lite' { 'PROXY_FLAME_PLAN_BUDGET_ID' }
        default { throw "Unknown product: $Product" }
    }
    $explicit = [string]$EnvValues[$envKey]
    if ($explicit) { return $explicit.Trim() }

    $staticUser = switch ($Product) {
        'residential' { [string]$EnvValues['PROXY_FLAME_PREMIUM_USERNAME'] }
        'residential-lite' { [string]$EnvValues['PROXY_FLAME_BUDGET_USERNAME'] }
    }
    $staticPrefix = ($staticUser -replace '-session-[a-zA-Z0-9]+$', '').Split('-')[0]

    $plans = Get-FlameActivePlans -Product $Product -EnvValues $EnvValues
    foreach ($plan in $plans) {
        $planId = [string](if ($plan.plan_id) { $plan.plan_id } elseif ($plan.id) { $plan.id } else { $plan.planId })
        $user = [string](if ($plan.proxy_username) { $plan.proxy_username } elseif ($plan.username) { $plan.username } else { '' })
        $prod = [string](if ($plan.product) { $plan.product } else { '' })
        if ($prod -eq $Product -and $staticPrefix -and $user.StartsWith($staticPrefix)) {
            return $planId
        }
    }
    foreach ($plan in $plans) {
        $prod = [string](if ($plan.product) { $plan.product } else { '' })
        if ($prod -eq $Product) {
            return [string](if ($plan.plan_id) { $plan.plan_id } elseif ($plan.id) { $plan.id } else { $plan.planId })
        }
    }
    return $null
}

function ConvertFrom-FlamePlanToProxyConfig {
    param(
        [Parameter(Mandatory = $true)]$PlanDetail,
        [string]$SessionSuffix = '',
        [hashtable]$EnvValues = @{}
    )
    $hostname = [string]$PlanDetail.connection.hostname
    $port = [int](if ($PlanDetail.connection.port_http) { $PlanDetail.connection.port_http } else { 8989 })
    $username = [string]$PlanDetail.proxy_username
    $password = [string]$PlanDetail.proxy_password
    if (-not $hostname) {
        # Fallback to static env when API omits connection block
        $product = [string]$PlanDetail.product
        $hostname = switch ($product) {
            'residential' {
                ([string]$EnvValues['PROXY_FLAME_PREMIUM_SERVER'] -replace '^https?://', '' -split ':')[0]
            }
            'residential-lite' {
                ([string]$EnvValues['PROXY_FLAME_BUDGET_SERVER'] -replace '^https?://', '' -split ':')[0]
            }
            default { 'proxy.flameproxies.com' }
        }
        $port = switch ($product) {
            'residential' {
                $s = [string]$EnvValues['PROXY_FLAME_PREMIUM_SERVER']
                if ($s -match ':(\d+)') { [int]$matches[1] } else { 8989 }
            }
            'residential-lite' {
                $s = [string]$EnvValues['PROXY_FLAME_BUDGET_SERVER']
                if ($s -match ':(\d+)') { [int]$matches[1] } else { 8989 }
            }
            default { 8989 }
        }
    }
    if ($SessionSuffix -and $username) {
        $base = ($username -replace '-session-[a-zA-Z0-9]+$', '')
        $username = "$base-session-$SessionSuffix"
    }
    return @{
        Server   = "http://${hostname}:$port"
        Username = $username
        Password = $password
        PlanId   = [string](if ($PlanDetail.plan_id) { $PlanDetail.plan_id } elseif ($PlanDetail.planId) { $PlanDetail.planId } else { '' })
        Product  = [string]$PlanDetail.product
    }
}

function Get-FlameProxyConfig {
    param(
        [Parameter(Mandatory = $true)]
        [ValidateSet('residential', 'residential-lite')]
        [string]$Product,
        [string]$SessionSuffix = '',
        [hashtable]$EnvValues = @{}
    )
    $cacheKey = "$Product::$SessionSuffix"
    if ($script:FlamePlanCache.ContainsKey($cacheKey)) {
        return $script:FlamePlanCache[$cacheKey]
    }
    $planId = Resolve-FlamePlanId -Product $Product -EnvValues $EnvValues
    if (-not $planId) {
        throw "No active Flame plan found for product=$Product"
    }
    $detail = Get-FlamePlanDetail -PlanId $planId -EnvValues $EnvValues
    $cfg = ConvertFrom-FlamePlanToProxyConfig -PlanDetail $detail -SessionSuffix $SessionSuffix -EnvValues $EnvValues
    if (-not $SessionSuffix) {
        $script:FlamePlanCache[$cacheKey] = $cfg
    }
    return $cfg
}

function Test-FlameGbRemaining {
    param(
        [hashtable]$EnvValues = @{},
        [double]$MinGbRemaining = 0.5,
        [string[]]$Products = @('residential', 'residential-lite')
    )
    try {
        $snap = Get-FlameUsageSnapshot -EnvValues $EnvValues
    } catch {
        return @{ ok = $true; issues = @(); snapshot = $null; skipped = $true }
    }
    $issues = @()
    foreach ($product in $Products) {
        $gb = switch ($product) {
            'residential' { $snap.residentialGb }
            'residential-lite' { $snap.residentialLiteGb }
            default { $null }
        }
        if ($null -ne $gb -and $gb -lt $MinGbRemaining) {
            $issues += "$product remaining ${gb}GB < ${MinGbRemaining}GB"
        }
    }
    return @{
        ok     = ($issues.Count -eq 0)
        issues = $issues
        snapshot = $snap
    }
}

function Get-FlamePlanBytesUsed {
    param(
        [Parameter(Mandatory = $true)][string]$PlanId,
        [hashtable]$EnvValues = @{}
    )
    $detail = Get-FlamePlanDetail -PlanId $PlanId -EnvValues $EnvValues
    $bytes = $detail.limits.bytes_used
    if ($null -eq $bytes) { $bytes = 0 }
    return [int64]$bytes
}

function Compare-FlameProxyConfigs {
    param(
        [hashtable]$Static,
        [hashtable]$Api
    )
    return [ordered]@{
        serverMatch   = ($Static.Server -eq $Api.Server)
        usernameMatch = ($Static.Username -eq $Api.Username)
        staticServer  = $Static.Server
        apiServer     = $Api.Server
        staticUser    = $Static.Username
        apiUser       = $Api.Username
    }
}
