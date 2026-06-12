#Requires -Version 5.1
# Self-contained Windows bootstrap: download release ZIP, extract, run local Install launcher.
# Used when Install Streamclone.cmd is run standalone (Downloads folder) without the full bundle.
param(
    [string]$InstallDir = (Join-Path $env:USERPROFILE 'streamclone'),
    [string]$Version = '',
    [switch]$Force
)

$ErrorActionPreference = 'Stop'
$Repo = 'Aron-Chu/streamclone'

function Get-ReleaseZipMeta {
    param([string]$Tag)
    $headers = @{ 'User-Agent' = 'streamclone-bootstrap' }
    if ($Tag) {
        $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/tags/$Tag" -Headers $headers
    } else {
        $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -Headers $headers
    }
    $asset = $release.assets | Where-Object { $_.name -like '*-windows.zip' } | Select-Object -First 1
    if (-not $asset) {
        throw "No windows.zip asset in release $($release.tag_name)"
    }
    return @{
        Url  = $asset.browser_download_url
        Tag  = $release.tag_name
        Name = $asset.name
    }
}

function Test-StreamcloneWebOk {
    try {
        $resp = Invoke-WebRequest -Uri 'http://localhost:8090/' -UseBasicParsing -TimeoutSec 5
        return ($resp.StatusCode -ge 200 -and $resp.StatusCode -lt 500)
    } catch {
        return $false
    }
}

function Get-LocalInstallVersion {
    param([string]$Dir)
    $versionFile = Join-Path $Dir 'VERSION'
    if (-not (Test-Path $versionFile)) { return '' }
    return (Get-Content $versionFile -Raw).Trim()
}

function Prepare-InstallDirectory {
    param([string]$Dir)
    Set-Location $env:TEMP
    if (-not (Test-Path -LiteralPath $Dir)) {
        New-Item -ItemType Directory -Path $Dir -Force | Out-Null
        return
    }
    try {
        Remove-Item -LiteralPath $Dir -Recurse -Force -ErrorAction Stop
        New-Item -ItemType Directory -Path $Dir -Force | Out-Null
        return
    } catch { }
    Get-ChildItem -LiteralPath $Dir -Force -ErrorAction SilentlyContinue |
        Remove-Item -Recurse -Force -ErrorAction SilentlyContinue
    $remaining = @(Get-ChildItem -LiteralPath $Dir -Force -ErrorAction SilentlyContinue).Count
    if ($remaining -gt 0) {
        throw "Cannot replace install folder ($Dir) - $remaining item(s) still in use. Close File Explorer windows and terminals open in that folder, then retry."
    }
}

Write-Host 'Step 1/4: Downloading latest release...' -ForegroundColor Cyan
$meta = Get-ReleaseZipMeta -Tag $Version
Write-Host "  $($meta.Name) ($($meta.Tag))"

$localVersion = Get-LocalInstallVersion -Dir $InstallDir
$launcher = Join-Path $InstallDir 'launchers\install-streamclone-launcher.ps1'

if (-not $Force -and $localVersion -eq $meta.Tag -and (Test-Path $launcher)) {
    if (Test-StreamcloneWebOk -and (Test-Path (Join-Path $InstallDir '.env'))) {
        Write-Host ''
        Write-Host "  Already on $($meta.Tag) and running at http://localhost:8090/" -ForegroundColor Green
        Write-Host '  Skipping re-download. Refreshing shortcuts...' -ForegroundColor Yellow
        & $launcher -Action install -LauncherRoot $InstallDir
        exit $LASTEXITCODE
    }
}

$savedEnv = $null
$savedProfile = $null
$savedEnvLocal = $null
if (Test-Path $InstallDir) {
    $envPath = Join-Path $InstallDir '.env'
    $profilePath = Join-Path $InstallDir '.streamclone-profile'
    $envLocalPath = Join-Path $InstallDir '.env.local'
    if (Test-Path $envPath) { $savedEnv = Get-Content $envPath -Raw }
    if (Test-Path $profilePath) { $savedProfile = Get-Content $profilePath -Raw }
    if (Test-Path $envLocalPath) { $savedEnvLocal = Get-Content $envLocalPath -Raw }
}

$tempZip = Join-Path ([System.IO.Path]::GetTempPath()) "streamclone-$([Guid]::NewGuid().N).zip"
try {
    Invoke-WebRequest -Uri $meta.Url -OutFile $tempZip -UseBasicParsing
    Prepare-InstallDirectory -Dir $InstallDir
    Expand-Archive -Path $tempZip -DestinationPath $InstallDir -Force
    $nested = Join-Path $InstallDir ("streamclone-" + $meta.Tag)
    if ((Test-Path $nested) -and -not (Test-Path (Join-Path $InstallDir 'VERSION'))) {
        Get-ChildItem $nested -Force | Move-Item -Destination $InstallDir -Force
        Remove-Item -Recurse -Force $nested
    }
    if ($savedEnv) {
        Set-Content -Path (Join-Path $InstallDir '.env') -Value $savedEnv -NoNewline
        Write-Host '  Preserved existing .env configuration' -ForegroundColor DarkGray
    }
    if ($savedProfile) {
        Set-Content -Path (Join-Path $InstallDir '.streamclone-profile') -Value $savedProfile -NoNewline
    }
    if ($savedEnvLocal) {
        Set-Content -Path (Join-Path $InstallDir '.env.local') -Value $savedEnvLocal -NoNewline
    }
} finally {
    Remove-Item -Force $tempZip -ErrorAction SilentlyContinue
}

if (-not (Test-Path (Join-Path $InstallDir 'VERSION'))) {
    throw "Release extract failed - VERSION missing in $InstallDir"
}

Write-Host "  Extracted to $InstallDir" -ForegroundColor Green

$launcher = Join-Path $InstallDir 'launchers\install-streamclone-launcher.ps1'
if (-not (Test-Path $launcher)) {
    throw "Missing launcher after extract: $launcher"
}

& $launcher -Action install -LauncherRoot $InstallDir
exit $LASTEXITCODE
