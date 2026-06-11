# Recreate env-sensitive services when container TWITCH_OAUTH_* differs from .env.
# docker compose restart does NOT reload env_file — only --force-recreate does.
param(
    [string]$EnvFile = '.env',
    [string[]]$Services = @('chat', 'metadata', 'analytics', 'emote')
)

$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent $PSScriptRoot
Set-Location $Root

. (Join-Path $PSScriptRoot 'lib\stack-progress.ps1')

function Read-EnvValue {
    param([string]$Path, [string]$Key)
    if (-not (Test-Path $Path)) { return '' }
    foreach ($line in Get-Content $Path) {
        if ($line -match "^$([regex]::Escape($Key))=(.*)$") {
            return $matches[1].Trim()
        }
    }
    return ''
}

function Get-ContainerEnvValue {
    param([string]$Container, [string]$Key)
    if (-not $Container) { return '' }
    $result = Invoke-EnvDockerCaptured -Arguments @('inspect', '-f', '{{.State.Status}}', $Container)
    if ($result.ExitCode -ne 0 -or ($result.Output | Select-Object -First 1) -ne 'running') { return $null }
    $linesResult = Invoke-EnvDockerCaptured -Arguments @('inspect', $Container, '--format', '{{range .Config.Env}}{{println .}}{{end}}')
    if ($linesResult.ExitCode -ne 0) { return $null }
    $prefix = "$Key="
    foreach ($line in $linesResult.Output) {
        if ($line.StartsWith($prefix)) {
            return $line.Substring($prefix.Length)
        }
    }
    return ''
}

$desiredId = Read-EnvValue -Path $EnvFile -Key 'TWITCH_OAUTH_CLIENT_ID'
if ([string]::IsNullOrWhiteSpace($desiredId)) {
    Write-Host "reload-env-if-stale: TWITCH_OAUTH_CLIENT_ID not in $EnvFile - skip (run make twitch-sync)"
    exit 0
}

$profile = Read-EnvValue -Path $EnvFile -Key 'STREAMCLONE_PROFILE'
if ([string]::IsNullOrWhiteSpace($profile)) { $profile = 'core' }
$useImages = (Read-EnvValue -Path $EnvFile -Key 'STREAMCLONE_USE_IMAGES') -eq '1'
if (-not $useImages -and (Read-EnvValue -Path $EnvFile -Key 'IMAGE_TAG')) { $useImages = $true }

$composeArgs = Get-StreamcloneComposeArgs -Root $Root -Profile $profile -UseImages:$useImages
$psResult = Invoke-EnvDockerCaptured -Arguments ($composeArgs + @('ps', '--format', '{{.Service}}|{{.Name}}|{{.State}}'))
if ($psResult.ExitCode -ne 0) {
    Write-Host 'reload-env-if-stale: docker compose ps unavailable - skip'
    exit 0
}

$stale = @()
foreach ($line in $psResult.Output) {
    if ([string]::IsNullOrWhiteSpace($line)) { continue }
    $parts = $line -split '\|', 3
    if ($parts.Count -lt 3) { continue }
    $service = $parts[0]
    $container = $parts[1]
    $state = $parts[2]
    if ($service -notin $Services) { continue }
    if ($state -ne 'running') { continue }
    $actual = Get-ContainerEnvValue -Container $container -Key 'TWITCH_OAUTH_CLIENT_ID'
    if ($null -eq $actual) { continue }
    if ($actual -ne $desiredId) {
        $stale += $service
    }
}

if ($stale.Count -eq 0) {
    Write-Host 'reload-env-if-stale: container OAuth env matches .env'
    exit 0
}

Write-Host "reload-env-if-stale: stale OAuth in [$($stale -join ', ')] - force-recreating: $($Services -join ' ')"
$code = Invoke-EnvDocker -Arguments ($composeArgs + @('up', '-d', '--no-deps', '--force-recreate') + $Services)
if ($code -ne 0) {
    Write-Host "reload-env-if-stale: docker compose recreate failed (exit $code)" -ForegroundColor Red
    exit $code
}
Write-Host "reload-env-if-stale: recreated $($Services -join ' ')"
exit 0
