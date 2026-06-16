#Requires -Version 5.1
param(
    [string]$InstallDir = '',
    [switch]$NonInteractive,
    [switch]$PruneImages,
    [switch]$PruneBaseImages,
    [switch]$SkipImagePrompt,
    [switch]$KeepInstallDir,
    [switch]$KeepVolumes,
    [switch]$SkipDockerCleanup,
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

function Test-StreamcloneDockerEngineRunning {
    $result = Invoke-EnvDockerCapturedWithTimeout -Arguments @('info') -TimeoutSec 10
    return (-not $result.TimedOut -and $result.ExitCode -eq 0)
}

function Wait-StreamcloneDockerEngine {
    param([int]$WaitSec = 120)
    $preflight = Join-Path $PSScriptRoot 'preflight-deps.ps1'
    if (Test-Path $preflight) {
        $raw = & $preflight -Quiet -Json -TryStartDocker 2>&1 | Select-Object -Last 1
        try {
            $summary = $raw | ConvertFrom-Json
            if ($summary.dockerEngineRunning) { return $true }
        } catch { }
    }
    $deadline = (Get-Date).AddSeconds($WaitSec)
    while ((Get-Date) -lt $deadline) {
        if (Test-StreamcloneDockerEngineRunning) { return $true }
        Start-Sleep -Seconds 3
    }
    return $false
}

function Test-StreamcloneUninstallNeedsDocker {
    param(
        [string]$Root,
        [bool]$RemoveImages
    )
    if ($RemoveImages) { return $true }
    return (Test-Path (Join-Path $Root '.env'))
}

function Resolve-StreamcloneUninstallDockerPlan {
    param(
        [string]$Root,
        [bool]$NeedsDocker,
        [bool]$NonInteractive
    )
    if (-not $NeedsDocker -or $SkipDockerCleanup) { return 'proceed' }
    if (Test-StreamcloneDockerEngineRunning) { return 'proceed' }
    if ($NonInteractive) { return 'defer' }

    Write-Host ''
    Write-Host 'Docker Desktop is not running.' -ForegroundColor Yellow
    Write-Host 'Containers, volumes, and images cannot be removed until the Docker engine is available.'
    Write-Host ''
    Write-Host '  [1] Start Docker Desktop and wait (recommended)'
    Write-Host '  [2] Defer Docker cleanup - remove shortcuts now; run Finish Streamclone Docker cleanup later'
    Write-Host '  [3] Cancel uninstall'
    Write-Host ''

    while ($true) {
        $choice = Read-Host 'Choose 1, 2, or 3 (default 1)'
        if ([string]::IsNullOrWhiteSpace($choice)) { $choice = '1' }
        switch ($choice.Trim()) {
            '3' { return 'cancel' }
            '2' { return 'defer' }
            '1' {
                Write-Host 'Waiting for Docker Desktop...' -ForegroundColor Cyan
                if (Wait-StreamcloneDockerEngine) {
                    Write-Host 'Docker is ready.' -ForegroundColor Green
                    return 'proceed'
                }
                Write-Host 'Docker is still not available.' -ForegroundColor Red
                $retry = Read-Host 'Continue without Docker cleanup? [y/N] (adds Finish Streamclone Docker cleanup shortcut)'
                if ($retry -match '^[Yy]') { return 'defer' }
                return 'cancel'
            }
            default {
                Write-Host 'Enter 1, 2, or 3.' -ForegroundColor Yellow
            }
        }
    }
}

function Save-StreamclonePendingDockerUninstall {
    param(
        [string]$Root,
        [bool]$RemoveImages,
        [bool]$RemoveBaseImages,
        [bool]$KeepVolumes
    )
    $dir = Join-Path $env:LOCALAPPDATA 'Streamclone'
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
    $stateFile = Join-Path $dir 'pending-docker-uninstall.json'
    @{
        installDir       = $Root
        removeImages     = $RemoveImages
        removeBaseImages = $RemoveBaseImages
        keepVolumes      = $KeepVolumes
        createdAt        = (Get-Date).ToString('o')
    } | ConvertTo-Json | Set-Content -LiteralPath $stateFile -Encoding UTF8
    return $stateFile
}

function Install-StreamcloneFinishDockerCleanupShortcut {
    param([string]$Root)
    $finishScript = Join-Path $Root 'scripts\finish-docker-uninstall.ps1'
    if (-not (Test-Path $finishScript)) {
        Write-Warning "Missing $finishScript - cannot create deferred cleanup shortcut."
        return
    }
    $desktop = [Environment]::GetFolderPath('Desktop')
    $cmdPath = Join-Path $desktop 'Finish Streamclone Docker cleanup.cmd'
    @(
        '@echo off'
        'color 0E'
        'title Streamclone - Finish Docker cleanup'
        'echo.'
        'echo   Start Docker Desktop first, then this script removes containers,'
        'echo   volumes, images, and the Streamclone install folder.'
        'echo.'
        ('powershell -NoProfile -ExecutionPolicy Bypass -File "' + $finishScript + '"')
        'if errorlevel 1 pause'
    ) | Set-Content -LiteralPath $cmdPath -Encoding ASCII
    Write-Host "Added Desktop\Finish Streamclone Docker cleanup.cmd" -ForegroundColor Green
}

function Stop-StreamcloneControlProcess {
    param([string]$Root)
    $controlPidFile = Join-Path $Root '.streamclone-setup-control.pid'
    if (Test-Path $controlPidFile) {
        $controlPid = (Get-Content $controlPidFile -Raw).Trim()
        if ($controlPid -match '^\d+$') {
            Stop-Process -Id ([int]$controlPid) -Force -ErrorAction SilentlyContinue
        }
        Remove-Item $controlPidFile -Force -ErrorAction SilentlyContinue
    }

    Get-CimInstance Win32_Process -Filter "Name='powershell.exe'" -ErrorAction SilentlyContinue |
        Where-Object { $_.CommandLine -like '*setup-control.ps1*' -and $_.CommandLine -notlike '*ensure-setup-control.ps1*' } |
        ForEach-Object {
            try { Stop-Process -Id ([int]$_.ProcessId) -Force -ErrorAction SilentlyContinue } catch { }
        }
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
    if ($profile -notin @('core', 'scraper', 'clipper', 'full')) {
        # Unknown/legacy profile marker: tear down everything to be safe.
        $profile = 'full'
    }
    $optionalProfiles = @((Get-EnvComposeProfiles -Profile $profile) + @('pulse') | Select-Object -Unique)
    if ($optionalProfiles.Count -gt 0) {
        Write-Host "Installed profile: $profile (optional services: $($optionalProfiles -join ', '))" -ForegroundColor DarkGray
    } else {
        Write-Host "Installed profile: $profile (no optional services)" -ForegroundColor DarkGray
    }
    foreach ($p in $optionalProfiles) {
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
        if ($result.ExitCode -ne 0) {
            $joined = ($result.Output -join ' ')
            if ($joined -match 'cannot find the file specified|docker API|Is the docker daemon running') {
                Write-Host 'Docker engine unavailable - stack may still be running. Start Docker Desktop and run Finish Streamclone Docker cleanup.cmd if needed.' -ForegroundColor Yellow
            }
        }
        if ($optionalProfiles.Count -gt 0) {
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
        }
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
    foreach ($name in @('.env', '.streamclone-profile', '.streamclone-setup-control.pid', 'runtime\clipper-twitch.env')) {
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
        # Scraper image is profile-gated and not in the core repo list; prune it explicitly.
        $refs = @(Get-StreamcloneCoreImageRefs -Tag $Tag) + "ghcr.io/aron-chu/streamclone/scraper:$Tag"
        foreach ($ref in $refs) {
            Invoke-EnvDockerCaptured -Arguments @('image', 'rm', '-f', $ref) | Out-Null
        }
    } finally {
        $ErrorActionPreference = $prev
    }
}

function Remove-InstallDirectoryDeferred {
    param([string]$Root)
    $escaped = $Root.Replace("'", "''")
    $cleanup = @(
        'Set-Location $env:TEMP',
        'Start-Sleep -Seconds 2',
        "Remove-Item -LiteralPath '$escaped' -Recurse -Force -ErrorAction SilentlyContinue"
    ) -join '; '
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
        Write-Host '  - Remove the Streamclone Desktop shortcut and legacy shortcuts'
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

    $needsDocker = Test-StreamcloneUninstallNeedsDocker -Root $root -RemoveImages:$removeImages
    $dockerPlan = Resolve-StreamcloneUninstallDockerPlan -Root $root -NeedsDocker:$needsDocker -NonInteractive:$NonInteractive
    if ($dockerPlan -eq 'cancel') {
        Write-Host 'Uninstall cancelled.' -ForegroundColor Yellow
        exit 2
    }
    $deferDocker = ($dockerPlan -eq 'defer')

    if ($deferDocker -and -not $ProgressFile) {
        Write-Host 'Deferred Docker cleanup:' -ForegroundColor Yellow
        Write-Host '  - Shortcuts removed (except Finish Streamclone Docker cleanup)'
        Write-Host '  - Install folder and .env kept until Docker cleanup finishes'
        Write-Host '  - Start Docker Desktop, then run Finish Streamclone Docker cleanup.cmd'
        Write-Host ''
    }

    if (-not $NonInteractive -and -not $ProgressFile) {
        $ans = Read-Host 'Type YES to continue'
        if ($ans.Trim().ToUpperInvariant() -ne 'YES') {
            Write-Host 'Uninstall cancelled.' -ForegroundColor Yellow
            exit 2
        }
    }

    if ($deferDocker) {
        Set-UninstallProgress -Title 'Deferred Docker cleanup' -Detail 'Removing shortcuts; keeping install folder until Docker is running.'
        Stop-StreamcloneControlProcess -Root $root
        $null = Save-StreamclonePendingDockerUninstall -Root $root -RemoveImages:$removeImages `
            -RemoveBaseImages:$removeBase -KeepVolumes:$KeepVolumes
        Remove-StreamcloneDesktopShortcuts
        Remove-StreamcloneMacShortcuts
        Install-StreamcloneFinishDockerCleanupShortcut -Root $root
        if (-not $ProgressFile) {
            Write-Host ''
            Write-Host 'Partial uninstall complete.' -ForegroundColor Green
            Write-Host 'Start Docker Desktop, then double-click Finish Streamclone Docker cleanup.cmd on your Desktop.' -ForegroundColor Yellow
        }
        Complete-UninstallProgress -ExitCode 0
        exit 3
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
