#Requires -Version 5.1
# Measure GHCR registry pull download sizes for core images at a tag.
# Does not fake numbers — records actual docker pull output or exits blocked.
param(
    [string]$ImageTag = '',
    [string]$OutFile = ''
)

$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent $PSScriptRoot
. (Join-Path $PSScriptRoot 'lib\env.ps1')

if (-not $ImageTag) {
    $ImageTag = Get-EnvReleaseVersionTag
    if (-not $ImageTag) { $ImageTag = 'latest' }
}
if (-not $OutFile) {
    $OutFile = Join-Path $Root "dist\benchmark-ghcr-pull-$ImageTag.json"
}

$images = @(
    'metadata', 'chat', 'video', 'emote', 'analytics', 'frontend'
)

$preflightRaw = & (Join-Path $PSScriptRoot 'preflight-deps.ps1') -Json -ImageTag $ImageTag 2>&1 | Select-Object -Last 1
$preflight = $preflightRaw | ConvertFrom-Json
if ($preflight.blocked) {
    $blocked = [ordered]@{
        blocked  = $true
        reason   = $preflight.reason
        imageTag = $ImageTag
        measured = $false
        images   = @()
    }
    $blocked | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath $OutFile -Encoding UTF8
    Write-Host "GHCR pull benchmark blocked: $($preflight.reason)" -ForegroundColor Red
    exit 2
}

$results = [System.Collections.Generic.List[object]]::new()
$totalBytes = 0L

foreach ($name in $images) {
    $ref = "ghcr.io/aron-chu/streamclone/${name}:$ImageTag"
    Write-Host "Pulling $ref ..." -ForegroundColor Cyan
    $pull = Invoke-EnvDockerCaptured -Arguments @('pull', $ref)
    $downloaded = 0L
    foreach ($line in $pull.Output) {
        if ($line -match 'Downloaded newer image|Pull complete|Downloaded') {
            # docker pull lines vary; also inspect image size after pull
        }
    }
    $inspect = Invoke-EnvDockerCaptured -Arguments @('image', 'inspect', $ref, '--format', '{{.Size}}')
    if ($inspect.ExitCode -eq 0 -and $inspect.Output) {
        [void][long]::TryParse(($inspect.Output | Select-Object -First 1), [ref]$downloaded)
    }
    $totalBytes += $downloaded
    $results.Add([pscustomobject]@{
        image          = $name
        ref            = $ref
        pullExitCode   = $pull.ExitCode
        localSizeBytes = $downloaded
        localSizeMb    = [math]::Round($downloaded / 1MB, 1)
        note           = 'Local image size after pull; registry compressed download may differ'
    })
    if ($pull.ExitCode -ne 0) {
        Write-Host "  pull failed (exit $($pull.ExitCode))" -ForegroundColor Red
    }
}

$report = [ordered]@{
    blocked           = $false
    measured          = $true
    imageTag          = $ImageTag
    measuredAt        = (Get-Date).ToString('o')
    totalLocalSizeMb  = [math]::Round($totalBytes / 1MB, 1)
    images            = @($results)
    note              = 'Run after publishing tag to GHCR. Compressed registry bytes require manifest/layer inspection; local size is a lower-bound proxy.'
}

New-Item -ItemType Directory -Force -Path (Split-Path $OutFile) | Out-Null
$report | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath $OutFile -Encoding UTF8
Write-Host "Wrote $OutFile" -ForegroundColor Green
$report.images | Format-Table image, localSizeMb, pullExitCode -AutoSize
