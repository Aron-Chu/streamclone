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

#region agent log
function Write-BootstrapAgentDebugLog {
    param(
        [string]$HypothesisId,
        [string]$Message,
        [hashtable]$Data = @{}
    )
    $entry = @{
        sessionId    = '1d406b'
        runId        = 'bootstrap-parse-fix'
        hypothesisId = $HypothesisId
        location     = 'bootstrap-windows-install.ps1'
        message      = $Message
        data         = $Data
        timestamp    = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
    } | ConvertTo-Json -Compress
    foreach ($logPath in @(
            (Join-Path (Split-Path $PSScriptRoot -Parent) 'debug-1d406b.log'),
            (Join-Path $env:TEMP 'debug-1d406b.log')
        )) {
        try { Add-Content -LiteralPath $logPath -Value $entry -Encoding UTF8 } catch { }
    }
}
#endregion

$bootstrapLibScript = Join-Path $PSScriptRoot 'lib\install-upgrade.ps1'
if (Test-Path $bootstrapLibScript) {
    . $bootstrapLibScript
    Write-BootstrapAgentDebugLog -HypothesisId 'H2' -Message 'dot-sourced local install-upgrade.ps1' -Data @{ psscriptRoot = $PSScriptRoot }
} else {
    $libDir = Join-Path $env:TEMP 'streamclone-bootstrap-lib'
    New-Item -ItemType Directory -Force -Path $libDir | Out-Null
    $base = "https://raw.githubusercontent.com/$Repo/master/scripts/lib"
    $headers = @{ 'User-Agent' = 'streamclone-bootstrap' }
    foreach ($name in @('env.ps1', 'stack-progress.ps1', 'install-upgrade.ps1')) {
        $dest = Join-Path $libDir $name
        Invoke-WebRequest -Uri "$base/$name" -OutFile $dest -Headers $headers -UseBasicParsing
    }
    . (Join-Path $libDir 'install-upgrade.ps1')
    Write-BootstrapAgentDebugLog -HypothesisId 'H2' -Message 'dot-sourced cached install-upgrade.ps1 from TEMP' -Data @{ libDir = $libDir }
}
Write-BootstrapAgentDebugLog -HypothesisId 'H1' -Message 'bootstrap parsed and libs loaded' -Data @{ installDir = $InstallDir }

function Test-StreamcloneWebOk {
    try {
        $resp = Invoke-WebRequest -Uri 'http://localhost:8090/' -UseBasicParsing -TimeoutSec 5
        return ($resp.StatusCode -ge 200 -and $resp.StatusCode -lt 500)
    } catch {
        return $false
    }
}

function Test-StreamcloneInstalledAt {
    param([string]$Dir)
    return (Test-Path (Join-Path $Dir 'scripts\start-streamclone.ps1'))
}

Write-Host 'Step 1/4: Downloading latest release...' -ForegroundColor Cyan
$meta = Get-StreamcloneReleaseZipMeta -Version $Version -Repo $Repo
Write-Host "  $($meta.Name) ($($meta.Tag))"

$launcher = Join-Path $InstallDir 'launchers\install-streamclone-launcher.ps1'
$versions = if (Test-StreamcloneInstalledAt -Dir $InstallDir) {
    Get-StreamcloneInstallVersions -Root $InstallDir -FetchLatest
} else {
    $null
}

if (-not $Force -and $versions -and $versions.bundleVersion -eq $meta.Tag -and (Test-Path $launcher)) {
    if (Test-StreamcloneWebOk -and (Test-Path (Join-Path $InstallDir '.env'))) {
        Write-Host ''
        Write-Host "  Already on $($meta.Tag) and running at http://localhost:8090/" -ForegroundColor Green
        Write-Host '  Refreshing install scripts and shortcuts...' -ForegroundColor Yellow
        if ($versions.bundleVersion -ne $versions.imageTag) {
            Write-Host "  Note: bundle $($versions.bundleVersion) but images $($versions.imageTag) - run Manage -> Update." -ForegroundColor Yellow
        }
        Update-StreamcloneBootstrapOverlayFromMaster -Dir $InstallDir -Repo $Repo
        & $launcher -Action install -LauncherRoot $InstallDir
        exit $LASTEXITCODE
    }
}

$existingInstall = Test-StreamcloneInstalledAt -Dir $InstallDir
$tempZip = Join-Path ([System.IO.Path]::GetTempPath()) "streamclone-$([Guid]::NewGuid().N).zip"
try {
    $lastPct = -1
    Invoke-StreamcloneReleaseDownload -Url $meta.Url -OutFile $tempZip -OnProgress {
        param($Read, $Total)
        if ($Total -gt 0) {
            $pct = [math]::Min(100, [math]::Floor(100.0 * $Read / $Total))
            if ($pct -ne $script:lastPct) {
                $script:lastPct = $pct
                Write-Host ("  Downloading... {0}%" -f $pct) -ForegroundColor DarkCyan
            }
        }
    }

    if ($existingInstall) {
        Write-Host '  Merging release into existing install (preserving .env and data)...' -ForegroundColor DarkGray
        Sync-StreamcloneReleaseBundle -ZipPath $tempZip -InstallDir $InstallDir -ExpectedTag $meta.Tag
    } else {
        if (-not (Test-Path -LiteralPath $InstallDir)) {
            New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
        }
        Sync-StreamcloneReleaseBundle -ZipPath $tempZip -InstallDir $InstallDir -ExpectedTag $meta.Tag
    }

    Update-StreamcloneBootstrapOverlayFromMaster -Dir $InstallDir -Repo $Repo
} finally {
    Remove-Item -Force $tempZip -ErrorAction SilentlyContinue
}

if (-not (Test-Path (Join-Path $InstallDir 'VERSION'))) {
    throw "Release extract failed - VERSION missing in $InstallDir"
}

$newVersions = Get-StreamcloneInstallVersions -Root $InstallDir
Write-Host "  Extracted to $InstallDir (bundle $($newVersions.bundleVersion))" -ForegroundColor Green
if ($newVersions.imageTag -and ($newVersions.bundleVersion -ne $newVersions.imageTag)) {
    Write-Host "  Images still on $($newVersions.imageTag) - Install will sync on setup." -ForegroundColor Yellow
}

$launcher = Join-Path $InstallDir 'launchers\install-streamclone-launcher.ps1'
if (-not (Test-Path $launcher)) {
    throw "Missing launcher after extract: $launcher"
}

& $launcher -Action install -LauncherRoot $InstallDir
exit $LASTEXITCODE
