#Requires -Version 5.1
# Helpers for visible install progress (pull + container status).
# Test-StreamcloneDockerPullDisplayLine is defined in env.ps1 (dot-sourced below).

. (Join-Path $PSScriptRoot 'env.ps1')

function Test-StreamcloneUseImagesFromRoot {
    param([string]$Root)
  # Release installs set STREAMCLONE_USE_IMAGES=1 / IMAGE_TAG in .env (see release-bundle.env).
  # Git checkouts also ship VERSION for tagging — that alone must not enable GHCR pulls.
    if ($env:STREAMCLONE_USE_IMAGES -eq '1') { return $true }
    $envPath = Join-Path $Root '.env'
    if (-not (Test-Path $envPath)) { return $false }
    $vals = Read-EnvKeyValueFile -Path $envPath
    if ($vals['STREAMCLONE_USE_IMAGES'] -eq '1') { return $true }
    if (-not [string]::IsNullOrWhiteSpace($vals['IMAGE_TAG'])) { return $true }
    return $false
}

function Test-StreamcloneLocalTunnelHlsCaddyConfig {
    param([string]$Root = '')
    if ([string]::IsNullOrWhiteSpace($Root)) { $Root = Get-EnvRepoRoot }
    $caddyFile = Join-Path $Root 'deploy\Caddyfile.local-tunnel'
    if (-not (Test-Path -LiteralPath $caddyFile)) { return $true }
    $item = Get-Item -LiteralPath $caddyFile -Force
    if ($item.PSIsContainer) { return $false }
    $content = Get-Content $caddyFile -Raw
    if ($content -match '@hls_local') { return $false }
    if ($content -notmatch 'header_up Authorization "Bearer streamclone-local-hls-cdn"') { return $false }
    if ($content -notmatch '@hls[\s\S]*path /live/\*') { return $false }
    return $true
}

function Get-StreamcloneProfileFromRoot {
    param([string]$Root, [string]$Default = 'core')
    $profileFile = Join-Path $Root '.streamclone-profile'
    if (Test-Path $profileFile) {
        $fromFile = (Get-Content $profileFile -Raw).Trim()
        if ($fromFile) { return $fromFile }
    }
    $envPath = Join-Path $Root '.env'
    if (Test-Path $envPath) {
        $vals = Read-EnvKeyValueFile -Path $envPath
        if (-not [string]::IsNullOrWhiteSpace($vals['STREAMCLONE_PROFILE'])) {
            return $vals['STREAMCLONE_PROFILE']
        }
    }
    return $Default
}

function Test-StreamcloneScraperUseImagesFromRoot {
    param([string]$Root)
    $envPath = Join-Path $Root '.env'
    if (-not (Test-Path $envPath)) { return $false }
    $vals = Read-EnvKeyValueFile -Path $envPath
    return ($vals['SCRAPER_USE_IMAGES'] -eq '1')
}

function Test-ScraperSiblingRepoReady {
    param([string]$Root = (Get-EnvRepoRoot))
    $sibling = Get-EnvScraperSiblingPath
    return (Test-Path (Join-Path $sibling '.git')) -or (Test-Path (Join-Path $sibling 'Dockerfile'))
}

function Test-ScraperBuildFromSource {
    param([string]$Root)
    if (-not (Test-StreamcloneScraperUseImagesFromRoot -Root $Root)) {
        return $true
    }
    if (Test-ScraperSiblingRepoReady -Root $Root) {
        return $true
    }
    $errLog = Join-Path $Root '.streamclone-start-scraper.log.err'
    if (Test-Path $errLog) {
        $text = Get-Content -LiteralPath $errLog -Raw -ErrorAction SilentlyContinue
        if ($text -match 'registry:\s*denied|error from registry:\s*denied') {
            return $true
        }
    }
    return $false
}

function Get-StreamcloneComposeArgs {
    param(
        [string]$Root,
        [string]$Profile = '',
        [switch]$UseImages,
        [switch]$NoUseImages,
        [switch]$ScraperSourceBuild,
        [switch]$RelativePaths
    )
    if ([string]::IsNullOrWhiteSpace($Profile)) {
        $Profile = Get-StreamcloneProfileFromRoot -Root $Root
    }
    $pullImages = $UseImages.IsPresent
    if (-not $UseImages.IsPresent -and -not $NoUseImages.IsPresent) {
        $pullImages = Test-StreamcloneUseImagesFromRoot -Root $Root
    } elseif ($NoUseImages.IsPresent) {
        $pullImages = $false
    }
    $scraperImages = Test-StreamcloneScraperUseImagesFromRoot -Root $Root
    if ($RelativePaths.IsPresent) {
        $args = @(
            'compose', '--env-file', '.env',
            '-f', 'deploy/docker-compose.yml',
            '-f', 'deploy/docker-compose.local-tunnel.yml'
        )
        if ($pullImages -or $scraperImages) {
            $args += '-f', 'deploy/docker-compose.release.yml'
        }
        if ($ScraperSourceBuild.IsPresent -or (Test-ScraperBuildFromSource -Root $Root)) {
            $args += '-f', 'deploy/docker-compose.scraper-source.yml'
        }
    } else {
        $args = @(
            'compose', '--env-file', (Join-Path $Root '.env'),
            '-f', (Join-Path $Root 'deploy\docker-compose.yml'),
            '-f', (Join-Path $Root 'deploy\docker-compose.local-tunnel.yml')
        )
        if ($pullImages -or $scraperImages) {
            $args += '-f', (Join-Path $Root 'deploy\docker-compose.release.yml')
        }
        if ($ScraperSourceBuild.IsPresent -or (Test-ScraperBuildFromSource -Root $Root)) {
            $args += '-f', (Join-Path $Root 'deploy\docker-compose.scraper-source.yml')
        }
    }
    foreach ($p in (Get-EnvComposeProfiles -Profile $Profile)) {
        $args += '--profile', $p
    }
    return $args
}

function Get-StreamcloneRunningContainerName {
    param(
        [string]$Root,
        [string]$Service
    )
    $composeArgs = Get-StreamcloneComposeArgs -Root $Root
    $result = Invoke-EnvDockerCaptured -Arguments ($composeArgs + @('ps', '-q', $Service))
    if ($result.ExitCode -ne 0 -or -not $result.Output) { return $null }
    $id = ($result.Output | Where-Object { -not [string]::IsNullOrWhiteSpace($_) } | Select-Object -First 1).Trim()
    if (-not $id) { return $null }
    $nameResult = Invoke-EnvDockerCaptured -Arguments @('inspect', '-f', '{{.Name}}', $id)
    if ($nameResult.ExitCode -ne 0 -or -not $nameResult.Output) { return $null }
    return ($nameResult.Output | Select-Object -First 1).Trim().TrimStart('/')
}

function Write-StreamcloneProgressHeader {
    param([string]$Title)
    Write-Host ''
    Write-Host $Title -ForegroundColor Cyan
    Write-Host ('-' * $Title.Length) -ForegroundColor DarkGray
}

function Show-StreamcloneContainerStatus {
    $prev = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        $result = Invoke-EnvDockerCaptured -Arguments @('ps', '-a', '--filter', 'name=streamclone', '--format', '{{.Names}}|{{.Status}}')
        if ($result.ExitCode -ne 0) {
            Write-Host '  (docker status unavailable)' -ForegroundColor DarkGray
            return
        }
        $lines = $result.Output
        if (-not $lines) {
            Write-Host '  (no streamclone containers yet)' -ForegroundColor DarkGray
            return
        }
        foreach ($line in $lines) {
            $parts = $line -split '\|', 2
            $name = ($parts[0] -replace '^streamclone-', '')
            $status = $parts[1]
            $color = 'Yellow'
            if ($status -match 'healthy|Up \d') { $color = 'Green' }
            elseif ($status -match 'Exited \(0\)') { $color = 'DarkGray' }
            elseif ($status -match 'Exit|unhealthy|restarting') { $color = 'Red' }
            Write-Host "  $name : $status" -ForegroundColor $color
        }
    } finally {
        $ErrorActionPreference = $prev
    }
}

function Wait-StreamcloneStackReady {
    param(
        [string]$Url = '',
        [int]$TimeoutSec = 300,
        [int]$IntervalSec = 3,
        [switch]$ShowContainers
    )
    if ([string]::IsNullOrWhiteSpace($Url)) {
        $Url = (Get-StreamcloneAppUrl) + '/'
    }
    $deadline = (Get-Date).AddSeconds($TimeoutSec)
    $attempt = 0
    Write-StreamcloneProgressHeader 'Waiting for services to become healthy'
    while ((Get-Date) -lt $deadline) {
        $attempt++
        if ($ShowContainers) {
            Write-Host "[$attempt] Container status:" -ForegroundColor DarkCyan
            Show-StreamcloneContainerStatus
        }
        try {
            $resp = Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 5
            if ($resp.StatusCode -ge 200 -and $resp.StatusCode -lt 500) {
                Write-Host ''
                Write-Host "Streamclone is ready at $Url" -ForegroundColor Green
                return $true
            }
        } catch {
            if (-not $ShowContainers -and ($attempt % 5 -eq 0)) {
                Write-Host "  still starting... (attempt $attempt)" -ForegroundColor DarkGray
            }
        }
        Start-Sleep -Seconds $IntervalSec
    }
    Write-Host ''
    Write-Host 'Streamclone did not become ready in time.' -ForegroundColor Red
    Show-StreamcloneContainerStatus
    return $false
}

function Test-StreamcloneHostServiceHealth {
    param([string]$Url)
    try {
        $resp = Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 3
        return ($resp.StatusCode -ge 200 -and $resp.StatusCode -lt 300)
    } catch {
        return $false
    }
}

function Test-StreamcloneHostPulseReady {
    return (
        (Test-StreamcloneHostServiceHealth -Url 'http://localhost:3000/api/health') -and
        (Test-StreamcloneHostServiceHealth -Url 'http://localhost:18086/health')
    )
}

function Enable-StreamclonePulseEnv {
    param([string]$Root)
    $envPath = Join-Path $Root '.env'
    $pulseEnv = Join-Path $Root 'deploy\env\profile-pulse.env'
    $defaults = if (Test-Path $pulseEnv) { Read-EnvKeyValueFile -Path $pulseEnv } else { @{} }
    $current = if (Test-Path $envPath) { Read-EnvKeyValueFile -Path $envPath } else { @{} }
    if (Test-StreamcloneHostPulseReady) {
        Set-EnvFileValue -Path $envPath -Key 'TIMESERIES_ENABLED' -Value 'true'
        Set-EnvFileValue -Path $envPath -Key 'TIMESERIES_BACKEND' -Value 'influxdb'
        Set-EnvFileValue -Path $envPath -Key 'INFLUXDB_URL' -Value 'http://host.docker.internal:18086'
        foreach ($key in @('INFLUXDB_ORG', 'INFLUXDB_BUCKET', 'TIMESERIES_WRITE_TIMEOUT_MS', 'TIMESERIES_QUEUE_SIZE')) {
            $value = [string]$current[$key]
            if ([string]::IsNullOrWhiteSpace($value)) { $value = [string]$defaults[$key] }
            if (-not [string]::IsNullOrWhiteSpace($value)) {
                Set-EnvFileValue -Path $envPath -Key $key -Value $value
            }
        }
        $token = [string]$current['INFLUXDB_TOKEN']
        if ([string]::IsNullOrWhiteSpace($token)) { $token = [string]$defaults['INFLUXDB_TOKEN'] }
        if (-not [string]::IsNullOrWhiteSpace($token)) {
            Set-EnvFileValue -Path $envPath -Key 'INFLUXDB_TOKEN' -Value $token
        }
        return
    }
    foreach ($key in @(
        'TIMESERIES_ENABLED',
        'TIMESERIES_BACKEND',
        'INFLUXDB_URL',
        'INFLUXDB_ORG',
        'INFLUXDB_BUCKET',
        'TIMESERIES_WRITE_TIMEOUT_MS',
        'TIMESERIES_QUEUE_SIZE',
        'INFLUXDB_INIT_USERNAME',
        'INFLUXDB_INIT_PASSWORD',
        'GRAFANA_ADMIN_USER',
        'GRAFANA_ADMIN_PASSWORD'
    )) {
        $value = [string]$defaults[$key]
        if (-not [string]::IsNullOrWhiteSpace($value)) {
            Set-EnvFileValue -Path $envPath -Key $key -Value $value
        }
    }
    $token = [string]$current['INFLUXDB_TOKEN']
    if ([string]::IsNullOrWhiteSpace($token)) { $token = [string]$defaults['INFLUXDB_TOKEN'] }
    if ([string]::IsNullOrWhiteSpace($token)) { $token = 'local-pulse-token' }
    Set-EnvFileValue -Path $envPath -Key 'INFLUXDB_TOKEN' -Value $token
}

function Get-StreamclonePulseComposeUpArgs {
    param(
        [string[]]$ComposeArgs,
        [bool]$PullImages,
        [bool]$ScraperSourceBuild
    )
    $helmPulse = Test-StreamcloneHostPulseReady
    $args = @($ComposeArgs) + @('up', '-d', '--remove-orphans', '--no-deps')
    if ($PullImages -and -not $ScraperSourceBuild) { $args += '--pull', 'missing' }
    if ($ScraperSourceBuild -or -not $PullImages) { $args += '--build' }
    $args += '--force-recreate'
    if ($helmPulse) {
        $args += 'analytics'
    } else {
        $args += 'influxdb', 'grafana', 'analytics'
    }
    return $args
}
