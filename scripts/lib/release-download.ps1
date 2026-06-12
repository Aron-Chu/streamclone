#Requires -Version 5.1
param(
    [string]$InstallDir,
    [string]$Version = '',
    [string]$Repo = 'Aron-Chu/streamclone'
)

$ErrorActionPreference = 'Stop'

. (Join-Path $PSScriptRoot 'install-upgrade.ps1')

$meta = Get-StreamcloneReleaseZipMeta -Version $Version -Repo $Repo
Write-Host "Downloading $($meta.Name) ($($meta.Tag))..."

$tempZip = Join-Path ([System.IO.Path]::GetTempPath()) "streamclone-$([Guid]::NewGuid().N).zip"
try {
    $lastPct = -1
    Invoke-StreamcloneReleaseDownload -Url $meta.Url -OutFile $tempZip -OnProgress {
        param($Read, $Total)
        if ($Total -gt 0) {
            $pct = [math]::Min(100, [math]::Floor(100.0 * $Read / $Total))
            if ($pct -ne $script:lastPct) {
                $script:lastPct = $pct
                Write-Host ("Downloading… {0}%" -f $pct) -ForegroundColor Cyan
            }
        }
    }

    $existing = Test-Path (Join-Path $InstallDir 'scripts\start-streamclone.ps1')
    if ($existing) {
        Sync-StreamcloneReleaseBundle -ZipPath $tempZip -InstallDir $InstallDir -ExpectedTag $meta.Tag
    } else {
        if (Test-Path $InstallDir) {
            Remove-Item -Recurse -Force $InstallDir
        }
        New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
        Sync-StreamcloneReleaseBundle -ZipPath $tempZip -InstallDir $InstallDir -ExpectedTag $meta.Tag
    }
} finally {
    Remove-Item -Force $tempZip -ErrorAction SilentlyContinue
}

if (-not (Test-Path (Join-Path $InstallDir 'VERSION'))) {
    throw "Release extract failed — VERSION missing in $InstallDir"
}

Write-Host "Installed release bundle to $InstallDir"
return $meta.Tag
