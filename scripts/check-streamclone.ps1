#Requires -Version 5.1
# Quick diagnostic: Docker, install dir, containers, and web UI reachability.
param(
    [string]$InstallDir = (Join-Path $env:USERPROFILE 'streamclone'),
    [switch]$Json
)

$ErrorActionPreference = 'Continue'
. (Join-Path $PSScriptRoot 'lib\diagnostics.ps1')

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

$report = Get-StreamcloneDiagnostics -Root $InstallDir

if (-not $Json) {
    Write-Host ''
    Write-Host 'Streamclone status check' -ForegroundColor Cyan
    Write-Host '------------------------'

    $versionParts = @()
    if ($report.bundleVersion) { $versionParts += "Bundle $($report.bundleVersion)" }
    if ($report.imageTag) { $versionParts += "Images $($report.imageTag)" }
    if ($report.latestRelease) { $versionParts += "Latest $($report.latestRelease)" }
    if ($versionParts.Count -gt 0) {
        $versionLine = $versionParts -join ' | '
        if ($report.upgradeNeeded) {
            $versionLine += ' (update available)'
            Write-Host $versionLine -ForegroundColor Yellow
        } else {
            Write-Host $versionLine -ForegroundColor DarkGray
        }
    }

    if ($report.coreImages) {
        $ci = $report.coreImages
        $coreLabel = "$($ci.present)/$($ci.total) core images downloaded"
        if ($ci.present -eq $ci.total) {
            Write-StatusLine 'Core images' 'ok' $coreLabel
        } else {
            Write-StatusLine 'Core images' 'warn' $coreLabel
        }
    }
    Write-Host ''

    if (Test-Path (Join-Path $InstallDir 'VERSION')) {
        $version = (Get-Content (Join-Path $InstallDir 'VERSION') -Raw).Trim()
        Write-StatusLine 'Install folder' 'ok' "$InstallDir ($version)"
    } elseif (Test-Path $InstallDir) {
        Write-StatusLine 'Install folder' 'warn' "$InstallDir (VERSION file missing - incomplete install?)"
    } else {
        Write-StatusLine 'Install folder' 'fail' "Not found at $InstallDir"
    }

    if ($report.configReady) {
        Write-StatusLine 'Configuration' 'ok' '.env present'
    } else {
        Write-StatusLine 'Configuration' 'warn' '.env missing - setup did not finish'
    }

    switch ($report.docker) {
        'running' { Write-StatusLine 'Docker' 'ok' 'Engine running' }
        'missing' { Write-StatusLine 'Docker' 'fail' 'Not installed or not on PATH' }
        'timeout' { Write-StatusLine 'Docker' 'fail' 'Engine not responding (timed out)' }
        default { Write-StatusLine 'Docker' 'fail' 'Installed but engine is not running' }
    }

    if ($report.docker -eq 'running') {
        $running = @($report.containers | Where-Object { $_.status -match '^Up' })
        if ($running.Count -gt 0) {
            Write-StatusLine 'Containers' 'ok' ("$($running.Count) running / $($report.containers.Count) total")
            foreach ($c in $report.containers) {
                $color = if ($c.status -match 'healthy|Up') { 'Green' } elseif ($c.status -match 'Exit|unhealthy') { 'Red' } else { 'Yellow' }
                Write-Host ("  {0} : {1}" -f $c.name, $c.status) -ForegroundColor $color
            }
        } else {
            Write-StatusLine 'Containers' 'warn' 'None running (stack stopped or never started)'
        }
    }

    if ($report.webOk) {
        Write-StatusLine 'Web UI' 'ok' $report.webUrl
    } else {
        Write-StatusLine 'Web UI' 'fail' "Not reachable at $($report.webUrl)"
    }

    if ($null -ne $report.hlsProxyConfigOk) {
        if ($report.hlsProxyConfigOk) {
            Write-StatusLine 'HLS proxy' 'ok' 'Caddy injects MediaMTX CDN bearer on /live/*'
        } else {
            Write-StatusLine 'HLS proxy' 'warn' 'Outdated Caddyfile.local-tunnel — update install bundle'
        }
    }

    if ($report.setupControl -and $report.setupControlProxy) {
        Write-StatusLine 'Setup helper' 'ok' 'Ready for Start Analytics / Clip Studio / Pulse (app proxy + port 9191)'
    } elseif ($report.setupControl) {
        Write-StatusLine 'Setup helper' 'warn' 'Running on port 9191, but app proxy cannot reach it'
    } else {
        Write-StatusLine 'Setup helper' 'warn' 'Not running - Start Analytics / Pulse buttons will not work'
    }

    Write-Host ''
    if ($report.healthy) {
        Write-Host ("Streamclone looks healthy. Open {0}" -f (Get-StreamcloneAppUrl)) -ForegroundColor Green
    } else {
        Write-Host 'Streamclone is not fully ready.' -ForegroundColor Yellow
        if ($report.suggestions.Count -gt 0) {
            Write-Host 'Suggested fixes:'
            foreach ($s in $report.suggestions) {
                Write-Host "  - $s"
            }
        }
    }
    Write-Host ''
} else {
    $report | ConvertTo-Json -Depth 6
}

if ($report.healthy) { exit 0 }
exit 1
