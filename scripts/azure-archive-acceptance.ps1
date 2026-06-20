# Mode B Azure archive plane — bronze/coverage polling via SSH + docker on the VM.
# Assumes Mode B compose is up on azure-streamclone with profile-archive + profile-azure-workers.
#
# Usage:
#   pwsh scripts/azure-archive-acceptance.ps1 -SshHost 203.0.113.10 -SshKey $env:USERPROFILE\.ssh\id_ed25519_streamclone
#   pwsh scripts/azure-archive-acceptance.ps1 -Stage acceptance -SshHost azure-streamclone -Duration 24h
#   pwsh scripts/azure-archive-acceptance.ps1 -Stage smoke -Duration 5m -PollInterval 1m -DryRun

param(
    [ValidateSet('smoke', 'acceptance')][string]$Stage = 'smoke',
    [string]$SshHost = '',
    [string]$SshUser = 'streamclone',
    [string]$SshKey = '',
    [string]$RemoteRepoPath = '~/streamclone-src',
    [string]$Duration = '',
    [string]$PollInterval = '',
    [int]$DurationMinutes = 0,
    [int]$PollMinutes = 0,
    [string]$DockerNetwork = 'streamclone-azure-archive-plane_default',
    [switch]$DryRun
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
Set-Location $repoRoot
. (Join-Path $PSScriptRoot 'lib\env.ps1')

if (-not $SshHost) {
    throw 'SshHost is required (Azure VM public IP or resolvable SSH hostname).'
}

if (-not $SshKey) {
    foreach ($candidate in @(
            (Join-Path $env:USERPROFILE '.ssh\id_ed25519_streamclone'),
            (Join-Path $env:USERPROFILE '.ssh\id_ed25519')
        )) {
        if (Test-Path $candidate) {
            $SshKey = $candidate
            break
        }
    }
}
if (-not $SshKey -or -not (Test-Path $SshKey)) {
    throw 'SSH private key not found. Pass -SshKey or create id_ed25519_streamclone under %USERPROFILE%\.ssh\.'
}

if (-not (Get-Command ssh -ErrorAction SilentlyContinue)) {
    throw 'OpenSSH client (ssh) is required on PATH.'
}
if (-not (Get-Command scp -ErrorAction SilentlyContinue)) {
    throw 'OpenSSH client (scp) is required on PATH.'
}

if (-not $Duration) {
    if ($DurationMinutes -gt 0) {
        $Duration = "${DurationMinutes}m"
    } else {
        $Duration = if ($Stage -eq 'acceptance') { '24h' } else { '6h' }
    }
}
if (-not $PollInterval) {
    if ($PollMinutes -gt 0) {
        $PollInterval = "${PollMinutes}m"
    } else {
        $PollInterval = if ($Duration -match '^(5m|10m|15m)$') { '1m' } else { '30m' }
    }
}

function Parse-Duration {
    param([string]$Text)
    $Text = $Text.Trim().ToLower()
    if ($Text -match '^(\d+)(m|h)$') {
        $n = [int]$matches[1]
        switch ($matches[2]) {
            'm' { return [TimeSpan]::FromMinutes($n) }
            'h' { return [TimeSpan]::FromHours($n) }
        }
    }
    throw "Invalid duration '$Text' (use e.g. 5m, 6h, 24h)"
}

function Get-SshTarget {
    return "${SshUser}@${SshHost}"
}

function Get-SshBaseArgs {
    return @('-o', 'BatchMode=yes', '-o', 'ConnectTimeout=15', '-i', $SshKey)
}

function Invoke-SshCommand {
    param([string]$RemoteCommand)
    $target = Get-SshTarget
    $args = Get-SshBaseArgs + @($target, $RemoteCommand)
    $result = & ssh @args 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw "SSH command failed (exit $LASTEXITCODE): $RemoteCommand`n$result"
    }
    return ($result | Out-String).TrimEnd()
}

function Invoke-RemoteBackfill {
    param([string[]]$BackfillArgs)
    $escapedArgs = ($BackfillArgs | ForEach-Object { "'$($_ -replace "'", "'\\''")'" }) -join ' '
    $remoteCmd = @"
set -euo pipefail
cd ${RemoteRepoPath}
docker run --rm \
  --network ${DockerNetwork} \
  -e DATABASE_URL=postgres://app:app@postgres:5432/streamclone?sslmode=disable \
  -e REDIS_URL=redis://redis:6379/0 \
  -v "`$(pwd)":/src:ro \
  -w /src \
  golang:1.25-alpine \
  go run ./cmd/backfill ${escapedArgs}
"@
    Invoke-SshCommand -RemoteCommand $remoteCmd
}

function Copy-RemoteCoverageFile {
    param(
        [string]$RemotePath,
        [string]$LocalPath
    )
    $target = Get-SshTarget
    $args = Get-SshBaseArgs + @("${target}:${RemotePath}", $LocalPath)
    & scp @args 2>&1 | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "scp failed for ${RemotePath}"
    }
}

function Test-RemoteModeBStack {
    Write-Host 'Checking Mode B stack on VM...' -ForegroundColor Cyan
    $psCmd = @"
set -euo pipefail
cd ${RemoteRepoPath}
docker ps --filter name=streamclone-analytics-workers --format '{{.Names}} {{.Status}}'
docker ps --filter name=streamclone-scraper --format '{{.Names}} {{.Status}}'
"@
    $out = Invoke-SshCommand -RemoteCommand $psCmd
    Write-Host $out
    if ($out -notmatch 'streamclone-analytics-workers') {
        throw 'streamclone-analytics-workers not running on VM — bring up Mode B compose first.'
    }
}

function Write-PollSnapshot {
    param(
        [string]$Label,
        [datetime]$SinceUtc,
        [string]$OutDir
    )
    $stamp = (Get-Date).ToUniversalTime().ToString('yyyyMMdd-HHmmss')
    Write-Host "`n=== $Label ($stamp UTC) ===" -ForegroundColor Cyan

    $bronzeStatus = Invoke-RemoteBackfill @('bronze', 'status')
    Write-Host $bronzeStatus

    $backfillStatus = Invoke-RemoteBackfill @('status')
    Write-Host $backfillStatus

    $since = $SinceUtc.ToString('yyyy-MM-ddTHH:mm:ssZ')
    $remoteOut = "/tmp/coverage-azure-$Stage-$stamp.json"
    Invoke-RemoteBackfill @('coverage', 'report', "--since=$since", "--out=$remoteOut") | Out-Null

    $localOut = Join-Path $OutDir "coverage-azure-archive-plane-$Stage-$stamp.json"
    Copy-RemoteCoverageFile -RemotePath $remoteOut -LocalPath $localOut
    Invoke-SshCommand -RemoteCommand "rm -f '$remoteOut'" | Out-Null
    Write-Host "Snapshot saved: $localOut" -ForegroundColor DarkGray
}

$durationSpan = Parse-Duration $Duration
$pollSpan = Parse-Duration $PollInterval
$benchDir = Join-Path $repoRoot 'docs\benchmarks'
New-Item -ItemType Directory -Force -Path $benchDir | Out-Null
$reportMd = Join-Path $benchDir "azure-archive-plane-acceptance-$Stage-$(Get-Date -Format 'yyyy-MM-dd').md"

Write-Host "Azure archive acceptance ($Stage)" -ForegroundColor Green
Write-Host "  ssh: $(Get-SshTarget)"
Write-Host "  remote repo: $RemoteRepoPath"
Write-Host "  duration: $Duration | poll: $PollInterval"
Write-Host "  bronze overlay: profile-bronze-$Stage.env (apply on VM before long runs)"
Write-Host "  results: $reportMd"

if ($DryRun) {
    Write-Host '[dry-run] Skipping SSH poll loop.' -ForegroundColor Yellow
    exit 0
}

Test-RemoteModeBStack

$startUtc = (Get-Date).ToUniversalTime()
$endUtc = $startUtc.Add($durationSpan)
@(
    "# Azure archive plane acceptance ($Stage)",
    '',
    "- Started: $($startUtc.ToString('o'))",
    "- Duration: $Duration",
    "- SSH host: $SshHost",
    "- Remote repo: $RemoteRepoPath",
    "- Stage overlay: profile-bronze-$Stage.env",
    ''
) | Set-Content -Path $reportMd -Encoding utf8

Write-PollSnapshot -Label 'initial' -SinceUtc $startUtc -OutDir $benchDir

while ((Get-Date).ToUniversalTime() -lt $endUtc) {
    $remaining = $endUtc - (Get-Date).ToUniversalTime()
    $sleepFor = if ($remaining -lt $pollSpan) { $remaining } else { $pollSpan }
    if ($sleepFor.TotalSeconds -le 0) { break }
    Write-Host "Sleeping $($sleepFor.ToString('g')) until next poll..." -ForegroundColor DarkGray
    Start-Sleep -Seconds ([int][Math]::Ceiling($sleepFor.TotalSeconds))
    if ((Get-Date).ToUniversalTime() -ge $endUtc) { break }
    Write-PollSnapshot -Label 'poll' -SinceUtc $startUtc -OutDir $benchDir
}

Write-PollSnapshot -Label 'final' -SinceUtc $startUtc -OutDir $benchDir

Add-Content -Path $reportMd -Value @(
    '',
    "- Finished: $((Get-Date).ToUniversalTime().ToString('o'))",
    "- See docs/benchmarks/coverage-azure-archive-plane-$Stage-*.json for snapshots.",
    ''
)
Write-Host "`nAzure acceptance run complete. Summary: $reportMd" -ForegroundColor Green
