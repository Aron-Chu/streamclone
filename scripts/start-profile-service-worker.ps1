#Requires -Version 5.1
# Queued optional-service compose start (runs when another compose up holds the lock).
param(
    [Parameter(Mandatory = $true)]
    [ValidateSet('scraper', 'clipper')]
    [string]$Service,
    [string]$Root = ''
)

$ErrorActionPreference = 'Stop'
if ([string]::IsNullOrWhiteSpace($Root)) {
    $Root = Split-Path -Parent $PSScriptRoot
}

. (Join-Path $PSScriptRoot 'lib\env.ps1')
. (Join-Path $PSScriptRoot 'lib\stack-progress.ps1')

$composeLockPath = Join-Path $Root '.streamclone-compose.lock'
$queuePath = Join-Path $Root '.streamclone-start-queue.json'

function Get-ComposeLock {
    if (-not (Test-Path $composeLockPath)) { return $null }
    try { return (Get-Content -LiteralPath $composeLockPath -Raw | ConvertFrom-Json) } catch { return $null }
}

function Wait-ComposeLockReleased {
    param([int]$TimeoutSec = 3600)
    $deadline = (Get-Date).AddSeconds($TimeoutSec)
    while ((Get-Date) -lt $deadline) {
        $lock = Get-ComposeLock
        if (-not $lock) { return $true }
        if ($lock.pid) {
            $proc = Get-Process -Id ([int]$lock.pid) -ErrorAction SilentlyContinue
            if (-not $proc) {
                Remove-Item -LiteralPath $composeLockPath -Force -ErrorAction SilentlyContinue
                return $true
            }
        }
        Start-Sleep -Seconds 2
    }
    return $false
}

function Start-QueuedServiceCompose {
    Set-Location $Root
    $envPath = Join-Path $Root '.env'
    if (-not (Test-Path $envPath)) { throw 'Missing .env' }

    if ($Service -eq 'scraper') {
        if (-not (Test-ScraperSiblingRepoReady -Root $Root)) {
            $sibling = Get-EnvScraperSiblingPath
            $parent = Split-Path -Parent $sibling
            New-Item -ItemType Directory -Path $parent -Force | Out-Null
            $clone = Invoke-EnvCapturedProcess -FilePath 'git' -ArgumentList @(
                'clone', 'https://github.com/Aron-Chu/streamclone-scraper.git', $sibling
            ) -TimeoutSec 300
            if ($clone.ExitCode -ne 0) {
                throw "Could not clone streamclone-scraper: $(($clone.Output -join ' '))"
            }
        }
    }

    $envValues = Read-EnvKeyValueFile -Path $envPath
    $useImages = ($envValues['SCRAPER_USE_IMAGES'] -eq '1') -or
        ($envValues['STREAMCLONE_USE_IMAGES'] -eq '1') -or
        (-not [string]::IsNullOrWhiteSpace($envValues['IMAGE_TAG']))

    $sourceBuild = ($Service -eq 'scraper') -and (Test-ScraperBuildFromSource -Root $Root)
    $composeArgs = Get-StreamcloneComposeArgs -Root $Root -Profile $Service -UseImages:$useImages -ScraperSourceBuild:$sourceBuild
    $docker = Get-EnvDockerExe
    if (-not $docker) { throw 'Docker not found' }

    $logFile = Join-Path $Root ".streamclone-start-$Service.log"
    $errLog = "${logFile}.err"
    foreach ($path in @($logFile, $errLog)) {
        if (Test-Path $path) { Remove-Item -LiteralPath $path -Force -ErrorAction SilentlyContinue }
    }

    $args = $composeArgs + @('up', '-d', '--remove-orphans')
    if ($useImages -and -not $sourceBuild) { $args += '--pull', 'missing' }
    if ($sourceBuild) { $args += '--build' }
    $args += $Service

    $proc = Start-Process -FilePath $docker `
        -ArgumentList (Join-EnvProcessArguments -Arguments $args) `
        -WorkingDirectory $Root `
        -WindowStyle Hidden `
        -RedirectStandardOutput $logFile `
        -RedirectStandardError $errLog `
        -PassThru

    Set-Content -LiteralPath $composeLockPath -Value (@{
        service = $Service
        pid     = $proc.Id
        started = (Get-Date).ToString('o')
    } | ConvertTo-Json -Compress) -Encoding UTF8

    try {
        Wait-Process -Id $proc.Id -ErrorAction SilentlyContinue
    } catch { }
    Remove-Item -LiteralPath $composeLockPath -Force -ErrorAction SilentlyContinue
}

if (-not (Wait-ComposeLockReleased)) {
    exit 1
}

try {
    Start-QueuedServiceCompose
} finally {
    if (Test-Path $queuePath) {
        try {
            $queue = @(Get-Content -LiteralPath $queuePath -Raw | ConvertFrom-Json)
            if ($queue.Count -gt 0) {
                $next = $queue[0]
                $rest = @($queue | Select-Object -Skip 1)
                if ($rest.Count -gt 0) {
                    Set-Content -LiteralPath $queuePath -Value ($rest | ConvertTo-Json -Compress) -Encoding UTF8
                } else {
                    Remove-Item -LiteralPath $queuePath -Force -ErrorAction SilentlyContinue
                }
                $psExe = if ($PSVersionTable.PSEdition -eq 'Core') { 'pwsh.exe' } else { 'powershell.exe' }
                Start-Process -FilePath $psExe -ArgumentList @(
                    '-NoProfile', '-ExecutionPolicy', 'Bypass',
                    '-File', (Join-Path $PSScriptRoot 'start-profile-service-worker.ps1'),
                    '-Service', [string]$next.service,
                    '-Root', $Root
                ) -WindowStyle Hidden | Out-Null
            }
        } catch { }
    }
}
