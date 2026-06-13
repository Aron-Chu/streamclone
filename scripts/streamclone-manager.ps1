#Requires -Version 5.1
# Streamclone Manager — install folder maintenance: status, repair, start/stop, uninstall.
param(
    [ValidateSet('menu', 'status', 'start', 'stop', 'repair', 'update', 'uninstall', 'open', 'logs')]
    [string]$Action = 'menu',
    [string]$InstallDir = '',
    [switch]$NonInteractive,
    [switch]$PruneImages
)

$ErrorActionPreference = 'Stop'
$Root = if ($InstallDir) {
    (Resolve-Path -LiteralPath $InstallDir).Path
} else {
    Split-Path -Parent $PSScriptRoot
}

. (Join-Path $PSScriptRoot 'lib\env.ps1')
. (Join-Path $PSScriptRoot 'lib\stack-progress.ps1')
. (Join-Path $PSScriptRoot 'lib\install-upgrade.ps1')

function Write-ManagerBanner {
    Write-Host ''
    Write-Host 'Streamclone Manager' -ForegroundColor Cyan
    Write-Host '=================' -ForegroundColor Cyan
    Write-Host "Install folder: $Root"
    $versions = Get-StreamcloneInstallVersions -Root $Root
    if ($versions.bundleVersion) {
        $line = "Bundle $($versions.bundleVersion)"
        if ($versions.imageTag -and ($versions.imageTag -ne $versions.bundleVersion)) {
            $line += " | Images $($versions.imageTag)"
        }
        Write-Host $line -ForegroundColor DarkGray
    }
    Write-Host ''
}

function Get-ManagerInstallRoot {
    if (-not (Test-Path (Join-Path $Root 'scripts\start-streamclone.ps1'))) {
        throw "Streamclone is not installed at $Root. Run Setup.exe or Install Streamclone.cmd first."
    }
}

function Invoke-ManagerRepair {
    Get-ManagerInstallRoot
    if (-not (Test-Path (Join-Path $Root '.env'))) {
        throw 'Missing .env - run setup first (Install Streamclone or Setup.exe).'
    }

    Write-Host 'Repair will:' -ForegroundColor Cyan
    Write-Host '  1. Re-pull release images from GHCR'
    Write-Host '  2. Recreate containers (keeps database and MinIO data)'
    Write-Host ('  3. Wait until {0} responds' -f (Get-StreamcloneAppUrl))
    Write-Host ''
    Write-Host 'First repair after a clean machine may take several minutes (large video image).' -ForegroundColor DarkGray
    Write-Host ''

    if (-not $NonInteractive) {
        $ans = Read-Host 'Continue? [Y/n]'
        if ($ans -match '^[Nn]') { return }
    }

    & (Join-Path $PSScriptRoot 'preflight-deps.ps1') -InstallHints
    if ($LASTEXITCODE -ne 0) { throw 'Preflight failed - start Docker Desktop and retry.' }

    $profile = Get-StreamcloneProfileFromRoot -Root $Root
    $composeArgs = Get-StreamcloneComposeArgs -Root $Root -Profile $profile

    Write-Host 'Pulling Docker images (~1.5 GB, 3-8 min on first install)...' -ForegroundColor Cyan
    Write-Host ''
    $pull = Invoke-EnvDockerComposePullWithRetry -ComposeArgs $composeArgs -OutputMode friendly
    if ($pull.ExitCode -ne 0) {
        throw "docker compose pull failed: $($pull.Output -join [Environment]::NewLine)"
    }

    Write-Host 'Recreating containers...' -ForegroundColor Cyan
    $up = Invoke-EnvDockerStreaming -Arguments ($composeArgs + @('up', '-d', '--remove-orphans', '--force-recreate', '--pull', 'always'))
    if ($up.ExitCode -ne 0) {
        throw "docker compose up failed: $($up.Output -join [Environment]::NewLine)"
    }

    & (Join-Path $PSScriptRoot 'lib\wait-stack.ps1') -TimeoutSec 300
    Write-Host ''
    Write-Host 'Repair complete.' -ForegroundColor Green
}

function Invoke-ManagerUpdate {
    Get-ManagerInstallRoot
    if (-not (Test-Path (Join-Path $Root '.env'))) {
        throw 'Missing .env - run setup first (Install Streamclone or Setup.exe).'
    }

    if (-not (Test-StreamcloneUpgradeNeeded -Root $Root)) {
        $versions = Get-StreamcloneInstallVersions -Root $Root
        Write-Host "Already up to date (bundle $($versions.bundleVersion), images $($versions.imageTag))." -ForegroundColor Green
        return
    }

    $versions = Get-StreamcloneInstallVersions -Root $Root
    Write-Host "Update will sync IMAGE_TAG to $($versions.bundleVersion) and re-pull images." -ForegroundColor Cyan
    Write-Host ''

    if (-not $NonInteractive) {
        $ans = Read-Host 'Continue? [Y/n]'
        if ($ans -match '^[Nn]') { return }
    }

    & (Join-Path $PSScriptRoot 'preflight-deps.ps1') -InstallHints
    if ($LASTEXITCODE -ne 0) { throw 'Preflight failed - start Docker Desktop and retry.' }

    Invoke-StreamcloneUpgrade -Root $Root
}

function Invoke-ManagerUninstall {
    Get-ManagerInstallRoot
    $args = @{
        InstallDir = $Root
    }
    if ($NonInteractive) { $args['NonInteractive'] = $true }
    if ($PruneImages) { $args['PruneImages'] = $true }
    & (Join-Path $PSScriptRoot 'uninstall-streamclone.ps1') @args
}

function Invoke-ManagerResetConfig {
    Get-ManagerInstallRoot
    $resetScript = Join-Path $PSScriptRoot 'reset-streamclone-config.ps1'
    if (-not (Test-Path $resetScript)) {
        throw "Missing $resetScript"
    }
    & $resetScript -InstallDir $Root -NonInteractive:$NonInteractive
}

function Show-ManagerUninstallMenu {
    Write-Host ''
    Write-Host '  Uninstall options:' -ForegroundColor Yellow
    Write-Host '    1) Full remove (volumes, config, shortcuts, install folder)'
    Write-Host '    2) Reset config only (keep install folder and data volumes)'
    Write-Host '    3) Cancel'
    Write-Host ''
    $choice = Read-Host 'Choose [1-3]'
    switch ($choice) {
        '1' { Invoke-ManagerUninstall; return $true }
        '2' { Invoke-ManagerResetConfig; return $true }
        '3' { return $false }
        default {
            Write-Host 'Invalid choice.' -ForegroundColor Yellow
            return $false
        }
    }
}

function Show-ManagerMenu {
    while ($true) {
        Write-ManagerBanner
        Write-Host '  1) Open Streamclone (start stack + browser)'
        Write-Host '  2) Stop Streamclone'
        Write-Host '  3) Status / diagnostics'
        Write-Host '  4) Repair (re-pull images + restart services)'
        if (Test-StreamcloneUpgradeNeeded -Root $Root) {
            Write-Host '  5) Update (sync IMAGE_TAG and pull new images)' -ForegroundColor Yellow
        } else {
            Write-Host '  5) Update (already on current bundle tag)'
        }
        Write-Host '  6) View recent logs'
        Write-Host '  7) Uninstall'
        Write-Host '  8) Exit'
        Write-Host ''
        $choice = Read-Host 'Choose an option [1-8]'
        switch ($choice) {
            '1' {
                & (Join-Path $PSScriptRoot 'start-streamclone.ps1')
                return
            }
            '2' {
                & (Join-Path $PSScriptRoot 'stop-streamclone.ps1')
            }
            '3' { & (Join-Path $PSScriptRoot 'check-streamclone.ps1') -InstallDir $Root }
            '4' { Invoke-ManagerRepair }
            '5' { Invoke-ManagerUpdate }
            '6' {
                Get-ManagerInstallRoot
                $composeArgs = Get-StreamcloneComposeArgs -Root $Root
                Invoke-EnvDocker -Arguments ($composeArgs + @('logs', '--tail', '80'))
            }
            '7' {
                if (Show-ManagerUninstallMenu) { return }
            }
            '8' { return }
            default { Write-Host 'Invalid choice.' -ForegroundColor Yellow }
        }
        Write-Host ''
        Read-Host 'Press Enter to continue'
    }
}

try {
    switch ($Action) {
        'menu' { Show-ManagerMenu }
        'status' {
            & (Join-Path $PSScriptRoot 'check-streamclone.ps1') -InstallDir $Root
            if ($LASTEXITCODE -ne 0) { exit 1 }
        }
        'start' {
            Get-ManagerInstallRoot
            & (Join-Path $PSScriptRoot 'start-streamclone.ps1')
        }
        'stop' {
            Get-ManagerInstallRoot
            & (Join-Path $PSScriptRoot 'stop-streamclone.ps1')
        }
        'repair' { Invoke-ManagerRepair }
        'update' { Invoke-ManagerUpdate }
        'uninstall' { Invoke-ManagerUninstall }
        'open' {
            Get-ManagerInstallRoot
            Start-Process (Get-StreamcloneAppUrl)
        }
        'logs' {
            Get-ManagerInstallRoot
            $composeArgs = Get-StreamcloneComposeArgs -Root $Root
            Invoke-EnvDocker -Arguments ($composeArgs + @('logs', '--tail', '120'))
        }
    }
} catch {
    Write-Host $_.Exception.Message -ForegroundColor Red
    exit 1
}
