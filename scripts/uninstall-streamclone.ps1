#Requires -Version 5.1
param(
    [string]$InstallDir = '',
    [switch]$NonInteractive,
    [switch]$PruneImages,
    [switch]$PruneBaseImages,
    [switch]$SkipImagePrompt,
    [switch]$KeepInstallDir,
    [switch]$KeepVolumes,
    [string]$ProgressFile = ''
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'lib\env.ps1')
. (Join-Path $PSScriptRoot 'lib\install-upgrade.ps1')

function Set-UninstallProgress {
    param(
        [string]$Title,
        [string]$Detail = '',
        [string]$Status = 'running'
    )
    if (-not $ProgressFile) { return }
    @(
        "TITLE=$Title"
        "DETAIL=$Detail"
        "STATUS=$Status"
    ) | Set-Content -LiteralPath $ProgressFile -Encoding UTF8
}

function Complete-UninstallProgress {
    param([int]$ExitCode = 0)
    Set-UninstallProgress -Title 'Uninstall complete' -Detail '' -Status "done|$ExitCode"
}

function Get-StreamcloneInstallRoot {
    param([string]$Hint)
    $candidates = @()
    if ($Hint) { $candidates += $Hint.TrimEnd('\', '/') }
    $candidates += (Join-Path $env:USERPROFILE 'streamclone')
    $scriptRoot = Split-Path -Parent $PSScriptRoot
    $candidates += $scriptRoot
    foreach ($candidate in $candidates) {
        if (Test-Path (Join-Path $candidate 'scripts\start-streamclone.ps1')) {
            return (Resolve-Path -LiteralPath $candidate).Path
        }
    }
    throw 'Streamclone install not found. Pass -InstallDir or run from an install folder.'
}

function Test-StreamcloneUseReleaseImages {
    param([string]$Root)
    if (Test-Path (Join-Path $Root 'VERSION')) { return $true }
    if (Test-Path (Join-Path $Root 'deploy\env\release-bundle.env')) { return $true }
    $envFile = Join-Path $Root '.env'
    if (Test-Path $envFile) {
        $vals = Read-EnvKeyValueFile -Path $envFile
        if ($vals['STREAMCLONE_USE_IMAGES'] -eq '1') { return $true }
    }
    return $false
}

function Stop-StreamcloneControlProcess {
    param([string]$Root)
    $controlPidFile = Join-Path $Root '.streamclone-setup-control.pid'
    if (-not (Test-Path $controlPidFile)) { return }
    $controlPid = (Get-Content $controlPidFile -Raw).Trim()
    if ($controlPid -match '^\d+$') {
        Stop-Process -Id ([int]$controlPid) -Force -ErrorAction SilentlyContinue
    }
    Remove-Item $controlPidFile -Force -ErrorAction SilentlyContinue
}

function Invoke-StreamcloneComposeDown {
    param(
        [string]$Root,
        [switch]$Volumes
    )
    $envFile = Join-Path $Root '.env'
    if (-not (Test-Path $envFile)) {
        Write-Host 'No .env - skipping compose down.' -ForegroundColor Yellow
        return
    }

    Set-Location $Root

    $profile = 'core'
    $profileFile = Join-Path $Root '.streamclone-profile'
    if (Test-Path $profileFile) {
        $profile = (Get-Content $profileFile -Raw).Trim()
    } elseif (Test-Path $envFile) {
        $vals = Read-EnvKeyValueFile -Path $envFile
        if ($vals['STREAMCLONE_PROFILE']) { $profile = $vals['STREAMCLONE_PROFILE'] }
    }

    $composeArgs = @(
        'compose', '--env-file', '.env',
        '-f', 'deploy/docker-compose.yml',
        '-f', 'deploy/docker-compose.local-tunnel.yml'
    )
    if (Test-StreamcloneUseReleaseImages -Root $Root) {
        $composeArgs += '-f', 'deploy/docker-compose.release.yml'
    }
    foreach ($p in @('scraper', 'clipper')) {
        $composeArgs += '--profile', $p
    }

    $downArgs = @('down', '--remove-orphans', '--timeout', '30')
    if ($Volumes) { $downArgs += '-v' }

    $prev = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        Write-Host 'Stopping Docker stack...' -ForegroundColor Cyan
        $result = Invoke-EnvDockerCaptured -Arguments ($composeArgs + $downArgs)
        foreach ($line in $result.Output) { Write-Host $line }
        $composeArgsNoProfiles = @(
            'compose', '--env-file', '.env',
            '-f', 'deploy/docker-compose.yml',
            '-f', 'deploy/docker-compose.local-tunnel.yml'
        )
        if (Test-StreamcloneUseReleaseImages -Root $Root) {
            $composeArgsNoProfiles += '-f', 'deploy/docker-compose.release.yml'
        }
        $result = Invoke-EnvDockerCaptured -Arguments ($composeArgsNoProfiles + $downArgs)
        foreach ($line in $result.Output) { Write-Host $line }
    } finally {
        $ErrorActionPreference = $prev
    }
}

function Remove-StreamcloneDesktopShortcuts {
    $desktop = [Environment]::GetFolderPath('Desktop')
    foreach ($name in @(
        'Streamclone.lnk',
        'Start Streamclone.lnk',
        'Stop Streamclone.lnk',
        'Manage Streamclone.lnk',
        'Check Streamclone.lnk',
        'Uninstall Streamclone.lnk'
    )) {
        $path = Join-Path $desktop $name
        if (Test-Path $path) {
            Remove-Item $path -Force
            Write-Host "Removed Desktop\$name"
        }
    }
}

function Remove-StreamcloneMacShortcuts {
    $appsDir = Join-Path $env:USERPROFILE 'Applications'
    if (-not (Test-Path $appsDir)) { return }
    foreach ($name in @(
        'Streamclone Start.command',
        'Streamclone Stop.command',
        'Streamclone Install.command',
        'Streamclone Manage.command',
        'Streamclone Check.command',
        'Streamclone Uninstall.command'
    )) {
        $path = Join-Path $appsDir $name
        if (Test-Path $path) {
            Remove-Item $path -Force -ErrorAction SilentlyContinue
            Write-Host "Removed $path"
        }
    }
}

function Remove-StreamcloneConfigFiles {
    param([string]$Root)
    foreach ($name in @('.env', '.streamclone-profile', '.streamclone-setup-control.pid')) {
        $path = Join-Path $Root $name
        if (Test-Path $path) {
            Remove-Item $path -Force
            Write-Host "Removed $name"
        }
    }
}

function Remove-StreamcloneImages {
    param(
        [string]$Root,
        [string]$Tag
    )
    if (-not $Tag) {
        $envFile = Join-Path $Root '.env'
        if (Test-Path $envFile) {
            $vals = Read-EnvKeyValueFile -Path $envFile
            $Tag = $vals['IMAGE_TAG']
        }
        $versionFile = Join-Path $Root 'VERSION'
        if (-not $Tag -and (Test-Path $versionFile)) {
            $Tag = (Get-Content $versionFile -Raw).Trim()
        }
    }
    if (-not $Tag) { $Tag = 'latest' }

    Write-Host "Pruning GHCR images (tag: $Tag)..." -ForegroundColor Cyan
    $prev = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        foreach ($ref in (Get-StreamcloneCoreImageRefs -Tag $Tag)) {
            Invoke-EnvDockerCaptured -Arguments @('image', 'rm', '-f', $ref) | Out-Null
        }
    } finally {
        $ErrorActionPreference = $prev
    }
}

function Remove-InstallDirectoryDeferred {
    param([string]$Root)
    $escaped = $Root.Replace("'", "''")
    $cleanup = @"
Set-Location `$env:TEMP
Start-Sleep -Seconds 2
Remove-Item -LiteralPath '$escaped' -Recurse -Force -ErrorAction SilentlyContinue
"@
    Start-Process -FilePath 'powershell.exe' `
        -ArgumentList @('-NoProfile', '-WindowStyle', 'Hidden', '-Command', $cleanup) `
        -WorkingDirectory $env:TEMP | Out-Null
    Write-Host "Scheduled removal of install folder: $Root" -ForegroundColor Yellow
}

try {
    $root = Get-StreamcloneInstallRoot -Hint $InstallDir
    $removeImages = $PruneImages.IsPresent
    $removeBase = $PruneBaseImages.IsPresent

    if (-not $removeImages -and -not $SkipImagePrompt -and -not $NonInteractive -and -not $ProgressFile) {
        $estimate = Get-StreamcloneDockerReclaimEstimate -Root $root
        if ($estimate.imageCount -gt 0) {
            Write-Host ''
            Write-Host "Streamclone Docker images: $($estimate.label) on disk ($($estimate.imageCount) image(s))." -ForegroundColor DarkGray
        }
        Write-Host ''
        Write-Host 'Docker image cleanup' -ForegroundColor Cyan
        Write-Host 'Streamclone images are safe to keep cached for faster reinstall/repair.'
        Write-Host 'Remove them only when you want to reclaim disk space or simulate a first-time install.'
        $pruneAns = Read-Host 'Also remove downloaded Streamclone Docker images? [y/N]'
        $removeImages = ($pruneAns -match '^[Yy]')
        if ($removeImages) {
            $baseAns = Read-Host 'Also remove base images (postgres, redis, caddy, ...)? [y/N]'
            $removeBase = ($baseAns -match '^[Yy]')
        }
    }

    if (-not $ProgressFile) {
        Write-Host ''
        Write-Host 'Streamclone - Complete uninstall' -ForegroundColor Red
        Write-Host '================================' -ForegroundColor Red
        Write-Host "Install folder: $root"
        Write-Host ''
        Write-Host 'This will:'
        Write-Host '  - Stop all Streamclone Docker containers'
        if (-not $KeepVolumes) {
            Write-Host '  - Delete Docker volumes (database, MinIO, clipper data)'
        } else {
            Write-Host '  - Stop containers (keep Docker volumes)'
        }
        Write-Host '  - Remove .env and local secrets'
        Write-Host '  - Remove Desktop / macOS shortcuts (Start, Stop, Manage, Check, Uninstall)'
        if ($removeImages) {
            Write-Host '  - Remove downloaded ghcr.io/aron-chu/streamclone images'
            if ($removeBase) {
                Write-Host '  - Remove base images (postgres, redis, minio, caddy, mediamtx, migrate)'
            }
        } else {
            Write-Host '  - Keep cached Docker images for faster reinstall'
        }
        if (-not $KeepInstallDir) {
            Write-Host '  - Delete the install folder'
        }
        Write-Host ''
    }

    if (-not $NonInteractive -and -not $ProgressFile) {
        $ans = Read-Host 'Type YES to continue'
        if ($ans -ne 'YES') {
            Write-Host 'Uninstall cancelled.' -ForegroundColor Yellow
            exit 2
        }
    }

    Set-UninstallProgress -Title 'Stopping Streamclone' -Detail 'Shutting down background processes.'
    Stop-StreamcloneControlProcess -Root $root

    $stopScript = Join-Path $PSScriptRoot 'stop-streamclone.ps1'
    if (Test-Path $stopScript) {
        Set-UninstallProgress -Title 'Stopping containers' -Detail 'Graceful stop before teardown.'
        & $stopScript
        Start-Sleep -Seconds 2
    }

    Set-UninstallProgress -Title 'Removing Docker stack' -Detail 'Stopping containers and deleting volumes.'
    Invoke-StreamcloneComposeDown -Root $root -Volumes:(-not $KeepVolumes)

    if ($removeImages) {
        Set-UninstallProgress -Title 'Removing Docker images' -Detail 'Pruning ghcr.io/aron-chu/streamclone images.'
        Remove-StreamcloneImages -Root $root
        if ($removeBase) {
            Remove-StreamcloneBaseImages
        }
    }

    Set-UninstallProgress -Title 'Removing local data' -Detail 'Deleting secrets, shortcuts, and config.'
    Remove-StreamcloneConfigFiles -Root $root
    Remove-StreamcloneDesktopShortcuts
    Remove-StreamcloneMacShortcuts

    if ($KeepInstallDir) {
        if (-not $ProgressFile) {
            Write-Host ''
            Write-Host "Uninstall complete (install folder kept): $root" -ForegroundColor Green
        }
    } else {
        Remove-InstallDirectoryDeferred -Root $root
        if (-not $ProgressFile) {
            Write-Host ''
            Write-Host 'Uninstall complete. Install folder will be removed shortly.' -ForegroundColor Green
            Write-Host "If $root remains, close terminals/File Explorer in that folder and delete manually." -ForegroundColor DarkGray
        }
    }

    Complete-UninstallProgress -ExitCode 0
} catch {
    Set-UninstallProgress -Title 'Uninstall failed' -Detail $_.Exception.Message -Status 'done|1'
    if (-not $ProgressFile) { throw }
    exit 1
}
