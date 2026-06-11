#Requires -Version 5.1
# Self-contained Windows bootstrap: download release ZIP, extract, run local Install launcher.
# Used when Install Streamclone.cmd is run standalone (Downloads folder) without the full bundle.
param(
    [string]$InstallDir = (Join-Path $env:USERPROFILE 'streamclone'),
    [string]$Version = ''
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

Write-Host 'Step 1/4: Downloading latest release...' -ForegroundColor Cyan
$meta = Get-ReleaseZipMeta -Tag $Version
Write-Host "  $($meta.Name) ($($meta.Tag))"

$tempZip = Join-Path ([System.IO.Path]::GetTempPath()) "streamclone-$([Guid]::NewGuid().N).zip"
try {
    Invoke-WebRequest -Uri $meta.Url -OutFile $tempZip -UseBasicParsing
    if (Test-Path $InstallDir) {
        Remove-Item -Recurse -Force $InstallDir
    }
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    Expand-Archive -Path $tempZip -DestinationPath $InstallDir -Force
    $nested = Join-Path $InstallDir ("streamclone-" + $meta.Tag)
    if ((Test-Path $nested) -and -not (Test-Path (Join-Path $InstallDir 'VERSION'))) {
        Get-ChildItem $nested -Force | Move-Item -Destination $InstallDir -Force
        Remove-Item -Recurse -Force $nested
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
