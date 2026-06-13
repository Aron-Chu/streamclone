#Requires -Version 5.1
# Helpers for visible install progress (pull + container status).
# Test-StreamcloneDockerPullDisplayLine is defined in env.ps1 (dot-sourced below).

. (Join-Path $PSScriptRoot 'env.ps1')

function Test-StreamcloneUseImagesFromRoot {
    param([string]$Root)
    if (Test-Path (Join-Path $Root 'VERSION')) { return $true }
    if ($env:STREAMCLONE_USE_IMAGES -eq '1') { return $true }
    $envPath = Join-Path $Root '.env'
    if (-not (Test-Path $envPath)) { return $false }
    $vals = Read-EnvKeyValueFile -Path $envPath
    if ($vals['STREAMCLONE_USE_IMAGES'] -eq '1') { return $true }
    if (-not [string]::IsNullOrWhiteSpace($vals['IMAGE_TAG'])) { return $true }
    return $false
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

function Get-StreamcloneComposeArgs {
    param(
        [string]$Root,
        [string]$Profile = '',
        [switch]$UseImages,
        [switch]$NoUseImages
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
    $args = @(
        'compose', '--env-file', (Join-Path $Root '.env'),
        '-f', (Join-Path $Root 'deploy\docker-compose.yml'),
        '-f', (Join-Path $Root 'deploy\docker-compose.local-tunnel.yml')
    )
    if ($pullImages -or $scraperImages) {
        $args += '-f', (Join-Path $Root 'deploy\docker-compose.release.yml')
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
