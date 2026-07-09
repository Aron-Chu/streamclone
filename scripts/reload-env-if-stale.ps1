# Recreate env-sensitive services when container env differs from .env.
# docker compose restart does NOT reload env_file — only --force-recreate does.
param(
    [string]$EnvFile = '.env',
    [string[]]$Services = @('chat', 'metadata', 'emote', 'frontend')
)

$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent $PSScriptRoot
Set-Location $Root

. (Join-Path $PSScriptRoot 'lib\env.ps1')
. (Join-Path $PSScriptRoot 'lib\stack-progress.ps1')

$envPath = if ([System.IO.Path]::IsPathRooted($EnvFile)) { $EnvFile } else { Join-Path $Root $EnvFile }
Ensure-LocalhostDevTokenImport -EnvFile $envPath | Out-Null

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

$desiredId = Read-EnvValue -Path $envPath -Key 'TWITCH_OAUTH_CLIENT_ID'
$desiredDevToken = Read-EnvValue -Path $envPath -Key 'TWITCH_DEV_TOKEN_IMPORT_ENABLED'
$checkOAuth = -not [string]::IsNullOrWhiteSpace($desiredId)

$profile = Read-EnvValue -Path $envPath -Key 'STREAMCLONE_PROFILE'
if ([string]::IsNullOrWhiteSpace($profile)) { $profile = 'core' }
$useImages = (Read-EnvValue -Path $envPath -Key 'STREAMCLONE_USE_IMAGES') -eq '1'
if (-not $useImages -and (Read-EnvValue -Path $envPath -Key 'IMAGE_TAG')) { $useImages = $true }

$composeArgs = Get-StreamcloneComposeArgs -Root $Root -Profile $profile -UseImages:$useImages
$psResult = Invoke-EnvDockerCaptured -Arguments ($composeArgs + @('ps', '--format', '{{.Service}}|{{.Name}}|{{.State}}'))
if ($psResult.ExitCode -ne 0) {
    Write-Host 'reload-env-if-stale: docker compose ps unavailable - skip'
    exit 0
}

$stale = [System.Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
foreach ($line in $psResult.Output) {
    if ([string]::IsNullOrWhiteSpace($line)) { continue }
    $parts = $line -split '\|', 3
    if ($parts.Count -lt 3) { continue }
    $service = $parts[0]
    $container = $parts[1]
    $state = $parts[2]
    if ($service -notin $Services) { continue }
    if ($state -ne 'running') { continue }

    if ($service -eq 'chat') {
        $actualDev = Get-ContainerEnvValue -Container $container -Key 'TWITCH_DEV_TOKEN_IMPORT_ENABLED'
        if ($null -ne $actualDev -and $actualDev -ne $desiredDevToken) {
            [void]$stale.Add('chat')
        }
    }

    if (-not $checkOAuth) { continue }
    $actual = Get-ContainerEnvValue -Container $container -Key 'TWITCH_OAUTH_CLIENT_ID'
    if ($null -eq $actual) { continue }
    if ($actual -ne $desiredId) {
        [void]$stale.Add($service)
    }
}

if ($stale.Count -eq 0) {
    if (-not $checkOAuth) {
        Write-Host 'reload-env-if-stale: TWITCH_OAUTH_CLIENT_ID not in .env - dev token env matches chat container'
    } else {
        Write-Host 'reload-env-if-stale: container env matches .env'
    }
    exit 0
}

$recreate = @($stale)
Write-Host "reload-env-if-stale: stale env in [$($recreate -join ', ')] - force-recreating"
$code = Invoke-EnvDocker -Arguments ($composeArgs + @('up', '-d', '--no-deps', '--force-recreate') + $recreate)
if ($code -ne 0) {
    Write-Host "reload-env-if-stale: docker compose recreate failed (exit $code)" -ForegroundColor Red
    exit $code
}
Write-Host "reload-env-if-stale: recreated $($recreate -join ' ')"
exit 0
