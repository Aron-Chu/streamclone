# Bronze / Tier-0 acceptance run: merge archive env, rebuild analytics, poll status + coverage.
param(
    [ValidateSet('smoke', 'acceptance')][string]$Stage = 'smoke',
    [string]$Duration = '',
    [string]$PollInterval = '',
    [int]$DurationMinutes = 0,
    [int]$PollMinutes = 0,
    [switch]$SkipRebuild,
    [switch]$DryRun
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
Set-Location $repoRoot
. (Join-Path $PSScriptRoot 'lib\env.ps1')
. (Join-Path $PSScriptRoot 'lib\stack-progress.ps1')

$stageOverlay = Join-Path $repoRoot "deploy\env\profile-bronze-$Stage.env"
$sources = @(
    (Join-Path $repoRoot 'deploy\env\profile-archive.env'),
    $stageOverlay,
    (Join-Path $repoRoot '.env.local'),
    (Join-Path $repoRoot '.env')
)
$runtimeEnv = Join-Path $repoRoot 'runtime\bronze-acceptance.env'
New-Item -ItemType Directory -Force -Path (Split-Path $runtimeEnv) | Out-Null
Merge-EnvFiles -OutFile $runtimeEnv -Sources $sources

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

function Read-MergedEnv {
    param([string]$Path)
    $vals = Read-EnvKeyValueFile -Path $Path
    foreach ($key in @('DATABASE_URL', 'ARCHIVE_AZURE_CONNECTION_STRING_FILE')) {
        if (-not $vals.ContainsKey($key) -or [string]::IsNullOrWhiteSpace([string]$vals[$key])) {
            Write-Warning "Missing $key in merged env ($Path). Coverage report may fail."
        }
    }
    return $vals
}

function Invoke-BackfillCLI {
    param([string[]]$Args)
    $envBackup = @{}
    foreach ($entry in (Read-EnvKeyValueFile -Path $runtimeEnv).GetEnumerator()) {
        $envBackup[$entry.Key] = [Environment]::GetEnvironmentVariable($entry.Key)
        Set-Item -Path "Env:$($entry.Key)" -Value $entry.Value
    }
    try {
        $go = Get-Command go -ErrorAction SilentlyContinue
        if ($go) {
            & go run ./cmd/backfill @Args
            if ($LASTEXITCODE -ne 0) { throw "backfill CLI failed: $Args" }
            return
        }
        $dockerArgs = @(
            'run', '--rm',
            '-v', "${repoRoot}:/src",
            '-w', '/src',
            '--env-file', $runtimeEnv,
            'golang:1.25-alpine',
            'go', 'run', './cmd/backfill'
        ) + $Args
        $code = Invoke-EnvDocker -Arguments $dockerArgs
        if ($code -ne 0) { throw "backfill CLI failed (docker): $Args" }
    } finally {
        foreach ($entry in $envBackup.GetEnumerator()) {
            if ($null -eq $entry.Value) {
                Remove-Item -Path "Env:$($entry.Key)" -ErrorAction SilentlyContinue
            } else {
                Set-Item -Path "Env:$($entry.Key)" -Value $entry.Value
            }
        }
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
    Invoke-BackfillCLI @('bronze', 'status')
    Invoke-BackfillCLI @('status')
    $since = $SinceUtc.ToString('yyyy-MM-ddTHH:mm:ssZ')
    $outFile = Join-Path $OutDir "coverage-$Stage-$stamp.json"
    Invoke-BackfillCLI @('coverage', 'report', "--since=$since", "--out=$outFile")
    Write-Host "Snapshot saved: $outFile" -ForegroundColor DarkGray
}

$durationSpan = Parse-Duration $Duration
$pollSpan = Parse-Duration $PollInterval
$benchDir = Join-Path $repoRoot 'docs\benchmarks'
New-Item -ItemType Directory -Force -Path $benchDir | Out-Null
$reportMd = Join-Path $benchDir "bronze-acceptance-$Stage-$(Get-Date -Format 'yyyy-MM-dd').md"

Write-Host "Bronze acceptance ($Stage)" -ForegroundColor Green
Write-Host "  overlay: $stageOverlay"
Write-Host "  merged env: $runtimeEnv"
Write-Host "  duration: $Duration | poll: $PollInterval"
Write-Host "  results: $reportMd"

$merged = Read-MergedEnv -Path $runtimeEnv
if ($DryRun) {
    Write-Host '[dry-run] Skipping compose rebuild and poll loop.' -ForegroundColor Yellow
    exit 0
}

if (-not $SkipRebuild) {
    $composeArgs = Get-StreamcloneComposeArgs -Root $repoRoot
    Write-Host 'Building analytics image...' -ForegroundColor Cyan
    $buildCode = Invoke-EnvDocker -Arguments ($composeArgs + @('build', 'analytics'))
    if ($buildCode -ne 0) { throw 'docker compose build analytics failed' }
    Write-Host 'Recreating analytics container...' -ForegroundColor Cyan
    $upCode = Invoke-EnvDocker -Arguments ($composeArgs + @('up', '-d', '--no-deps', '--force-recreate', 'analytics'))
    if ($upCode -ne 0) { throw 'docker compose up analytics failed' }
}

$startUtc = (Get-Date).ToUniversalTime()
$endUtc = $startUtc.Add($durationSpan)
"# Bronze acceptance ($Stage)`n`n- Started: $($startUtc.ToString('o'))`n- Duration: $Duration`n- Stage overlay: profile-bronze-$Stage.env`n" | Set-Content -Path $reportMd -Encoding utf8

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

$connFile = [string]$merged['ARCHIVE_AZURE_CONNECTION_STRING_FILE']
if (-not [string]::IsNullOrWhiteSpace($connFile) -and (Test-Path $connFile)) {
    Write-Host "`nAzure blob prefix summary (read-only):" -ForegroundColor Cyan
    try {
        $az = Get-Command az -ErrorAction SilentlyContinue
        if ($az) {
            $conn = (Get-Content -Raw $connFile).Trim()
            az storage blob list --connection-string $conn --container-name streamclone-archive --prefix streamclone/bronze --num-results 20 --output table
        } else {
            Write-Warning 'Azure CLI (az) not installed; skip blob list.'
        }
    } catch {
        Write-Warning "Azure blob list failed: $_"
    }
}

Add-Content -Path $reportMd -Value "`n- Finished: $((Get-Date).ToUniversalTime().ToString('o'))`n- See docs/benchmarks/coverage-$Stage-*.json for snapshots.`n"
Write-Host "`nAcceptance run complete. Summary: $reportMd" -ForegroundColor Green
