#Requires -Version 5.1
param(
    [string]$SetupExe = '',
    [string]$LogFile = '',
    [string]$ImageTag = ''
)

$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent $PSScriptRoot
. (Join-Path $PSScriptRoot 'lib\env.ps1')

if (-not $SetupExe) {
    $version = Get-EnvReleaseVersionTag
    if (-not $version) { $version = 'v0.1.4' }
    $candidate = Join-Path $Root "dist\Streamclone-Setup-$version.exe"
    if (Test-Path $candidate) {
        $SetupExe = $candidate
    } elseif ($version) {
        throw "Setup.exe for $version not found: $candidate. Build it with scripts\build-windows-installer.ps1 before benchmarking this release."
    } else {
        $SetupExe = Join-Path $Root 'dist\Streamclone-Setup-v0.1.3.exe'
    }
}
if (-not $LogFile) {
    $LogFile = Join-Path $Root 'dist\benchmark-exe-install.log'
}
if (-not $ImageTag) {
    $ImageTag = Get-EnvReleaseVersionTag
}

$progressFile = Join-Path $env:TEMP 'streamclone-setup-progress.txt'
$preflightFile = Join-Path $Root 'dist\benchmark-exe-install-preflight.json'
$swTotal = [System.Diagnostics.Stopwatch]::StartNew()
$phases = [System.Collections.Generic.List[object]]::new()
$lastTitle = ''

function Record-Phase {
    param([string]$Title, [string]$Detail)
    if ($Title -eq $lastTitle) { return }
    $script:lastTitle = $Title
    $phases.Add([pscustomobject]@{
        ElapsedSec = [math]::Round($swTotal.Elapsed.TotalSeconds, 1)
        Title      = $Title
        Detail     = $Detail
    })
}

Write-Host "Benchmark: $SetupExe"
Write-Host "Progress:  $progressFile"
Write-Host "Log:       $LogFile"
Write-Host ''

if (-not (Test-Path $SetupExe)) {
    throw "Setup.exe not found: $SetupExe"
}

if ($ImageTag) {
    $preflightRaw = & (Join-Path $PSScriptRoot 'preflight-deps.ps1') -Json -ImageTag $ImageTag 2>&1 | Select-Object -Last 1
} else {
    $preflightRaw = & (Join-Path $PSScriptRoot 'preflight-deps.ps1') -Json 2>&1 | Select-Object -Last 1
}
$preflight = $preflightRaw | ConvertFrom-Json
$preflight | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath $preflightFile -Encoding UTF8
Write-Host "Preflight JSON: $preflightFile"
Write-Host "Preflight blocked: $($preflight.blocked) reason: $($preflight.reason)"
if ($preflight.blocked) {
    Write-Host 'Benchmark aborted — Docker preflight failed. Start Docker Desktop manually; benchmarks never auto-launch it.' -ForegroundColor Red
    exit 2
}

if (Test-Path $progressFile) { Remove-Item $progressFile -Force }
if (Test-Path $LogFile) { Remove-Item $LogFile -Force }

$previousNoBrowser = $env:STREAMCLONE_NO_BROWSER
$env:STREAMCLONE_NO_BROWSER = '1'
$swTotal.Restart()
try {
    $proc = Start-Process -FilePath $SetupExe -ArgumentList @(
        '/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', "/LOG=$LogFile"
    ) -PassThru -WindowStyle Hidden
} finally {
    $env:STREAMCLONE_NO_BROWSER = $previousNoBrowser
}

while (-not $proc.HasExited) {
    if (Test-Path $progressFile) {
        $lines = Get-Content $progressFile -ErrorAction SilentlyContinue
        $title = ($lines | Where-Object { $_ -like 'TITLE=*' } | Select-Object -Last 1) -replace '^TITLE=', ''
        $detail = ($lines | Where-Object { $_ -like 'DETAIL=*' } | Select-Object -Last 1) -replace '^DETAIL=', ''
        $status = ($lines | Where-Object { $_ -like 'STATUS=*' } | Select-Object -Last 1) -replace '^STATUS=', ''
        if ($title) { Record-Phase -Title $title -Detail $detail }
        if ($status -like 'blocked|*') { break }
        if ($status -like 'done|*') { break }
    }
    Start-Sleep -Milliseconds 400
}
$proc.WaitForExit()
$totalSec = [math]::Round($swTotal.Elapsed.TotalSeconds, 1)

$welcomeOk = $false
try {
    $r = Invoke-WebRequest -Uri 'http://localhost:8090/' -UseBasicParsing -TimeoutSec 10
    $welcomeOk = ($r.StatusCode -eq 200)
} catch { }

$containerResult = Invoke-EnvDockerCaptured -Arguments @('ps', '--filter', 'name=streamclone', '--format', '{{.Names}}: {{.Status}}')
$containers = if ($containerResult.ExitCode -eq 0) { $containerResult.Output -join '; ' } else { 'Docker status unavailable' }

Write-Host "=== Streamclone Setup.exe benchmark ===" -ForegroundColor Cyan
Write-Host "Exit code:       $($proc.ExitCode)"
Write-Host "Total time:      ${totalSec}s"
Write-Host "Install folder:  $(Test-Path (Join-Path $env:USERPROFILE 'streamclone'))"
Write-Host "Directory HTTP:  $(if ($welcomeOk) { '200 OK' } else { 'FAILED' })"
Write-Host ''
Write-Host 'Phases:' -ForegroundColor Cyan
$phases | Format-Table -AutoSize
Write-Host "Containers: $containers"

if ($proc.ExitCode -ne 0) { exit $proc.ExitCode }
