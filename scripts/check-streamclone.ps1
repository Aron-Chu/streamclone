#Requires -Version 5.1
# Quick diagnostic: Docker, install dir, containers, and web UI reachability.
param(
    [string]$InstallDir = (Join-Path $env:USERPROFILE 'streamclone'),
    [switch]$Json
)

$ErrorActionPreference = 'Continue'
. (Join-Path $PSScriptRoot 'lib\env.ps1')

$report = [ordered]@{
    installDir   = $InstallDir
    version      = ''
    configReady  = $false
    docker       = 'unknown'
    webOk        = $false
    webUrl       = 'http://localhost:8090/'
    containers   = @()
    healthy      = $false
}
$suggestions = [System.Collections.Generic.List[string]]::new()

function Write-StatusLine {
    param([string]$Label, [string]$Status, [string]$Detail = '')
    if ($Json) { return }
    $color = switch ($Status) {
        'ok' { 'Green' }
        'warn' { 'Yellow' }
        'fail' { 'Red' }
        default { 'Gray' }
    }
    $suffix = if ($Detail) { " - $Detail" } else { '' }
    Write-Host ("[{0}] {1}{2}" -f $Status.ToUpper(), $Label, $suffix) -ForegroundColor $color
}

if (-not $Json) {
    Write-Host ''
    Write-Host 'Streamclone status check' -ForegroundColor Cyan
    Write-Host '------------------------'
}

$versionFile = Join-Path $InstallDir 'VERSION'
if (Test-Path $versionFile) {
    $report.version = (Get-Content $versionFile -Raw).Trim()
    Write-StatusLine 'Install folder' 'ok' "$InstallDir ($($report.version))"
} elseif (Test-Path $InstallDir) {
    Write-StatusLine 'Install folder' 'warn' "$InstallDir (VERSION file missing - incomplete install?)"
    [void]$suggestions.Add('Re-run Install Streamclone.cmd or Streamclone-Setup.exe.')
} else {
    Write-StatusLine 'Install folder' 'fail' "Not found at $InstallDir"
    [void]$suggestions.Add('Run Install Streamclone.cmd from the latest release.')
}

$envFile = Join-Path $InstallDir '.env'
$report.configReady = Test-Path $envFile
if ($report.configReady) {
    Write-StatusLine 'Configuration' 'ok' '.env present'
} else {
    Write-StatusLine 'Configuration' 'warn' '.env missing - setup did not finish'
    [void]$suggestions.Add('Run setup: powershell -File scripts\setup.ps1 -Profile core -NonInteractive -UseImages')
}

$docker = Get-EnvDockerExe
if (-not $docker) {
    $report.docker = 'missing'
    Write-StatusLine 'Docker' 'fail' 'Not installed or not on PATH'
    [void]$suggestions.Add('Install Docker Desktop and ensure it is running.')
} else {
    $info = Invoke-EnvDockerCapturedWithTimeout -Arguments @('info') -TimeoutSec 15
    if ($info.TimedOut) {
        $report.docker = 'timeout'
        Write-StatusLine 'Docker' 'fail' 'Engine not responding (timed out)'
        [void]$suggestions.Add('Start Docker Desktop and wait until it shows Running.')
    } elseif ($info.ExitCode -ne 0) {
        $report.docker = 'stopped'
        Write-StatusLine 'Docker' 'fail' 'Installed but engine is not running'
        [void]$suggestions.Add('Open Docker Desktop and wait until the whale icon is steady.')
    } else {
        $report.docker = 'running'
        Write-StatusLine 'Docker' 'ok' 'Engine running'
    }
}

if ($report.docker -eq 'running') {
    $ps = Invoke-EnvDockerCaptured -Arguments @('ps', '-a', '--filter', 'name=streamclone', '--format', '{{.Names}}|{{.Status}}')
    if ($ps.ExitCode -eq 0 -and $ps.Output) {
        foreach ($line in $ps.Output) {
            $parts = $line -split '\|', 2
            $report.containers += [pscustomobject]@{
                name   = $parts[0]
                status = $parts[1]
            }
        }
        $running = @($report.containers | Where-Object { $_.status -match '^Up' })
        if ($running.Count -gt 0) {
            Write-StatusLine 'Containers' 'ok' ("$($running.Count) running / $($report.containers.Count) total")
            if (-not $Json) {
                foreach ($c in $report.containers) {
                    $color = if ($c.status -match 'healthy|Up') { 'Green' } elseif ($c.status -match 'Exit|unhealthy') { 'Red' } else { 'Yellow' }
                    Write-Host ("  {0} : {1}" -f ($c.name -replace '^streamclone-', ''), $c.status) -ForegroundColor $color
                }
            }
        } else {
            Write-StatusLine 'Containers' 'warn' 'None running (stack stopped or never started)'
            [void]$suggestions.Add('Use Start Streamclone.cmd or scripts\start-streamclone.ps1')
        }
    } else {
        Write-StatusLine 'Containers' 'warn' 'No streamclone containers found'
        [void]$suggestions.Add('Run Install Streamclone.cmd or Start Streamclone.cmd.')
    }
}

try {
    $resp = Invoke-WebRequest -Uri $report.webUrl -UseBasicParsing -TimeoutSec 5
    if ($resp.StatusCode -ge 200 -and $resp.StatusCode -lt 500) {
        $report.webOk = $true
        Write-StatusLine 'Web UI' 'ok' $report.webUrl
    } else {
        Write-StatusLine 'Web UI' 'warn' "HTTP $($resp.StatusCode) at $($report.webUrl)"
    }
} catch {
    Write-StatusLine 'Web UI' 'fail' "Not reachable at $($report.webUrl)"
    if ($report.docker -eq 'running' -and $report.containers.Count -gt 0) {
        [void]$suggestions.Add('Containers exist but UI is not up yet - wait 1-2 min or check: docker compose logs caddy frontend')
    }
}

$report.healthy = ($report.docker -eq 'running') -and $report.webOk -and ($report.configReady -or ($report.containers.Count -gt 0))

function Test-SetupControlHealth {
    try {
        $resp = Invoke-WebRequest -Uri 'http://127.0.0.1:9191/health' -UseBasicParsing -TimeoutSec 2
        if ($resp.StatusCode -eq 200) {
            $body = $resp.Content | ConvertFrom-Json
            return [bool]$body.ok
        }
    } catch { }
    return $false
}

$setupControlOk = Test-SetupControlHealth
if ($setupControlOk) {
    Write-StatusLine 'Setup helper' 'ok' 'Ready for Start Analytics / Clip Studio (port 9191)'
} else {
    Write-StatusLine 'Setup helper' 'warn' 'Not running - Start Analytics button will not work'
    [void]$suggestions.Add('Run Start Streamclone.cmd once, or: powershell -File scripts\ensure-setup-control.ps1')
}

if ($Json) {
    $report['setupControl'] = $setupControlOk
    $report['suggestions'] = @($suggestions)
    $report | ConvertTo-Json -Depth 4
} else {
    Write-Host ''
    if ($report.healthy) {
        Write-Host 'Streamclone looks healthy. Open http://localhost:8090/' -ForegroundColor Green
        if (-not $report.configReady) {
            Write-Host 'Note: .env is missing but containers are still running from a prior install.' -ForegroundColor DarkYellow
            Write-Host 'Use Start Streamclone next time. Re-run Install only if you need to upgrade or reset.' -ForegroundColor DarkGray
        }
    } else {
        Write-Host 'Streamclone is not fully ready.' -ForegroundColor Yellow
        if ($suggestions.Count -gt 0) {
            Write-Host 'Suggested fixes:'
            foreach ($s in $suggestions) {
                Write-Host "  - $s"
            }
        }
    }
    Write-Host ''
}

if ($report.healthy) { exit 0 }
exit 1
