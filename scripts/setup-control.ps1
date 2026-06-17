#Requires -Version 5.1
# Local-only HTTP helper so the directory status UI can start optional compose profiles.
param(
    [int]$Port = 9191,
    [string]$PidFile = '',
    [string]$Root = ''
)

$ErrorActionPreference = 'Stop'
if ([string]::IsNullOrWhiteSpace($Root)) {
    $Root = Split-Path -Parent $PSScriptRoot
} else {
    $Root = (Resolve-Path -LiteralPath $Root).Path
}
if ([string]::IsNullOrWhiteSpace($PidFile)) {
    $PidFile = Join-Path $Root '.streamclone-setup-control.pid'
}

. (Join-Path $PSScriptRoot 'lib\env.ps1')
. (Join-Path $PSScriptRoot 'lib\stack-progress.ps1')
. (Join-Path $PSScriptRoot 'lib\diagnostics.ps1')

$envPath = Join-Path $Root '.env'
$envValues = if (Test-Path $envPath) { Read-EnvKeyValueFile -Path $envPath } else { @{} }
$setupControlToken = [string]$envValues['SETUP_CONTROL_TOKEN']
$composeLockPath = Join-Path $Root '.streamclone-compose.lock'
$composeQueuePath = Join-Path $Root '.streamclone-start-queue.json'

function Get-StreamcloneComposeLock {
    if (-not (Test-Path $composeLockPath)) { return $null }
    try { return (Get-Content -LiteralPath $composeLockPath -Raw | ConvertFrom-Json) } catch { return $null }
}

function Test-StreamcloneComposeLockActive {
    param($Lock)
    if (-not $Lock) { return $false }
    if (-not $Lock.pid) { return $false }
    return $null -ne (Get-Process -Id ([int]$Lock.pid) -ErrorAction SilentlyContinue)
}

function Read-StreamcloneOptionalStartQueue {
    if (-not (Test-Path $composeQueuePath)) { return @() }
    try {
        $raw = Get-Content -LiteralPath $composeQueuePath -Raw | ConvertFrom-Json
        if ($null -eq $raw) { return @() }
        if ($raw -is [System.Array]) { return @($raw) }
        return @($raw)
    } catch {
        return @()
    }
}

function Write-StreamcloneOptionalStartQueue {
    param([array]$Queue)
    if ($Queue.Count -eq 0) {
        Remove-Item -LiteralPath $composeQueuePath -Force -ErrorAction SilentlyContinue
        return
    }
    Set-Content -LiteralPath $composeQueuePath -Value ($Queue | ConvertTo-Json -Compress) -Encoding UTF8
}

function Add-StreamcloneOptionalStartQueue {
    param([string]$Service)
    $entry = @{ service = $Service; queuedAt = (Get-Date).ToString('o') }
    $queue = Read-StreamcloneOptionalStartQueue
    foreach ($item in $queue) {
        if ($item.service -eq $Service) { return $false }
    }
    $queue += $entry
    Write-StreamcloneOptionalStartQueue -Queue $queue
    return $true
}

function Format-StreamcloneStartDetail {
    param(
        [string]$Service,
        [string]$Detail,
        [string]$Blob = ''
    )
    $text = "$Detail $Blob".Trim()
    if ($text -match 'registry:\s*denied|error from registry:\s*denied') {
        return 'GitHub container registry denied the Analytics image. Building from streamclone-scraper instead (first build takes 5-15 min).'
    }
    if ($text -match 'scraper-preflight:\s*(.+?)(?:\r|$)') {
        $msg = $Matches[1].Trim()
        if ($msg -match 'not running') {
            return 'Waiting for Analytics container — Camoufox warmup will run once Docker finishes.'
        }
        return $msg
    }
    if ($text -match 'WriteErrorException|FullyQualifiedErrorId') {
        return 'Optional service startup hit a script error — check Docker Desktop; retry in a minute.'
    }
    if ($text -match 'docker compose failed') {
        return "Docker compose failed while starting $Service. See .streamclone-start-$Service.log.err in the install folder."
    }
    return $Detail
}

function Start-StreamcloneComposeLockWatcher {
    param(
        [ValidateSet('scraper', 'clipper', 'pulse')]
        [string]$Service,
        [int]$ProcessId
    )
    $worker = Join-Path $PSScriptRoot 'start-profile-service-worker.ps1'
    $psExe = if ($PSVersionTable.PSEdition -eq 'Core') { 'pwsh.exe' } else { 'powershell.exe' }
    $releaseScript = @"
`$lockPath = '$($composeLockPath -replace "'", "''")'
`$queuePath = '$($composeQueuePath -replace "'", "''")'
`$root = '$($Root -replace "'", "''")'
`$service = '$Service'
`$pidToWait = $ProcessId
`$worker = '$($worker -replace "'", "''")'
`$psExe = '$psExe'
try { Wait-Process -Id `$pidToWait -ErrorAction SilentlyContinue } catch { }
if (Test-Path `$lockPath) {
  try {
    `$lock = Get-Content -LiteralPath `$lockPath -Raw | ConvertFrom-Json
    if (`$lock.service -eq `$service -and [int]`$lock.pid -eq `$pidToWait) {
      Remove-Item -LiteralPath `$lockPath -Force -ErrorAction SilentlyContinue
    }
  } catch { Remove-Item -LiteralPath `$lockPath -Force -ErrorAction SilentlyContinue }
}
if (Test-Path `$queuePath) {
  try {
    `$queue = @(Get-Content -LiteralPath `$queuePath -Raw | ConvertFrom-Json)
    if (`$queue.Count -gt 0) {
      `$next = `$queue[0]
      `$rest = @(`$queue | Select-Object -Skip 1)
      if (`$rest.Count -gt 0) { Set-Content -LiteralPath `$queuePath -Value (`$rest | ConvertTo-Json -Compress) -Encoding UTF8 }
      else { Remove-Item -LiteralPath `$queuePath -Force -ErrorAction SilentlyContinue }
      Start-Process -FilePath `$psExe -ArgumentList @('-NoProfile','-ExecutionPolicy','Bypass','-File',`$worker,'-Service',[string]`$next.service,'-Root',`$root) -WindowStyle Hidden | Out-Null
    }
  } catch { }
}
"@
    Start-Process -FilePath $psExe -ArgumentList @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-Command', $releaseScript) -WindowStyle Hidden | Out-Null
}

function Sync-SetupControlTokenFromEnv {
    if (-not (Test-Path $envPath)) { return }
    $script:envValues = Read-EnvKeyValueFile -Path $envPath
    $script:setupControlToken = [string]$envValues['SETUP_CONTROL_TOKEN']
}

function Write-JsonResponse {
    param(
        [System.Net.HttpListenerResponse]$Response,
        [int]$StatusCode,
        [object]$Body
    )
    $json = ($Body | ConvertTo-Json -Compress -Depth 6)
    $bytes = [System.Text.Encoding]::UTF8.GetBytes($json)
    $Response.StatusCode = $StatusCode
    $Response.ContentType = 'application/json; charset=utf-8'
    $Response.Headers.Add('Access-Control-Allow-Origin', '*')
    $Response.ContentLength64 = $bytes.Length
    $Response.OutputStream.Write($bytes, 0, $bytes.Length)
    $Response.Close()
}

function Test-SetupControlAuthorized {
    param([System.Net.HttpListenerRequest]$Request)
    Sync-SetupControlTokenFromEnv
    if ([string]::IsNullOrWhiteSpace($setupControlToken)) {
        return $false
    }
    $provided = $Request.Headers['X-Streamclone-Setup-Token']
    if ([string]::IsNullOrWhiteSpace($provided)) { return $false }
    return ($provided -eq $setupControlToken)
}

function Test-ScraperUseImagesFromEnv {
    return ([string]$envValues['SCRAPER_USE_IMAGES'] -eq '1')
}

function Ensure-ScraperSiblingRepo {
    if (Test-ScraperUseImagesFromEnv) {
        return
    }
    $sibling = Get-EnvScraperSiblingPath
    $hasRepo = (Test-Path (Join-Path $sibling '.git')) -or (Test-Path (Join-Path $sibling 'Dockerfile'))
    if ($hasRepo) { return }
    Write-Host "Cloning streamclone-scraper to $sibling ..."
    $parent = Split-Path -Parent $sibling
    New-Item -ItemType Directory -Path $parent -Force | Out-Null
    $clone = Invoke-EnvCapturedProcess -FilePath 'git' -ArgumentList @('clone', 'https://github.com/Aron-Chu/streamclone-scraper.git', $sibling) -TimeoutSec 300
    if ($clone.ExitCode -ne 0) {
        $log = ($clone.Output -join [Environment]::NewLine).Trim()
        throw "Could not clone streamclone-scraper: $log"
    }
}

function Get-SetupControlUseImages {
    return (Test-ScraperUseImagesFromEnv) -or
        ($envValues['STREAMCLONE_USE_IMAGES'] -eq '1') -or
        (-not [string]::IsNullOrWhiteSpace($envValues['IMAGE_TAG']))
}

function Get-StreamcloneOptionalServiceLabel {
    param([string]$Service)
    switch ($Service) {
        'scraper' { return 'Analytics' }
        'clipper' { return 'Clip Studio' }
        'pulse' { return 'Pulse Dashboards' }
        default { return $Service }
    }
}

function Get-StreamcloneOptionalComposeTargets {
    param([string]$Service)
    if ($Service -eq 'pulse' -and $phase -ne 'Docker reported an error') {
        return @('influxdb', 'grafana', 'analytics')
    }
    return @($Service)
}

function Invoke-ProfileServiceStop {
    param(
        [ValidateSet('scraper', 'clipper', 'pulse')]
        [string]$Service
    )

    Set-Location $Root
    if (-not (Test-Path $envPath)) {
        throw 'Missing .env - run scripts/setup.ps1 first.'
    }

    $useWslDocker = Test-StreamcloneUseWslDockerCli -Root $Root
    $profileForCompose = if ($Service -eq 'pulse' -and (Test-StreamcloneHostPulseReady)) { 'core' } else { $Service }
    $composeArgs = Get-StreamcloneComposeArgs -Root $Root -Profile $profileForCompose -UseImages:(Get-SetupControlUseImages) -RelativePaths:$useWslDocker
    $targets = Get-StreamcloneOptionalComposeTargets -Service $Service
    $stopArgs = $composeArgs + @('stop') + $targets
    $result = Invoke-EnvDockerCaptured -Arguments $stopArgs -Root $Root
    $output = ($result.Output -join [Environment]::NewLine).Trim()
    if ($result.ExitCode -ne 0) {
        throw "docker compose stop failed: $output"
    }
    if ($Service -eq 'pulse') {
        try { [void](Stop-StreamcloneHelmPulsePortForward -Root $Root) } catch { }
    }
    return $output
}

function Invoke-ProfileServiceUp {
    param(
        [ValidateSet('scraper', 'clipper', 'pulse')]
        [string]$Service
    )

    Set-Location $Root
    if (-not (Test-Path $envPath)) {
        throw 'Missing .env - run scripts/setup.ps1 first.'
    }

    $profile = $Service
    if ($Service -eq 'scraper') {
        Ensure-ScraperSiblingRepo
    } elseif ($Service -eq 'pulse') {
        Enable-StreamclonePulseEnv -Root $Root
        $script:envValues = Read-EnvKeyValueFile -Path $envPath
        if (Test-StreamcloneHostPulseReady) {
            [void](Start-StreamcloneHelmPulsePortForward -Root $Root)
        }
    }

    $useImages = Get-SetupControlUseImages
    $pullImages = $useImages -or (Test-StreamcloneUseImagesFromRoot -Root $Root)
    $useWslDocker = Test-StreamcloneUseWslDockerCli -Root $Root
    $profileForCompose = if ($Service -eq 'pulse' -and (Test-StreamcloneHostPulseReady)) { 'core' } else { $Service }
    $composeArgs = Get-StreamcloneComposeArgs -Root $Root -Profile $profileForCompose -UseImages:$useImages -RelativePaths:$useWslDocker
    if ($Service -eq 'pulse') {
        $upArgs = Get-StreamclonePulseComposeUpArgs -ComposeArgs $composeArgs -PullImages:$pullImages -ScraperSourceBuild:$false
    } else {
        $upArgs = $composeArgs + @('up', '-d', '--remove-orphans')
        if ($useImages) { $upArgs += '--pull', 'missing' }
        $upArgs += Get-StreamcloneOptionalComposeTargets -Service $Service
    }
    $result = Invoke-EnvDockerCaptured -Arguments $upArgs -Root $Root
    $output = ($result.Output -join [Environment]::NewLine).Trim()
    if ($result.ExitCode -ne 0) {
        throw "docker compose failed: $output"
    }
    if ($Service -eq 'pulse' -and (Test-StreamcloneHostPulseReady)) {
        [void](Start-StreamcloneHelmPulsePortForward -Root $Root)
    }
    return $output
}

function Start-ProfileServiceUpAsync {
    param(
        [ValidateSet('scraper', 'clipper', 'pulse')]
        [string]$Service
    )

    Set-Location $Root
    if (-not (Test-Path $envPath)) {
        throw 'Missing .env - run scripts/setup.ps1 first.'
    }

    Sync-SetupControlTokenFromEnv

    $lock = Get-StreamcloneComposeLock
    if (Test-StreamcloneComposeLockActive -Lock $lock) {
        if ($lock.service -eq $Service) {
            return "$Service compose is already running (pid $($lock.pid))."
        }
        if ($lock.service -ne $Service) {
            [void](Add-StreamcloneOptionalStartQueue -Service $Service)
            $other = Get-StreamcloneOptionalServiceLabel -Service $lock.service
            $self = Get-StreamcloneOptionalServiceLabel -Service $Service
            return "$self queued - will start automatically after $other finishes (Docker runs one compose step at a time)."
        }
    }

    $scraperSourceBuild = $false
    if ($Service -eq 'scraper') {
        Ensure-ScraperSiblingRepo
        $scraperSourceBuild = Test-ScraperBuildFromSource -Root $Root
    } elseif ($Service -eq 'pulse') {
        Enable-StreamclonePulseEnv -Root $Root
        $script:envValues = Read-EnvKeyValueFile -Path $envPath
        if (Test-StreamcloneHostPulseReady) {
            [void](Start-StreamcloneHelmPulsePortForward -Root $Root)
        }
    }

    $useImages = Get-SetupControlUseImages
    $pullImages = $useImages -or (Test-StreamcloneUseImagesFromRoot -Root $Root)
    $useWslDocker = Test-StreamcloneUseWslDockerCli -Root $Root
    $profileForCompose = if ($Service -eq 'pulse' -and (Test-StreamcloneHostPulseReady)) { 'core' } else { $Service }
    $composeArgs = Get-StreamcloneComposeArgs -Root $Root -Profile $profileForCompose -UseImages:$useImages -RelativePaths:$useWslDocker -ScraperSourceBuild:$scraperSourceBuild
    $docker = Get-EnvDockerExe
    if (-not $docker) {
        throw "Docker is required. Install Docker Desktop and ensure 'docker.exe' is on PATH."
    }

    $logFile = Join-Path $Root ".streamclone-start-$Service.log"
    $errLog = "${logFile}.err"
    foreach ($path in @($logFile, $errLog)) {
        if (Test-Path $path) { Remove-Item -LiteralPath $path -Force -ErrorAction SilentlyContinue }
    }
    $args = if ($Service -eq 'pulse') {
        Get-StreamclonePulseComposeUpArgs -ComposeArgs $composeArgs -PullImages:$pullImages -ScraperSourceBuild:$scraperSourceBuild
    } else {
        $serviceArgs = $composeArgs + @('up', '-d', '--remove-orphans')
        if ($pullImages -and -not $scraperSourceBuild) { $serviceArgs += '--pull', 'missing' }
        if ($scraperSourceBuild) { $serviceArgs += '--build' }
        $serviceArgs += Get-StreamcloneOptionalComposeTargets -Service $Service
        $serviceArgs
    }

    $proc = if ($useWslDocker) {
        $wslRoot = Get-StreamcloneWslRootPath -Root $Root
        if (-not $wslRoot) { throw 'Could not resolve WSL path for Streamclone root.' }
        $bashCmd = "cd $(($wslRoot -replace "'", "'\\''")) && docker $(Join-EnvProcessArguments -Arguments $args)"
        Start-Process -FilePath 'wsl.exe' `
            -ArgumentList @('bash', '-lc', $bashCmd) `
            -WindowStyle Hidden `
            -RedirectStandardOutput $logFile `
            -RedirectStandardError $errLog `
            -PassThru
    } else {
        Start-Process -FilePath $docker `
            -ArgumentList (Join-EnvProcessArguments -Arguments $args) `
            -WorkingDirectory $Root `
            -WindowStyle Hidden `
            -RedirectStandardOutput $logFile `
            -RedirectStandardError $errLog `
            -PassThru
    }

    Set-Content -LiteralPath $composeLockPath -Value (@{
        service = $Service
        pid     = $proc.Id
        started = (Get-Date).ToString('o')
    } | ConvertTo-Json -Compress) -Encoding UTF8

    Start-StreamcloneComposeLockWatcher -Service $Service -ProcessId $proc.Id
    return "compose start initiated (pid $($proc.Id)); see $logFile"
}

function Start-ScraperCamoufoxWarmupAsync {
    $warmupFlag = Join-Path $Root '.streamclone-scraper-warmup.requested'
    if (Test-Path $warmupFlag) {
        return 'Camoufox warmup already queued'
    }
    Set-Content -LiteralPath $warmupFlag -Value (Get-Date).ToString('o') -Encoding UTF8

    $warmupLog = Join-Path $Root '.streamclone-scraper-warmup.log'
    $warmupErr = "${warmupLog}.err"
    foreach ($path in @($warmupLog, $warmupErr)) {
        if (Test-Path $path) { Remove-Item -LiteralPath $path -Force -ErrorAction SilentlyContinue }
    }
    $preflight = Join-Path $Root 'scripts\scraper-preflight.ps1'
    if (-not (Test-Path $preflight)) {
        return 'scraper-preflight.ps1 missing — run scripts/warm-camoufox-profile.ps1 manually if viewer sync fails'
    }
    $psExe = if ($PSVersionTable.PSEdition -eq 'Core') { 'pwsh.exe' } else { 'powershell.exe' }
    Start-Process -FilePath $psExe `
        -ArgumentList @(
            '-NoProfile',
            '-ExecutionPolicy', 'Bypass',
            '-File', $preflight
        ) `
        -WorkingDirectory $Root `
        -WindowStyle Hidden `
        -RedirectStandardOutput $warmupLog `
        -RedirectStandardError $warmupErr `
        | Out-Null
    return 'Camoufox warmup probe started (scraper-preflight). If Cloudflare blocks, run scripts/warm-camoufox-profile.ps1 once.'
}

function Get-StreamcloneProfileStartStatus {
    param(
        [ValidateSet('scraper', 'clipper', 'pulse')]
        [string]$Service
    )

    $logFile = Join-Path $Root ".streamclone-start-$Service.log"
    $errLog = "${logFile}.err"
    $lines = [System.Collections.Generic.List[string]]::new()
    foreach ($path in @($logFile, $errLog)) {
        if (-not (Test-Path $path)) { continue }
        foreach ($line in (Get-Content -LiteralPath $path -Tail 10 -ErrorAction SilentlyContinue)) {
            $trimmed = "$line".Trim()
            if ($trimmed -and -not ($trimmed -match '^[a-f0-9]{12}\s')) {
                [void]$lines.Add($trimmed)
            }
        }
    }

    $detail = if ($lines.Count -gt 0) { $lines[$lines.Count - 1] } else { 'Preparing Docker compose...' }
    $blob = ($lines -join ' ').ToLowerInvariant()
    $percent = 15
    $phase = 'Starting service'

    if (Test-Path $composeQueuePath) {
        foreach ($item in (Read-StreamcloneOptionalStartQueue)) {
            if ($item.service -eq $Service) {
                $percent = 12
                $phase = 'Queued'
                $other = if ($Service -eq 'scraper') { 'Clip Studio' } else { 'Analytics' }
                if ($Service -eq 'pulse') { $other = 'another optional service' }
                $detail = "Waiting for $other to finish - $other compose will run first, then this service starts automatically."
                break
            }
        }
    }

    if ($blob -match 'cloning|clone') {
        $percent = 20
        $phase = 'Downloading scraper repo'
    }
    if ($blob -match '(\d+\.?\d*)\s*mb\s*/\s*(\d+\.?\d*)\s*mb') {
        $doneMb = [double]$Matches[1]
        $totalMb = [double]$Matches[2]
        if ($totalMb -gt 0) {
            $ratio = [math]::Min(1.0, $doneMb / $totalMb)
            $percent = [math]::Max($percent, [int](25 + (40 * $ratio)))
            $phase = 'Building scraper image'
            $detail = ('{0:N0} MB / {1:N0} MB downloaded' -f $doneMb, $totalMb)
        }
    }
    if ($blob -match 'pulling|pull complete|downloading|extracting|pulled') {
        $percent = 45
        $phase = 'Pulling container image'
    }
    if ($blob -match 'created|starting|recreat|container') {
        $percent = 70
        $phase = 'Creating containers'
    }
    if ($blob -match 'error|failed|denied|cannot') {
        $percent = 5
        $phase = 'Docker reported an error'
        $detail = Format-StreamcloneStartDetail -Service $Service -Detail $detail -Blob $blob
    }

    $nameFilter = if ($Service -eq 'scraper') { 'streamclone-scraper' } elseif ($Service -eq 'pulse') { 'streamclone-grafana' } else { 'streamclone-clipper' }
    $ps = Invoke-EnvDockerCaptured -Arguments @('ps', '--filter', "name=$nameFilter", '--format', '{{.Status}}')
    if ($ps.ExitCode -eq 0 -and $ps.Output) {
        $status = ($ps.Output | Select-Object -First 1).Trim()
        if ($status -match 'Up') {
            $percent = [math]::Max($percent, 82)
            $phase = 'Container is up'
            $detail = $status
            if ($Service -eq 'scraper') {
                Start-ScraperCamoufoxWarmupAsync | Out-Null
            }
        }
        if ($status -match 'healthy') {
            $percent = 92
            $phase = 'Waiting for API health'
        }
    }

    if ($Service -eq 'pulse') {
        if (-not (Test-StreamcloneHostPulseReady)) {
            $percent = [math]::Max($percent, 86)
            $phase = 'Waiting for Grafana and InfluxDB'
            $detail = 'Waiting for Grafana on localhost:3000 and InfluxDB on localhost:18086...'
        } else {
            $ts = Get-StreamclonePulseTimeseriesStatus
            if ($null -eq $ts) {
                $percent = [math]::Max($percent, 88)
                $phase = 'Waiting for analytics export'
                $detail = 'Grafana and InfluxDB are up - waiting for Analytics to report Pulse export status.'
            } elseif (($ts.enabled -ne $true) -or ($ts.configured -ne $true)) {
                $percent = [math]::Max($percent, 90)
                $phase = 'Enabling analytics export'
                $detail = 'Analytics is restarting with InfluxDB export enabled.'
            } else {
                $backfillState = [string]$ts.backfillState
                $rollupCount = 0L
                $exportedCount = 0L
                if ($null -ne $ts.backfillRollups) { $rollupCount = [int64]$ts.backfillRollups }
                if ($null -ne $ts.backfillExported) { $exportedCount = [int64]$ts.backfillExported }
                if ($backfillState -eq 'running') {
                    $ratio = if ($rollupCount -gt 0) { [math]::Min(1.0, $exportedCount / [double]$rollupCount) } else { 0.5 }
                    $percent = [math]::Max($percent, [int](90 + (8 * $ratio)))
                    $phase = 'Backfilling analytics'
                    $detail = if ($rollupCount -gt 0) {
                        "Exported $exportedCount / $rollupCount local rollups to InfluxDB."
                    } else {
                        'Scanning local analytics history for Pulse backfill...'
                    }
                } elseif ($backfillState -eq 'failed') {
                    $percent = 5
                    $phase = 'Analytics export failed'
                    $detail = [string]$ts.backfillLastError
                    if ([string]::IsNullOrWhiteSpace($detail)) { $detail = [string]$ts.lastError }
                    if ([string]::IsNullOrWhiteSpace($detail)) { $detail = 'Pulse backfill failed; check analytics container logs.' }
                } elseif (([string]$ts.state -eq 'ready') -and ([string]::IsNullOrWhiteSpace($backfillState) -or $backfillState -eq 'completed')) {
                    $percent = 100
                    $phase = 'Ready'
                    $detail = 'Pulse dashboards are ready - Grafana, InfluxDB, analytics export, and backfill are online.'
                } elseif ([string]$ts.state -eq 'degraded') {
                    $percent = 5
                    $phase = 'Analytics export degraded'
                    $detail = [string]$ts.lastError
                    if ([string]::IsNullOrWhiteSpace($detail)) { $detail = 'InfluxDB export is degraded; check analytics container logs.' }
                } else {
                    $percent = [math]::Max($percent, 92)
                    $phase = 'Waiting for analytics export'
                    $detail = "Timeseries state: $($ts.state); backfill: $backfillState"
                }
            }
        }
    }

    $warmup = $null
    if ($Service -eq 'scraper') {
        $warmupLog = Join-Path $Root '.streamclone-scraper-warmup.log'
        $warmupErr = "${warmupLog}.err"
        $warmupLines = [System.Collections.Generic.List[string]]::new()
        foreach ($path in @($warmupLog, $warmupErr)) {
            if (-not (Test-Path $path)) { continue }
            foreach ($line in (Get-Content -LiteralPath $path -Tail 6 -ErrorAction SilentlyContinue)) {
                $trimmed = "$line".Trim()
                if ($trimmed) { [void]$warmupLines.Add($trimmed) }
            }
        }
        if ($warmupLines.Count -gt 0) {
            $warmupBlob = ($warmupLines -join ' ').ToLowerInvariant()
            if ($warmupBlob -match 'camoufox (sequential )?scrape ok|meta#ecs|chart true') {
                $warmup = 'Camoufox profile warm — TwitchTracker probe ok'
                if ($percent -lt 96) { $percent = 96 }
                if ($phase -eq 'Container is up' -or $phase -eq 'Waiting for API health') {
                    $phase = 'Warming Camoufox profile'
                }
            } elseif ($warmupBlob -match 'cloudflare|warm the camoufox profile') {
                $warmup = 'Cloudflare blocked — run scripts/warm-camoufox-profile.ps1 once, then retry sync'
            } elseif ($warmupBlob -match 'not running|waiting for scraper|scraper healthy') {
                $warmup = 'Waiting for Analytics container, then probing Camoufox / TwitchTracker…'
                if ($percent -ge 82 -and $percent -lt 94) {
                    $percent = 94
                    $phase = 'Warming Camoufox profile'
                }
            } elseif ($warmupBlob -match 'writeerror|fullyqualifiederrorid') {
                $warmup = Format-StreamcloneStartDetail -Service 'scraper' -Detail ($warmupLines[$warmupLines.Count - 1]) -Blob $warmupBlob
            } else {
                $warmup = Format-StreamcloneStartDetail -Service 'scraper' -Detail ($warmupLines[$warmupLines.Count - 1]) -Blob $warmupBlob
            }
        } elseif ($percent -ge 82) {
            $warmup = 'Camoufox warmup queued after container start'
        }
    }

    return @{
        service = $Service
        percent = $percent
        phase   = $phase
        detail  = $detail
        lines   = @($lines.ToArray() | Select-Object -Last 5)
        warmup  = $warmup
    }
}

function Invoke-SyncClipperAuth {
    Set-Location $Root
    if (-not (Test-Path $envPath)) {
        throw 'Missing .env - run scripts/setup.ps1 first.'
    }
    if (-not (Sync-ClipperAuthFromRuntime -Root $Root -EnvFile $envPath)) {
        return @{ ok = $true; merged = $false; message = 'no runtime clipper auth file yet' }
    }

    $script:envValues = Read-EnvKeyValueFile -Path $envPath
    $useImages = ($envValues['STREAMCLONE_USE_IMAGES'] -eq '1') -or (-not [string]::IsNullOrWhiteSpace($envValues['IMAGE_TAG']))
    $profile = [string]$envValues['STREAMCLONE_PROFILE']
    if ([string]::IsNullOrWhiteSpace($profile)) { $profile = 'core' }

    $clipperRunning = $false
    $prev = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        $psResult = Invoke-EnvDockerCaptured -Arguments @('ps', '--filter', 'name=streamclone-clipper', '--format', '{{.Names}}')
        if ($psResult.ExitCode -eq 0 -and $psResult.Output) {
            $clipperRunning = $true
        }
    } finally {
        $ErrorActionPreference = $prev
    }

    $log = ''
    if ($clipperRunning -or $profile -in @('clipper', 'full')) {
        $composeArgs = Get-StreamcloneComposeArgs -Root $Root -Profile $profile -UseImages:$useImages
        $result = Invoke-EnvDockerCaptured -Arguments ($composeArgs + @('up', '-d', '--no-deps', '--force-recreate', 'clipper'))
        $log = ($result.Output -join [Environment]::NewLine).Trim()
        if ($result.ExitCode -ne 0) {
            throw "clipper recreate failed: $log"
        }
    }

    Invoke-EnsureFrontendClipperConfig -EnvFile $envPath | Out-Null

    return @{
        ok = $true
        merged = $true
        recreated = ($clipperRunning -or $profile -in @('clipper', 'full'))
        message = 'clipper credentials merged from sign-in'
        log = $log
    }
}

Set-Content -Path $PidFile -Value $PID -NoNewline

function Ensure-SetupControlUrlAcl {
    param([int]$ListenerPort = 9191)
    $url = "http://+:$ListenerPort/"
    $show = & netsh http show urlacl url=$url 2>&1 | Out-String
    if ($show -match [regex]::Escape($url)) { return $true }
    $user = if ($env:USERNAME) { "$env:USERDOMAIN\$env:USERNAME" } else { 'Everyone' }
    $null = & netsh http add urlacl url=$url user=$user 2>&1
    $show = & netsh http show urlacl url=$url 2>&1 | Out-String
    return ($show -match [regex]::Escape($url))
}

function Start-SetupControlHttpListener {
    param([int]$ListenerPort = 9191)
    $wildcard = "http://+:$ListenerPort/"
    if (Ensure-SetupControlUrlAcl -ListenerPort $ListenerPort) {
        $listener = [System.Net.HttpListener]::new()
        $listener.Prefixes.Add($wildcard)
        try {
            $listener.Start()
            return $listener
        } catch {
            try { $listener.Close() } catch { }
        }
    }

    $listener = [System.Net.HttpListener]::new()
    $prefixes = [System.Collections.Generic.List[string]]::new()
    $prefixes.Add("http://127.0.0.1:$ListenerPort/")
    $prefixes.Add("http://[::1]:$ListenerPort/")
    foreach ($prefix in $prefixes) {
        if (-not $listener.Prefixes.Contains($prefix)) {
            $listener.Prefixes.Add($prefix)
        }
    }
    try {
        $listener.Start()
    } catch {
        throw
    }
    return $listener
}

$listener = Start-SetupControlHttpListener -ListenerPort $Port

try {
    while ($listener.IsListening) {
        $context = $listener.GetContext()
        $request = $context.Request
        $response = $context.Response
        $path = ($request.Url.AbsolutePath -replace '/+$', '')
        if ([string]::IsNullOrWhiteSpace($path)) { $path = '/' }

        if ($request.HttpMethod -eq 'OPTIONS') {
            $response.Headers.Add('Access-Control-Allow-Origin', '*')
            $response.Headers.Add('Access-Control-Allow-Methods', 'GET, POST, OPTIONS')
            $response.Headers.Add('Access-Control-Allow-Headers', 'Content-Type, X-Streamclone-Setup-Token')
            $response.StatusCode = 204
            $response.Close()
            continue
        }

        try {
            if ($request.HttpMethod -eq 'GET' -and ($path -eq '/health' -or $path -eq '/')) {
                Write-JsonResponse -Response $response -StatusCode 200 -Body @{
                    ok = $true
                    service = 'setup-control'
                    root = $Root
                }
                continue
            }

            if ($request.HttpMethod -eq 'GET' -and $path -eq '/endpoints') {
                Write-JsonResponse -Response $response -StatusCode 200 -Body @{
                    ok = $true
                    service = 'setup-control'
                    endpoints = @(
                        @{ method = 'GET'; path = '/health'; auth = $false; description = 'Daemon health probe' }
                        @{ method = 'GET'; path = '/diagnostics'; auth = $false; description = 'Host + compose diagnostics for the directory UI' }
                        @{ method = 'GET'; path = '/diagnostics/network'; auth = $false; description = 'Docker network I/O stats for Streamclone containers' }
                        @{ method = 'GET'; path = '/endpoints'; auth = $false; description = 'This route list' }
                        @{ method = 'GET'; path = '/start/scraper/status'; auth = $false; description = 'Async scraper profile start progress' }
                        @{ method = 'GET'; path = '/start/clipper/status'; auth = $false; description = 'Async clipper profile start progress' }
                        @{ method = 'GET'; path = '/start/pulse/status'; auth = $false; description = 'Async Pulse service start progress' }
                        @{ method = 'POST'; path = '/start/scraper'; auth = $true; description = 'Start analytics scraper compose profile' }
                        # Deprecated: prefer external ReplayForge (../replayforge). POST /start/clipper starts legacy in-repo clipper profile only.
                        @{ method = 'POST'; path = '/start/clipper'; auth = $true; description = 'Start clipper compose profile (deprecated — use ReplayForge)' }
                        @{ method = 'POST'; path = '/start/pulse'; auth = $true; description = 'Start Pulse Grafana/Influx services' }
                        @{ method = 'POST'; path = '/stop/scraper'; auth = $true; description = 'Stop analytics scraper compose profile' }
                        @{ method = 'POST'; path = '/stop/clipper'; auth = $true; description = 'Stop clipper compose profile' }
                        @{ method = 'POST'; path = '/stop/pulse'; auth = $true; description = 'Stop Pulse Grafana/Influx services' }
                        @{ method = 'POST'; path = '/sync-clipper-auth'; auth = $true; description = 'Merge signed-in Twitch tokens into clipper env' }
                    )
                    scripts = @(
                        @{ name = 'backup-streamclone.ps1'; path = 'scripts/backup-streamclone.ps1'; description = 'Dump Postgres + MinIO backup instructions' }
                    )
                }
                continue
            }

            if ($request.HttpMethod -eq 'GET' -and $path -eq '/diagnostics') {
                $report = Get-StreamcloneDiagnostics -Root $Root
                Write-JsonResponse -Response $response -StatusCode 200 -Body $report
                continue
            }

            if ($request.HttpMethod -eq 'GET' -and $path -eq '/diagnostics/network') {
                $report = Get-StreamcloneNetworkStats
                Write-JsonResponse -Response $response -StatusCode 200 -Body $report
                continue
            }

            if ($request.HttpMethod -eq 'GET' -and $path -match '^/start/(scraper|clipper|pulse)/status$') {
                $service = $Matches[1]
                $status = Get-StreamcloneProfileStartStatus -Service $service
                Write-JsonResponse -Response $response -StatusCode 200 -Body @{
                    ok      = $true
                    service = $status.service
                    percent = $status.percent
                    phase   = $status.phase
                    detail  = $status.detail
                    lines   = $status.lines
                    warmup  = $status.warmup
                }
                continue
            }

            if ($request.HttpMethod -eq 'POST' -and $path -match '^/start/(scraper|clipper|pulse)$') {
                if (-not (Test-SetupControlAuthorized -Request $request)) {
                    Write-JsonResponse -Response $response -StatusCode 401 -Body @{ ok = $false; error = 'unauthorized' }
                    continue
                }
                $service = $Matches[1]
                $log = Start-ProfileServiceUpAsync -Service $service
                $warmup = if ($service -eq 'scraper') {
                    'Camoufox warmup probe started in background (scraper-preflight). Manual CF pass: scripts/warm-camoufox-profile.ps1'
                } else { $null }
                Write-JsonResponse -Response $response -StatusCode 200 -Body @{
                    ok      = $true
                    service = $service
                    message = 'starting'
                    log     = $log
                    warmup  = $warmup
                }
                continue
            }

            if ($request.HttpMethod -eq 'POST' -and $path -match '^/stop/(scraper|clipper|pulse)$') {
                if (-not (Test-SetupControlAuthorized -Request $request)) {
                    Write-JsonResponse -Response $response -StatusCode 401 -Body @{ ok = $false; error = 'unauthorized' }
                    continue
                }
                $service = $Matches[1]
                try {
                    $log = Invoke-ProfileServiceStop -Service $service
                    Write-JsonResponse -Response $response -StatusCode 200 -Body @{
                        ok      = $true
                        service = $service
                        message = 'stopped'
                        log     = $log
                    }
                } catch {
                    Write-JsonResponse -Response $response -StatusCode 500 -Body @{ ok = $false; error = $_.Exception.Message }
                }
                continue
            }

            if ($request.HttpMethod -eq 'POST' -and $path -eq '/sync-clipper-auth') {
                if (-not (Test-SetupControlAuthorized -Request $request)) {
                    Write-JsonResponse -Response $response -StatusCode 401 -Body @{ ok = $false; error = 'unauthorized' }
                    continue
                }
                $result = Invoke-SyncClipperAuth
                Write-JsonResponse -Response $response -StatusCode 200 -Body $result
                continue
            }

            Write-JsonResponse -Response $response -StatusCode 404 -Body @{ ok = $false; error = 'not_found' }
        } catch {
            Write-JsonResponse -Response $response -StatusCode 500 -Body @{ ok = $false; error = $_.Exception.Message }
        }
    }
} finally {
    if (Test-Path $PidFile) { Remove-Item $PidFile -Force -ErrorAction SilentlyContinue }
    $listener.Stop()
    $listener.Close()
}
