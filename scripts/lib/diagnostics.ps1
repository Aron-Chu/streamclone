#Requires -Version 5.1
# Shared host diagnostics for check-streamclone.ps1 and setup-control GET /diagnostics.

. (Join-Path $PSScriptRoot 'env.ps1')
. (Join-Path $PSScriptRoot 'stack-progress.ps1')
. (Join-Path $PSScriptRoot 'install-upgrade.ps1')

function Test-StreamcloneHostServiceHealth {
    param([string]$Url)
    try {
        $resp = Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 3
        return ($resp.StatusCode -ge 200 -and $resp.StatusCode -lt 300)
    } catch {
        return $false
    }
}

function Test-StreamcloneSetupControlHealth {
    param([int]$HealthPort = 9191)
    try {
        $resp = Invoke-WebRequest -Uri "http://127.0.0.1:$HealthPort/health" -UseBasicParsing -TimeoutSec 2
        if ($resp.StatusCode -eq 200) {
            $body = $resp.Content | ConvertFrom-Json
            return [bool]$body.ok
        }
    } catch { }
    return $false
}

function Get-StreamcloneStartLogTail {
    param(
        [string]$Root,
        [string]$Service,
        [int]$Lines = 20
    )
    $logFile = Join-Path $Root ".streamclone-start-$Service.log"
    if (-not (Test-Path $logFile)) { return '' }
    try {
        return ((Get-Content $logFile -Tail $Lines -ErrorAction Stop) -join [Environment]::NewLine)
    } catch {
        return ''
    }
}

function Get-StreamcloneDiagnostics {
    param(
        [string]$Root,
        [string]$WebUrl = ''
    )
    if ([string]::IsNullOrWhiteSpace($WebUrl)) {
        $WebUrl = (Get-StreamcloneAppUrl) + '/'
    }

    $suggestions = [System.Collections.Generic.List[string]]::new()
    $envFile = Join-Path $Root '.env'
    $envValues = if (Test-Path $envFile) { Read-EnvKeyValueFile -Path $envFile } else { @{} }
    $profile = Get-StreamcloneProfileFromRoot -Root $Root
    $versions = Get-StreamcloneInstallVersions -Root $Root -FetchLatest
    $imageTag = $versions.imageTag
    if ([string]::IsNullOrWhiteSpace($imageTag)) {
        $imageTag = $versions.bundleVersion
    }
    $upgradeNeeded = Test-StreamcloneUpgradeNeeded -Root $Root

    $configReady = Test-Path $envFile
    if (-not $configReady) {
        [void]$suggestions.Add('Run setup: powershell -File scripts\setup.ps1 -Profile core -NonInteractive -UseImages')
    }

    $docker = Get-EnvDockerExe
    $dockerState = 'unknown'
    if (-not $docker) {
        $dockerState = 'missing'
        [void]$suggestions.Add('Install Docker Desktop and ensure it is running.')
    } else {
        $info = Invoke-EnvDockerCapturedWithTimeout -Arguments @('info') -TimeoutSec 15
        if ($info.TimedOut) {
            $dockerState = 'timeout'
            [void]$suggestions.Add('Start Docker Desktop and wait until it shows Running.')
        } elseif ($info.ExitCode -ne 0) {
            $dockerState = 'stopped'
            [void]$suggestions.Add('Open Docker Desktop and wait until the whale icon is steady.')
        } else {
            $dockerState = 'running'
        }
    }

    $coreImages = $null
    if ($dockerState -eq 'running' -and $imageTag) {
        $status = Get-StreamcloneCoreImageStatus -Root $Root -Tag $imageTag
        $coreImages = @{
            present = $status.present
            total   = $status.total
            missing = @($status.missing)
            tag     = $imageTag
        }
    }

    if ($upgradeNeeded) {
        [void]$suggestions.Add('Run Manage Streamclone -> Update to sync Docker images with the bundle version.')
    }
    if ($coreImages -and $coreImages.present -lt $coreImages.total) {
        [void]$suggestions.Add("$($coreImages.present)/$($coreImages.total) core images downloaded - run Start Streamclone to resume.")
    }

    $containers = @()
    if ($dockerState -eq 'running') {
        $ps = Invoke-EnvDockerCaptured -Arguments @('ps', '-a', '--filter', 'name=streamclone', '--format', '{{.Names}}|{{.Status}}')
        if ($ps.ExitCode -eq 0 -and $ps.Output) {
            foreach ($line in $ps.Output) {
                $parts = $line -split '\|', 2
                $containers += [pscustomobject]@{
                    name   = ($parts[0] -replace '^streamclone-', '')
                    status = $parts[1]
                }
            }
            $running = @($containers | Where-Object { $_.status -match '^Up' })
            if ($running.Count -eq 0) {
                [void]$suggestions.Add('Use Start Streamclone.cmd or scripts\start-streamclone.ps1')
            }
        } else {
            [void]$suggestions.Add('Run Install Streamclone.cmd or Start Streamclone.cmd.')
        }
    }

    $webOk = $false
    try {
        $resp = Invoke-WebRequest -Uri $WebUrl -UseBasicParsing -TimeoutSec 5
        $webOk = ($resp.StatusCode -ge 200 -and $resp.StatusCode -lt 500)
    } catch {
        if ($dockerState -eq 'running' -and $containers.Count -gt 0) {
            [void]$suggestions.Add('Containers exist but UI is not up yet - wait 1-2 min or check: docker compose logs caddy frontend')
        }
    }

    if (-not $webOk) {
        $localhostOk = Test-StreamcloneWebReachable -Url 'http://localhost:8090/' -TimeoutSec 3
        $loopbackOk = Test-StreamcloneWebReachable -Url (Get-StreamcloneAppUrl) -TimeoutSec 5
        if (-not $localhostOk -and $loopbackOk) {
            $webOk = $true
            $WebUrl = (Get-StreamcloneAppUrl) + '/'
            [void]$suggestions.Add('Use http://127.0.0.1:8090/ — localhost is broken on this PC (WSL port relay on [::1]:8090).')
        }
    }

    $setupControl = Test-StreamcloneSetupControlHealth
    if (-not $setupControl) {
        [void]$suggestions.Add('Run Start Streamclone.cmd once, or: powershell -File scripts\ensure-setup-control.ps1')
    }

    $scraperReady = Test-StreamcloneHostServiceHealth -Url 'http://localhost:8000/health'
    $clipperReady = $false
    foreach ($clipperUrl in @((Get-StreamcloneAppUrl '/v1/clipper/health'), 'http://127.0.0.1:8095/health')) {
        if (Test-StreamcloneHostServiceHealth -Url $clipperUrl) {
            $clipperReady = $true
            break
        }
    }

    $sibling = Get-EnvScraperSiblingPath
    $siblingPresent = (Test-Path (Join-Path $sibling '.git')) -or (Test-Path (Join-Path $sibling 'Dockerfile'))
    if (-not $siblingPresent) {
        [void]$suggestions.Add('Clone streamclone-scraper sibling repo for Analytics charts.')
    }

    $healthy = ($dockerState -eq 'running') -and $webOk -and ($configReady -or ($containers.Count -gt 0))

    return [ordered]@{
        ok               = $true
        healthy          = $healthy
        profile          = $profile
        bundleVersion    = $versions.bundleVersion
        imageTag         = $imageTag
        latestRelease    = $versions.latestRelease
        upgradeNeeded    = $upgradeNeeded
        coreImages       = $coreImages
        docker           = $dockerState
        configReady      = $configReady
        webOk            = $webOk
        webUrl           = $WebUrl
        setupControl     = $setupControl
        containers       = @($containers)
        optionalServices = @{
            scraper = if ($scraperReady) { 'ready' } else { 'offline' }
            clipper = if ($clipperReady) { 'ready' } else { 'offline' }
        }
        scraperSibling   = @{
            path    = $sibling
            present = $siblingPresent
        }
        recentStartLogs  = @{
            scraper = Get-StreamcloneStartLogTail -Root $Root -Service 'scraper'
            clipper = Get-StreamcloneStartLogTail -Root $Root -Service 'clipper'
        }
        suggestions      = @($suggestions)
    }
}
