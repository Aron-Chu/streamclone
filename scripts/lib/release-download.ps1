#Requires -Version 5.1
param(
    [string]$InstallDir,
    [string]$Version = '',
    [string]$Repo = 'Aron-Chu/streamclone'
)

$ErrorActionPreference = 'Stop'

function Get-LatestReleaseAssetUrl {
    param([string]$Suffix)
    $headers = @{ 'User-Agent' = 'streamclone-install' }
    if ($Version) {
        $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/tags/$Version" -Headers $headers
    } else {
        $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -Headers $headers
    }
    $asset = $release.assets | Where-Object { $_.name -like "*$Suffix" } | Select-Object -First 1
    if (-not $asset) {
        throw "No release asset matching *$Suffix in release $($release.tag_name)"
    }
    return @{ Url = $asset.browser_download_url; Tag = $release.tag_name; Name = $asset.name }
}

$meta = Get-LatestReleaseAssetUrl -Suffix '-windows.zip'
Write-Host "Downloading $($meta.Name) ($($meta.Tag))..."

$tempZip = Join-Path ([System.IO.Path]::GetTempPath()) "streamclone-$([Guid]::NewGuid().N).zip"
try {
    Invoke-WebRequest -Uri $meta.Url -OutFile $tempZip -UseBasicParsing
    if (Test-Path $InstallDir) {
        Remove-Item -Recurse -Force $InstallDir
    }
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    Expand-Archive -Path $tempZip -DestinationPath $InstallDir -Force
    # Legacy archives nested streamclone-<tag>/ — hoist if needed.
    $nested = Join-Path $InstallDir ("streamclone-" + $meta.Tag)
    if ((Test-Path $nested) -and -not (Test-Path (Join-Path $InstallDir 'VERSION'))) {
        Get-ChildItem $nested -Force | Move-Item -Destination $InstallDir -Force
        Remove-Item -Recurse -Force $nested
    }
} finally {
    Remove-Item -Force $tempZip -ErrorAction SilentlyContinue
}

if (-not (Test-Path (Join-Path $InstallDir 'VERSION'))) {
    throw "Release extract failed — VERSION missing in $InstallDir"
}

Write-Host "Installed release bundle to $InstallDir"
return $meta.Tag
