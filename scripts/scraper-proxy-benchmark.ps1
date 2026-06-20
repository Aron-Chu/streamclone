# Benchmark scraper targets across direct egress vs Flame residential proxy profiles.
param(
    [string[]]$Profiles = @('direct', 'premium', 'budget'),
    [string]$OutFile = '',
    [int]$ScrapeTimeoutMs = 120000,
    # Retry failed probes with a fresh Flame sticky-session id (new residential IP).
    [int]$MaxAttemptsPerProbe = 1,
    [switch]$RotateSessionOnFail,
    # Recreate scraper container between retry attempts (fresh Camoufox; slower, higher success on bad sessions).
    [switch]$RecreateScraperOnRetry,
    # Alternate premium/budget credentials on each retry (uses PROXY_POOL when both are configured).
    [switch]$RotatePoolOnFail,
    # Resolve premium/budget from Flame API; adds usage accounting to JSON report.
    [switch]$UseFlameApi,
    [double]$MinGbRemaining = 0.5
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
Set-Location $repoRoot
. (Join-Path $PSScriptRoot 'lib\env.ps1')
. (Join-Path $PSScriptRoot 'lib\stack-progress.ps1')
. (Join-Path $PSScriptRoot 'lib\flame-proxy-api.ps1')

if ($MaxAttemptsPerProbe -gt 1 -and -not $RotateSessionOnFail) {
    $RotateSessionOnFail = $true
}

function Read-LocalEnv {
    $merged = @{}
    $paths = @(
        (Join-Path $repoRoot '.env'),
        (Join-Path $repoRoot '.env.local')
    )
    foreach ($path in $paths) {
        if (-not (Test-Path $path)) { continue }
        foreach ($entry in (Read-EnvKeyValueFile -Path $path).GetEnumerator()) {
            $merged[$entry.Key] = $entry.Value
        }
    }
    return $merged
}

function Get-StaticProxyConfig {
    param(
        [string]$ProfileName,
        [hashtable]$EnvValues,
        [string]$SessionSuffix = ''
    )
    $cfg = switch ($ProfileName) {
        'direct' { @{ Server = ''; Username = ''; Password = '' } }
        'premium' {
            @{
                Server   = [string]$EnvValues['PROXY_FLAME_PREMIUM_SERVER']
                Username = [string]$EnvValues['PROXY_FLAME_PREMIUM_USERNAME']
                Password = [string]$EnvValues['PROXY_FLAME_PREMIUM_PASSWORD']
            }
        }
        'budget' {
            @{
                Server   = [string]$EnvValues['PROXY_FLAME_BUDGET_SERVER']
                Username = [string]$EnvValues['PROXY_FLAME_BUDGET_USERNAME']
                Password = [string]$EnvValues['PROXY_FLAME_BUDGET_PASSWORD']
            }
        }
        default { throw "Unknown static profile: $ProfileName" }
    }
    if ($SessionSuffix -and $cfg.Username) {
        $base = ($cfg.Username -replace '-session-[a-zA-Z0-9]+$', '')
        $cfg.Username = "$base-session-$SessionSuffix"
    }
    return $cfg
}

function Get-ProxyConfig {
    param(
        [string]$ProfileName,
        [hashtable]$EnvValues,
        [string]$SessionSuffix = '',
        [hashtable]$FlamePlanBytesBefore = @{}
    )
    $cfg = switch ($ProfileName) {
        'direct' {
            @{
                Server   = ''
                Username = ''
                Password = ''
            }
        }
        'premium' {
            $s = Get-StaticProxyConfig -ProfileName 'premium' -EnvValues $EnvValues -SessionSuffix $SessionSuffix
            $s.Source = 'static'
            $s
        }
        'budget' {
            $s = Get-StaticProxyConfig -ProfileName 'budget' -EnvValues $EnvValues -SessionSuffix $SessionSuffix
            $s.Source = 'static'
            $s
        }
        'api_premium' {
            if ($UseFlameApi) {
                $api = Get-FlameProxyConfig -Product 'residential' -SessionSuffix $SessionSuffix -EnvValues $EnvValues
                $api.Source = 'api'
                $api
            } else {
                throw 'api_premium requires -UseFlameApi'
            }
        }
        'api_budget' {
            if ($UseFlameApi) {
                $api = Get-FlameProxyConfig -Product 'residential-lite' -SessionSuffix $SessionSuffix -EnvValues $EnvValues
                $api.Source = 'api'
                $api
            } else {
                throw 'api_budget requires -UseFlameApi'
            }
        }
        default { throw "Unknown profile: $ProfileName" }
    }

    if ($UseFlameApi -and $ProfileName -in @('premium', 'budget')) {
        $product = if ($ProfileName -eq 'premium') { 'residential' } else { 'residential-lite' }
        if (-not $script:FlameApiStaticResolved) { $script:FlameApiStaticResolved = @{} }
        if (-not $script:FlameApiStaticResolved.ContainsKey($ProfileName)) {
            try {
                $api = Get-FlameProxyConfig -Product $product -SessionSuffix '' -EnvValues $EnvValues
                $script:FlameApiStaticResolved[$ProfileName] = $api
            } catch {
                Write-Warning "Flame API resolve failed for $ProfileName, using static env: $_"
                $script:FlameApiStaticResolved[$ProfileName] = $null
            }
        }
        $api = $script:FlameApiStaticResolved[$ProfileName]
        if ($api -and $api.Server -and $api.Username -and $api.Password) {
            $cfg.Server = $api.Server
            $cfg.Username = $api.Username
            $cfg.Password = $api.Password
            $cfg.PlanId = $api.PlanId
            $cfg.Product = $api.Product
            $cfg.Source = 'api_with_static_fallback'
        }
    }

    if ($SessionSuffix -and $cfg.Username -and $ProfileName -notin @('api_premium', 'api_budget')) {
        $base = ($cfg.Username -replace '-session-[a-zA-Z0-9]+$', '')
        $cfg.Username = "$base-session-$SessionSuffix"
    }
    return $cfg
}

function Get-FlameProductForProfile {
    param([string]$ProfileName)
    switch ($ProfileName) {
        'premium' { return 'residential' }
        'budget' { return 'residential-lite' }
        'api_premium' { return 'residential' }
        'api_budget' { return 'residential-lite' }
        default { return $null }
    }
}

function Test-ProfileGbGate {
    param(
        [string]$ProfileName,
        [hashtable]$EnvValues,
        [double]$MinGbRemaining
    )
    $product = Get-FlameProductForProfile -ProfileName $ProfileName
    if (-not $product) { return @{ ok = $true } }
    return Test-FlameGbRemaining -EnvValues $EnvValues -MinGbRemaining $MinGbRemaining -Products @($product)
}

function New-FlameSessionSuffix {
    return ([guid]::NewGuid().ToString('N')).Substring(0, 12)
}

function Format-ProxyPoolEntry {
    param([hashtable]$ProxyConfig)
    if (-not $ProxyConfig.Server) { return $null }
    $server = $ProxyConfig.Server -replace '^https?://', ''
    return "$server`:$($ProxyConfig.Username):$($ProxyConfig.Password)"
}

function Get-PoolProxyConfigs {
    param([hashtable]$EnvValues)
    $configs = @()
    foreach ($name in @('premium', 'budget')) {
        $cfg = Get-StaticProxyConfig -ProfileName $name -EnvValues $EnvValues
        if ($cfg.Server -and $cfg.Username -and $cfg.Password) {
            $configs += @{ profile = $name; config = $cfg }
        }
    }
    return $configs
}

function Invoke-ComposeScraper {
    param(
        [hashtable]$ProxyConfig,
        [string[]]$ProxyPoolEntries = @()
    )
    $useImages = (Test-Path (Join-Path $repoRoot 'VERSION'))
    $sourceBuild = Test-ScraperBuildFromSource -Root $repoRoot
    $composeArgs = Get-StreamcloneComposeArgs -Root $repoRoot -Profile 'scraper' -UseImages:$useImages -ScraperSourceBuild:$sourceBuild
    $args = @('up', '-d', '--no-deps', '--force-recreate')
    if ($sourceBuild) { $args += '--build' }
    $args += 'scraper'

    $saved = @{
        PROXY_SERVER   = $env:PROXY_SERVER
        PROXY_USERNAME = $env:PROXY_USERNAME
        PROXY_PASSWORD = $env:PROXY_PASSWORD
        PROXY_POOL     = $env:PROXY_POOL
    }
    try {
        if ($ProxyConfig.Server) {
            $env:PROXY_SERVER = $ProxyConfig.Server
            $env:PROXY_USERNAME = $ProxyConfig.Username
            $env:PROXY_PASSWORD = $ProxyConfig.Password
        } else {
            $env:PROXY_SERVER = ''
            $env:PROXY_USERNAME = ''
            $env:PROXY_PASSWORD = ''
        }
        if ($ProxyPoolEntries.Count -gt 0) {
            $env:PROXY_POOL = ($ProxyPoolEntries -join ',')
        } else {
            $env:PROXY_POOL = ''
        }
        $lastCode = 1
        for ($composeAttempt = 1; $composeAttempt -le 3; $composeAttempt++) {
            $code = Invoke-EnvDocker -Arguments ($composeArgs + $args)
            $lastCode = $code
            if ($code -eq 0) { break }
            if ($composeAttempt -lt 3) {
                Write-Warning "docker compose scraper recreate failed (exit $code); retry $composeAttempt/3 in 8s"
                Start-Sleep -Seconds 8
            }
        }
        if ($lastCode -ne 0) {
            throw "docker compose scraper recreate failed (exit $lastCode)"
        }
        Start-Sleep -Seconds 3
    } finally {
        $env:PROXY_SERVER = $saved.PROXY_SERVER
        $env:PROXY_USERNAME = $saved.PROXY_USERNAME
        $env:PROXY_PASSWORD = $saved.PROXY_PASSWORD
        $env:PROXY_POOL = $saved.PROXY_POOL
    }
}

function Wait-ScraperHealth {
    for ($i = 1; $i -le 90; $i++) {
        $prev = $ErrorActionPreference
        $ErrorActionPreference = 'SilentlyContinue'
        try {
            docker exec streamclone-scraper curl -sf http://127.0.0.1:8000/health 2>$null | Out-Null
            if ($LASTEXITCODE -eq 0) { return }
        } catch {
            # Container may still be recreating.
        } finally {
            $ErrorActionPreference = $prev
        }
        Start-Sleep -Seconds 2
    }
    throw 'scraper health check timed out'
}

function Invoke-Probe {
    param(
        [string]$ProbeId,
        [string]$ProbeUrl,
        [bool]$UseProxy
    )
    $script = Join-Path $repoRoot 'scripts\scrape-test-inline.py'
    docker cp $script streamclone-scraper:/tmp/scrape-probe.py | Out-Null
    $envArgs = @(
        "-e", "SCRAPE_URL=$ProbeUrl",
        "-e", "SCRAPE_PROBE_ID=$ProbeId",
        "-e", "SCRAPE_TIMEOUT_MS=$ScrapeTimeoutMs",
        "-e", "SCRAPE_JSON=true",
        "-e", "USE_PROXY=$($UseProxy.ToString().ToLower())"
    )
    $raw = docker exec @envArgs streamclone-scraper python /tmp/scrape-probe.py 2>&1
    $text = ($raw -join "`n").Trim()
    $line = ($text -split "`n" | Where-Object { $_ -match '^\{' } | Select-Object -Last 1)
    if (-not $line) {
        return @{
            id         = $ProbeId
            url        = $ProbeUrl
            useProxy   = $UseProxy
            success    = $false
            durationMs = 0
            error      = $text
            exitCode   = 1
        }
    }
    return ($line | ConvertFrom-Json)
}

function Invoke-ProbeWithRetries {
    param(
        [string]$ProfileName,
        [hashtable]$EnvValues,
        [hashtable]$InitialProxy,
        [string]$ProbeId,
        [string]$ProbeUrl,
        [bool]$UseProxy,
        [array]$PoolConfigs = @(),
        [scriptblock]$ResolveProxy = $null
    )
    $attempts = @()
    $maxAttempts = [Math]::Max(1, $MaxAttemptsPerProbe)
    $proxy = $InitialProxy
    $poolIndex = 0
    $lastResult = $null

    for ($attempt = 1; $attempt -le $maxAttempts; $attempt++) {
        $sessionSuffix = ''
        if ($UseProxy -and ($RotateSessionOnFail -or $attempt -gt 1)) {
            $sessionSuffix = New-FlameSessionSuffix
            if ($ResolveProxy) {
                $proxy = & $ResolveProxy $sessionSuffix
            } else {
                $proxy = Get-ProxyConfig -ProfileName $ProfileName -EnvValues $EnvValues -SessionSuffix $sessionSuffix
            }
        }

        if ($UseProxy -and $RotatePoolOnFail -and $PoolConfigs.Count -gt 0) {
            $pick = $PoolConfigs[($poolIndex % $PoolConfigs.Count)]
            $poolIndex++
            if (-not $sessionSuffix) { $sessionSuffix = New-FlameSessionSuffix }
            $proxy = Get-ProxyConfig -ProfileName $pick.profile -EnvValues $EnvValues -SessionSuffix $sessionSuffix
        }

        # Recreate only on profile entry (attempt 1 handled by caller) or when rotating session after failure.
        $needsRecreate = ($attempt -gt 1) -and (
            ($UseProxy -and $sessionSuffix) -or
            $RotateSessionOnFail -or
            $RecreateScraperOnRetry -or
            $RotatePoolOnFail
        )
        if ($needsRecreate) {
            $poolEntries = @()
            if ($UseProxy -and $PoolConfigs.Count -gt 1 -and $RotatePoolOnFail) {
                foreach ($entry in $PoolConfigs) {
                    $e = Format-ProxyPoolEntry -ProxyConfig (Get-ProxyConfig -ProfileName $entry.profile -EnvValues $EnvValues -SessionSuffix $(New-FlameSessionSuffix))
                    if ($e) { $poolEntries += $e }
                }
            }
            Write-Host "    attempt $attempt/$maxAttempts recreate scraper session=$sessionSuffix..."
            Invoke-ComposeScraper -ProxyConfig $proxy -ProxyPoolEntries $poolEntries
            Wait-ScraperHealth
        }

        Write-Host "  Probe $ProbeId attempt $attempt/$maxAttempts (useProxy=$UseProxy)..."
        $result = Invoke-Probe -ProbeId $ProbeId -ProbeUrl $ProbeUrl -UseProxy:$UseProxy
        $lastResult = $result
        $attempts += [ordered]@{
            attempt       = $attempt
            sessionSuffix = $sessionSuffix
            proxyProfile  = $ProfileName
            success       = [bool]$result.success
            durationMs    = $result.durationMs
            cloudflare    = [bool]$result.cloudflare
            error         = [string]$result.error
        }
        $mark = if ($result.success) { 'OK' } else { 'FAIL' }
        Write-Host "    $mark ms=$($result.durationMs) cf=$($result.cloudflare)"

        if ($result.success) {
            $result | Add-Member -NotePropertyName attempts -NotePropertyValue $attempts -Force
            return $result
        }

        if (-not $RotateSessionOnFail -and -not $RotatePoolOnFail -and -not $RecreateScraperOnRetry -and $attempt -ge 1) {
            break
        }
        if ($attempt -lt $maxAttempts) {
            Start-Sleep -Seconds 2
        }
    }

    $lastResult | Add-Member -NotePropertyName attempts -NotePropertyValue $attempts -Force
    return $lastResult
}

$probes = @(
    @{ id = 'tt_detail_1'; url = 'https://twitchtracker.com/jynxzi/streams/318832886110' },
    @{ id = 'tt_detail_2'; url = 'https://twitchtracker.com/ishowspeed/streams/318098150359' },
    @{ id = 'tt_list'; url = 'https://twitchtracker.com/xqc/streams' },
    @{ id = 'reddit_json'; url = 'https://old.reddit.com/r/LivestreamFail/hot.json?limit=5&raw_json=1' },
    @{ id = 'reddit_search'; url = 'https://old.reddit.com/r/LivestreamFail/search?q=xqc&restrict_sr=1&sort=new&t=all&limit=8' }
)

$envValues = Read-LocalEnv
$script:FlameApiStaticResolved = @{}
$runAt = (Get-Date).ToUniversalTime().ToString('o')
$profileResults = @()
$flameApiBlock = $null
$plansUsed = @{}
$planBytesBefore = @{}

if ($UseFlameApi) {
    Write-Host 'Flame API: waiting 60s for rate-limit cooldown...' -ForegroundColor DarkGray
    Start-Sleep -Seconds 60
    Write-Host 'Flame API: capturing balance before benchmark...' -ForegroundColor Cyan
    try {
        $gate = Test-FlameGbRemaining -EnvValues $envValues -MinGbRemaining $MinGbRemaining
        if (-not $gate.ok) {
            foreach ($issue in $gate.issues) {
                Write-Warning "Flame API: $issue — proxy profiles may be skipped"
            }
        }
        $balanceBefore = Get-FlameUsageSnapshot -EnvValues $envValues
        foreach ($product in @('residential', 'residential-lite')) {
            try {
                $planId = Resolve-FlamePlanId -Product $product -EnvValues $envValues
                if ($planId) {
                    $planBytesBefore[$planId] = Get-FlamePlanBytesUsed -PlanId $planId -EnvValues $envValues
                }
            } catch { }
        }
        $flameApiBlock = [ordered]@{
            balanceBefore = $balanceBefore
            balanceAfter  = $null
            plansUsed     = @()
        }
    } catch {
        Write-Warning "Flame API preflight skipped: $_"
    }
}

foreach ($profileName in $Profiles) {
    try {
    Write-Host ""
    Write-Host "=== Profile: $profileName ==="
    $proxy = $null
    try {
        $proxy = Get-ProxyConfig -ProfileName $profileName -EnvValues $envValues
    } catch {
        Write-Warning "Skipping $profileName — $($_.Exception.Message)"
        continue
    }
    if ($profileName -ne 'direct' -and $UseFlameApi) {
        $gate = Test-ProfileGbGate -ProfileName $profileName -EnvValues $envValues -MinGbRemaining $MinGbRemaining
        if (-not $gate.ok) {
            foreach ($issue in $gate.issues) {
                Write-Warning "Skipping $profileName — $issue"
            }
            continue
        }
    }
    if ($profileName -ne 'direct' -and [string]::IsNullOrWhiteSpace($proxy.Server)) {
        Write-Warning "Skipping $profileName  -  set PROXY_FLAME_$($profileName.ToUpper())_* in .env.local or use -UseFlameApi"
        continue
    }

    if ($UseFlameApi -and $proxy.PlanId) {
        $plansUsed[$proxy.PlanId] = @{
            planId  = $proxy.PlanId
            product = [string]$proxy.Product
            profile = $profileName
        }
    }

    $staticCompare = $null
    if ($profileName -in @('api_premium', 'api_budget')) {
        $staticName = if ($profileName -eq 'api_premium') { 'premium' } else { 'budget' }
        $staticCfg = Get-StaticProxyConfig -ProfileName $staticName -EnvValues $envValues
        $staticCompare = Compare-FlameProxyConfigs -Static $staticCfg -Api $proxy
    }

    $resolveProxy = {
        param($suffix)
        Get-ProxyConfig -ProfileName $profileName -EnvValues $envValues -SessionSuffix $suffix
    }

    $poolConfigs = @()
    if ($RotatePoolOnFail) {
        $poolConfigs = Get-PoolProxyConfigs -EnvValues $envValues
    }

    $useProxy = ($profileName -ne 'direct')
    Write-Host "Recreating scraper for profile..."
    Invoke-ComposeScraper -ProxyConfig $proxy
    Wait-ScraperHealth

    $probeResults = @()
    foreach ($probe in $probes) {
        if ($MaxAttemptsPerProbe -gt 1 -or $RotateSessionOnFail -or $RecreateScraperOnRetry) {
            $result = Invoke-ProbeWithRetries `
                -ProfileName $profileName `
                -EnvValues $envValues `
                -InitialProxy $proxy `
                -ProbeId $probe.id `
                -ProbeUrl $probe.url `
                -UseProxy:$useProxy `
                -PoolConfigs $poolConfigs `
                -ResolveProxy $resolveProxy
        } else {
            Write-Host "  Probe $($probe.id) (useProxy=$useProxy)..."
            $result = Invoke-Probe -ProbeId $probe.id -ProbeUrl $probe.url -UseProxy:$useProxy
            $mark = if ($result.success) { 'OK' } else { 'FAIL' }
            Write-Host "    $mark ms=$($result.durationMs) cf=$($result.cloudflare)"
        }
        $probeResults += $result
    }

    $profileEntry = [ordered]@{
        name            = $profileName
        proxyConfigured = [bool]$proxy.Server
        proxyServer     = if ($proxy.Server) { $proxy.Server } else { $null }
        proxySource     = if ($proxy.Source) { $proxy.Source } else { $null }
        planId          = if ($proxy.PlanId) { $proxy.PlanId } else { $null }
        product         = if ($proxy.Product) { $proxy.Product } else { $null }
        staticCompare   = $staticCompare
        probes          = $probeResults
    }
    $profileResults += $profileEntry
    } catch {
        Write-Warning "Profile $profileName aborted: $_"
        $profileResults += [ordered]@{
            name            = $profileName
            proxyConfigured = $false
            error           = [string]$_.Exception.Message
            probes          = @()
        }
    }
}

if ($UseFlameApi -and $flameApiBlock) {
    try {
        $flameApiBlock.balanceAfter = Get-FlameUsageSnapshot -EnvValues $envValues
        $usedList = @()
        foreach ($entry in $plansUsed.Values) {
            $entryPlanId = $entry.planId
            $before = if ($planBytesBefore.ContainsKey($entryPlanId)) { $planBytesBefore[$entryPlanId] } else { 0 }
            $after = Get-FlamePlanBytesUsed -PlanId $entryPlanId -EnvValues $envValues
            $usedList += [ordered]@{
                planId          = $entryPlanId
                product         = $entry.product
                profile         = $entry.profile
                bytesUsedDelta  = [int64]($after - $before)
            }
        }
        $flameApiBlock.plansUsed = $usedList
    } catch {
        Write-Warning "Flame API usage snapshot after benchmark failed: $_"
    }
}

if ($profileResults.Count -eq 0) {
    throw 'No profiles ran  -  configure .env.local or pass -Profiles direct'
}

$report = [ordered]@{
    runAt               = $runAt
    host                = $env:COMPUTERNAME
    maxAttemptsPerProbe = $MaxAttemptsPerProbe
    rotateSessionOnFail = [bool]$RotateSessionOnFail
    recreateOnRetry     = [bool]$RecreateScraperOnRetry
    rotatePoolOnFail    = [bool]$RotatePoolOnFail
    useFlameApi         = [bool]$UseFlameApi
    profiles            = $profileResults
}
if ($flameApiBlock) {
    $report.flameApi = $flameApiBlock
}

if ([string]::IsNullOrWhiteSpace($OutFile)) {
    $stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
    $dir = Join-Path $repoRoot 'docs\benchmarks'
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
    $OutFile = Join-Path $dir "scraper-proxy-$stamp.json"
}

($report | ConvertTo-Json -Depth 8) | Set-Content -LiteralPath $OutFile -Encoding UTF8
Write-Host ""
Write-Host "Wrote $($profileResults.Count) profile(s) -> $OutFile"

$targetProfiles = @('premium', 'budget')
$ranTargets = @($profileResults | Where-Object { $_.name -in $targetProfiles })
$failures = @()
foreach ($prof in $ranTargets) {
    foreach ($probe in $prof.probes) {
        if (-not $probe.success) {
            $failures += "$($prof.name)/$($probe.id)"
        }
    }
}
if ($failures.Count -eq 0 -and $ranTargets.Count -gt 0) {
    Write-Host "All premium/budget probes passed." -ForegroundColor Green
    exit 0
}
if ($ranTargets.Count -gt 0) {
    Write-Host "Failing probes: $($failures -join ', ')" -ForegroundColor Yellow
    exit 1
}
exit 0
