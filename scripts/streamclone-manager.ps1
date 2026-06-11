#Requires -Version 5.1
# Streamclone Manager — install folder maintenance: status, repair, start/stop, uninstall.
param(
    [ValidateSet('menu', 'status', 'start', 'stop', 'repair', 'uninstall', 'open', 'logs')]
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

function Write-ManagerBanner {
    Write-Host ''
    Write-Host 'Streamclone Manager' -ForegroundColor Cyan
    Write-Host '=================' -ForegroundColor Cyan
    Write-Host "Install folder: $Root"
    Write-Host ''
}

function Get-ManagerInstallRoot {
    if (-not (Test-Path (Join-Path $Root 'scripts\start-streamclone.ps1'))) {
        throw "Streamclone is not installed at $Root. Run Setup.exe or Install Streamclone.cmd first."
    }
}

function Show-ManagerStatus {
    Get-ManagerInstallRoot
    Write-Host '--- Prerequisites ---' -ForegroundColor Yellow
    & (Join-Path $PSScriptRoot 'preflight-deps.ps1') -InstallHints
  if ($LASTEXITCODE -ne 0) { return $false }

    Write-Host ''
    Write-Host '--- Containers ---' -ForegroundColor Yellow
    $summary = Get-StreamcloneContainerSummary -Root $Root
    Write-Host $summary
    Write-Host ''

    $url = 'http://localhost:8090/'
    try {
        $sw = [System.Diagnostics.Stopwatch]::StartNew()
        $resp = Invoke-WebRequest -Uri $url -UseBasicParsing -TimeoutSec 8
        $sw.Stop()
        if ($resp.StatusCode -ge 200 -and $resp.StatusCode -lt 500) {
            Write-Host "Web UI: ready at $url (${sw.ElapsedMilliseconds}ms)" -ForegroundColor Green
            return $true
        }
        Write-Host "Web UI: responded HTTP $($resp.StatusCode)" -ForegroundColor Yellow
    } catch {
        Write-Host "Web UI: not ready at $url ($($_.Exception.Message))" -ForegroundColor Red
        Write-Host 'Tip: first install can take 3-8 minutes while images download and services become healthy.' -ForegroundColor DarkGray
    }
    return $false
}

function Invoke-ManagerRepair {
    Get-ManagerInstallRoot
    if (-not (Test-Path (Join-Path $Root '.env'))) {
        throw 'Missing .env — run setup first (Install Streamclone or Setup.exe).'
    }

    Write-Host 'Repair will:' -ForegroundColor Cyan
    Write-Host '  1. Re-pull release images from GHCR'
    Write-Host '  2. Recreate containers (keeps database and MinIO data)'
    Write-Host '  3. Wait until http://localhost:8090 responds'
    Write-Host ''
    Write-Host 'First repair after a clean machine may take several minutes (large video image).' -ForegroundColor DarkGray
    Write-Host ''

    if (-not $NonInteractive) {
        $ans = Read-Host 'Continue? [Y/n]'
        if ($ans -match '^[Nn]') { return }
    }

    & (Join-Path $PSScriptRoot 'preflight-deps.ps1') -InstallHints
    if ($LASTEXITCODE -ne 0) { throw 'Preflight failed — start Docker Desktop and retry.' }

    $profile = Get-StreamcloneProfileFromRoot -Root $Root
    $composeArgs = Get-StreamcloneComposeArgs -Root $Root -Profile $profile

    Write-Host 'Pulling images...' -ForegroundColor Cyan
    $pull = Invoke-EnvDockerCaptured -Arguments ($composeArgs + @('pull'))
    if ($pull.ExitCode -ne 0) {
        throw "docker compose pull failed: $($pull.Output -join [Environment]::NewLine)"
    }

    Write-Host 'Recreating containers...' -ForegroundColor Cyan
    $up = Invoke-EnvDockerCaptured -Arguments ($composeArgs + @('up', '-d', '--remove-orphans', '--force-recreate', '--pull', 'always'))
    if ($up.ExitCode -ne 0) {
        throw "docker compose up failed: $($up.Output -join [Environment]::NewLine)"
    }

    & (Join-Path $PSScriptRoot 'lib\wait-stack.ps1') -TimeoutSec 300
    Write-Host ''
    Write-Host 'Repair complete.' -ForegroundColor Green
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

function Show-ManagerMenu {
    while ($true) {
        Write-ManagerBanner
        Write-Host '  1) Open Streamclone (start stack + browser)'
        Write-Host '  2) Stop Streamclone'
        Write-Host '  3) Status / diagnostics'
        Write-Host '  4) Repair (re-pull images + restart services)'
        Write-Host '  5) View recent logs'
        Write-Host '  6) Uninstall'
        Write-Host '  7) Exit'
        Write-Host ''
        $choice = Read-Host 'Choose an option [1-7]'
        switch ($choice) {
            '1' {
                & (Join-Path $PSScriptRoot 'start-streamclone.ps1')
                return
            }
            '2' {
                & (Join-Path $PSScriptRoot 'stop-streamclone.ps1')
            }
            '3' { Show-ManagerStatus }
            '4' { Invoke-ManagerRepair }
            '5' {
                Get-ManagerInstallRoot
                $composeArgs = Get-StreamcloneComposeArgs -Root $Root
                Invoke-EnvDocker -Arguments ($composeArgs + @('logs', '--tail', '80'))
            }
            '6' {
                $prune = Read-Host 'Also remove downloaded Docker images? [y/N]'
                if ($prune -match '^[Yy]') { Invoke-ManagerUninstall -PruneImages }
                else { Invoke-ManagerUninstall }
                return
            }
            '7' { return }
            default { Write-Host 'Invalid choice.' -ForegroundColor Yellow }
        }
        Write-Host ''
        Read-Host 'Press Enter to continue'
    }
}

function Get-StreamcloneContainerSummary {
    param([string]$Root)
    $prev = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        $composeArgs = Get-StreamcloneComposeArgs -Root $Root
        $result = Invoke-EnvDockerCaptured -Arguments ($composeArgs + @('ps', '-a', '--format', '{{.Name}}|{{.Status}}'))
        if ($result.ExitCode -ne 0) { return 'Docker status unavailable.' }
        $lines = $result.Output
        if (-not $lines) { return 'No Streamclone containers found.' }
        $parts = foreach ($line in $lines) {
            $split = $line -split '\|', 2
            $name = ($split[0] -replace '^streamclone-', '' -replace '-\d+$', '')
            $state = $split[1]
            if ($state -match 'healthy') { "$name ready" }
            elseif ($state -match 'Up') { "$name starting" }
            elseif ($state -match 'Exited \(0\)') { "$name done" }
            else { "$name $state" }
        }
        return ($parts -join ' · ')
    } finally {
        $ErrorActionPreference = $prev
    }
}

try {
    switch ($Action) {
        'menu' { Show-ManagerMenu }
        'status' { if (-not (Show-ManagerStatus)) { exit 1 } }
        'start' {
            Get-ManagerInstallRoot
            & (Join-Path $PSScriptRoot 'start-streamclone.ps1')
        }
        'stop' {
            Get-ManagerInstallRoot
            & (Join-Path $PSScriptRoot 'stop-streamclone.ps1')
        }
        'repair' { Invoke-ManagerRepair }
        'uninstall' { Invoke-ManagerUninstall }
        'open' {
            Get-ManagerInstallRoot
            Start-Process 'http://localhost:8090/'
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
