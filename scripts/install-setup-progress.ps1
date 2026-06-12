#Requires -Version 5.1
# Headless setup for Streamclone-Setup.exe — writes progress for the Inno wizard UI.
param(
    [Parameter(Mandatory = $true)]
    [string]$InstallDir,
    [Parameter(Mandatory = $true)]
    [string]$ProgressFile
)

$ErrorActionPreference = 'Stop'

. (Join-Path $PSScriptRoot 'lib\stack-progress.ps1')
. (Join-Path $PSScriptRoot 'lib\install-upgrade.ps1')

function Set-InstallProgress {
    param(
        [string]$Title,
        [string]$Detail = '',
        [string]$Status = 'running'
    )
    @(
        "TITLE=$Title"
        "DETAIL=$Detail"
        "STATUS=$Status"
    ) | Set-Content -LiteralPath $ProgressFile -Encoding UTF8
}

function Complete-InstallProgress {
    param([int]$ExitCode = 0)
    Set-InstallProgress -Title 'Setup complete' -Detail 'Opening Streamclone...' -Status "done|$ExitCode"
}

function Get-StreamcloneContainerSummary {
    $prev = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        $result = Invoke-EnvDockerCaptured -Arguments @('ps', '-a', '--filter', 'name=streamclone', '--format', '{{.Names}}|{{.Status}}')
        if ($result.ExitCode -ne 0) { return 'Docker status unavailable...' }
        $lines = $result.Output
        if (-not $lines) { return 'Starting containers...' }
        $parts = foreach ($line in $lines) {
            $split = $line -split '\|', 2
            $name = ($split[0] -replace '^streamclone-', '')
            $state = $split[1]
            if ($state -match 'healthy') { "$name ready" }
            elseif ($state -match 'Up') { "$name starting" }
            elseif ($state -match 'Exited \(0\)') { "$name done" }
            else { "$name $state" }
        }
        return ($parts -join ' | ')
    } finally {
        $ErrorActionPreference = $prev
    }
}

function Invoke-DockerComposePullWithProgress {
    param([string[]]$ComposeArgs)
    Set-InstallProgress -Title 'Pulling Docker images' -Detail 'First install downloads ~1.5 GB (video and emote are largest).'
    $prev = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        $onPullLine = {
            param($line)
            $text = "$line".Trim()
            if ($text -match '\[\+\]\s+Pulling\s+(\d+)/(\d+)') {
                Set-InstallProgress -Title 'Pulling Docker images' -Detail "Image $($matches[1]) of $($matches[2])"
                return
            }
            if ($text -match 'Pulled|up to date|Pull complete') {
                $short = $text
                if ($short.Length -gt 90) { $short = $short.Substring(0, 87) + '...' }
                Set-InstallProgress -Title 'Pulling Docker images' -Detail $short
            }
        }
        $result = Invoke-EnvDockerComposePullWithRetry -ComposeArgs $ComposeArgs -OnLine $onPullLine -OutputMode friendly
        $pullOutput = $result.Output
        $pullExitCode = $result.ExitCode
        if ($pullExitCode -ne 0) {
            $logsDir = Join-Path $InstallDir 'logs'
            New-Item -ItemType Directory -Force -Path $logsDir | Out-Null
            $pullLog = Join-Path $logsDir 'setup-pull.log'
            @($pullOutput) | Set-Content -LiteralPath $pullLog -Encoding UTF8
            throw "docker compose pull failed after retries (exit $pullExitCode). Log: $pullLog. Try Start Streamclone.cmd once images partially download."
        }
    } finally {
        $ErrorActionPreference = $prev
    }
}

function Wait-StreamcloneStackReadyWithProgress {
    param(
        [string]$Url = 'http://localhost:8090/',
        [int]$TimeoutSec = 300,
        [int]$IntervalSec = 3
    )
    Set-InstallProgress -Title 'Waiting for services' -Detail 'First install: 3-8 min while images download and frontend becomes healthy.'
    $deadline = (Get-Date).AddSeconds($TimeoutSec)
    while ((Get-Date) -lt $deadline) {
        Set-InstallProgress -Title 'Waiting for services' -Detail (Get-StreamcloneContainerSummary)
        try {
            $resp = Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 5
            if ($resp.StatusCode -ge 200 -and $resp.StatusCode -lt 500) {
                return $true
            }
        } catch { }
        Start-Sleep -Seconds $IntervalSec
    }
    return $false
}

try {
    $InstallDir = (Resolve-Path -LiteralPath $InstallDir).Path
    Set-Location $InstallDir

    Set-InstallProgress -Title 'Checking prerequisites' -Detail 'Verifying Docker Desktop, context, and local ports.'
    $preflightScript = Join-Path $InstallDir 'scripts\preflight-deps.ps1'
    if (-not (Test-Path $preflightScript)) {
        $preflightScript = Join-Path $PSScriptRoot 'preflight-deps.ps1'
    }
    $imageTag = $env:IMAGE_TAG
    if ([string]::IsNullOrWhiteSpace($imageTag) -and (Test-Path (Join-Path $InstallDir 'VERSION'))) {
        $imageTag = (Get-Content (Join-Path $InstallDir 'VERSION') -Raw).Trim()
    }
    if (-not [string]::IsNullOrWhiteSpace($imageTag)) {
        $preflightRaw = & $preflightScript -Quiet -Json -ImageTag $imageTag 2>&1 | Select-Object -Last 1
    } else {
        $preflightRaw = & $preflightScript -Quiet -Json 2>&1 | Select-Object -Last 1
    }
    $preflight = $null
    try {
        $preflight = $preflightRaw | ConvertFrom-Json
    } catch {
        throw "Preflight check failed to produce JSON: $preflightRaw"
    }
    if ($preflight.blocked) {
        $reason = if ($preflight.reason) { [string]$preflight.reason } else { 'Docker prerequisites not met' }
        Set-InstallProgress -Title 'Setup blocked' -Detail $reason -Status "blocked|$reason"
        exit 1
    }

    $setupPs1 = Join-Path $InstallDir 'scripts\setup.ps1'
    $shortcutPs1 = Join-Path $InstallDir 'scripts\install-desktop-shortcut.ps1'
    if (-not (Test-Path $setupPs1)) {
        throw "Missing setup script: $setupPs1"
    }

    $freshInstall = -not (Test-Path (Join-Path $InstallDir '.env'))
    if (-not $freshInstall) {
        if (Test-StreamcloneUpgradeNeeded -Root $InstallDir) {
            $versions = Get-StreamcloneInstallVersions -Root $InstallDir
            Set-InstallProgress -Title 'Updating Streamclone' -Detail "Syncing images to $($versions.bundleVersion)..."
            $onUpgradeLine = {
                param($line)
                if (-not (Test-StreamcloneDockerPullDisplayLine -Line $line)) { return }
                $text = "$line".Trim()
                if ($text -match 'Pulling|Pulled|Pull complete|Download|up to date|Recreat|Starting|Retrying|attempt') {
                    $short = $text
                    if ($short.Length -gt 90) { $short = $short.Substring(0, 87) + '...' }
                    Set-InstallProgress -Title 'Updating Streamclone' -Detail $short
                }
            }
            Invoke-StreamcloneUpgrade -Root $InstallDir -OnLine $onUpgradeLine -PullOutputMode friendly
            if (-not (Wait-StreamcloneStackReadyWithProgress)) { throw 'services did not become ready after upgrade' }
        } else {
            Set-InstallProgress -Title 'Starting Streamclone' -Detail 'Already configured - bringing stack online.'
            $composeArgs = Get-StreamcloneComposeArgs -Root $InstallDir -Profile core -UseImages
            $code = Invoke-EnvDocker -Arguments ($composeArgs + @('up', '-d', '--remove-orphans', '--pull', 'missing'))
            if ($code -ne 0) { throw 'docker compose up failed' }
            if (-not (Wait-StreamcloneStackReadyWithProgress)) { throw 'services did not become ready' }
        }
    } else {
        Set-InstallProgress -Title 'Creating configuration' -Detail 'Generating secrets and .env file.'
        & $setupPs1 -Profile core -NonInteractive -UseImages -NoUp -NoSmoke -SkipPreflight
        if ($LASTEXITCODE -ne 0) { throw 'setup.ps1 failed' }

        $composeArgs = Get-StreamcloneComposeArgs -Root $InstallDir -Profile core -UseImages
        Invoke-DockerComposePullWithProgress -ComposeArgs $composeArgs

        Set-InstallProgress -Title 'Starting containers' -Detail 'Launching Docker services.'
        $code = Invoke-EnvDocker -Arguments ($composeArgs + @('up', '-d', '--remove-orphans', '--pull', 'missing'))
        if ($code -ne 0) { throw 'docker compose up failed' }

        if (-not (Wait-StreamcloneStackReadyWithProgress)) { throw 'services did not become ready' }

        Set-InstallProgress -Title 'Verifying install' -Detail 'Running quick health checks.'
        & powershell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $InstallDir 'scripts\smoke-core.ps1')
        if ($LASTEXITCODE -ne 0) { throw 'smoke checks failed' }
    }

    if (Test-Path $shortcutPs1) {
        Set-InstallProgress -Title 'Adding Desktop shortcut' -Detail 'Streamclone.lnk opens the manager menu.'
        & $shortcutPs1 -InstallDir $InstallDir
    }

    & powershell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $InstallDir 'scripts\ensure-setup-control.ps1') -Root $InstallDir

    if ($env:STREAMCLONE_NO_BROWSER -ne '1') {
        Start-Process 'http://localhost:8090/'
    }
    Write-Host 'Optional Analytics and Clip Studio: open app -> Stack status -> Start Analytics / Clip Studio.' -ForegroundColor DarkGray
    Complete-InstallProgress -ExitCode 0
    exit 0
} catch {
    Set-InstallProgress -Title 'Setup failed' -Detail $_.Exception.Message -Status 'done|1'
    exit 1
}
