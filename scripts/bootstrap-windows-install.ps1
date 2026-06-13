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

function Write-StreamcloneAgentDebugLog {
    param(
        [string]$HypothesisId,
        [string]$Location,
        [string]$Message,
        [hashtable]$Data = @{},
        [string]$RunId = 'pre-fix'
    )
    #region agent log
    try {
        $entry = [ordered]@{
            sessionId    = 'ccdd9b'
            runId        = $RunId
            hypothesisId = $HypothesisId
            location     = $Location
            message      = $Message
            data         = $Data
            timestamp    = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
        }
        $line = ($entry | ConvertTo-Json -Compress) + [Environment]::NewLine
        foreach ($logPath in @(
                (Join-Path $env:TEMP 'debug-ccdd9b.log'),
                (Join-Path $InstallDir 'debug-ccdd9b.log')
            )) {
            if ($logPath) {
                $logDir = Split-Path $logPath -Parent
                if ($logDir -and -not (Test-Path $logDir)) {
                    New-Item -ItemType Directory -Force -Path $logDir | Out-Null
                }
                Add-Content -LiteralPath $logPath -Value $line -Encoding utf8
            }
        }
    } catch { }
    #endregion
}

function Get-StreamcloneGitHubMasterSha {
    param([string]$Repo)
    $headers = @{ 'User-Agent' = 'streamclone-bootstrap' }
    return (Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/commits/master" -Headers $headers).sha
}

function Test-StreamcloneBootstrapScriptParses {
    param([string]$Path)
    $tokens = $null
    $errors = $null
    [void][System.Management.Automation.Language.Parser]::ParseFile($Path, [ref]$tokens, [ref]$errors)
    return @{
        Ok     = ($errors.Count -eq 0)
        Errors = @($errors | ForEach-Object { $_.ToString() })
    }
}

function Resolve-StreamcloneBootstrapEnvScript {
    foreach ($dir in @(
            $script:StreamcloneBootstrapLibDir,
            (Join-Path $env:TEMP 'streamclone-bootstrap-lib'),
            (Join-Path $PSScriptRoot 'lib')
        )) {
        if ([string]::IsNullOrWhiteSpace($dir)) { continue }
        $candidate = Join-Path $dir 'env.ps1'
        if (Test-Path $candidate) { return $candidate }
    }
    return $null
}

$script:StreamcloneMasterSha = Get-StreamcloneGitHubMasterSha -Repo $Repo
Write-StreamcloneAgentDebugLog -HypothesisId 'A' -Location 'bootstrap:start' -Message 'resolved master commit sha' -Data @{
    sha          = $script:StreamcloneMasterSha
    psscriptRoot = $PSScriptRoot
    installDir   = $InstallDir
}

$bootstrapLibScript = Join-Path $PSScriptRoot 'lib\install-upgrade.ps1'
if (Test-Path $bootstrapLibScript) {
    $script:StreamcloneBootstrapLibDir = Join-Path $PSScriptRoot 'lib'
    . $bootstrapLibScript
} else {
    $libDir = Join-Path $env:TEMP 'streamclone-bootstrap-lib'
    if (Test-Path $libDir) {
        Remove-Item -LiteralPath $libDir -Recurse -Force -ErrorAction SilentlyContinue
    }
    New-Item -ItemType Directory -Force -Path $libDir | Out-Null
    $base = "https://raw.githubusercontent.com/$Repo/$($script:StreamcloneMasterSha)/scripts/lib"
    $headers = @{ 'User-Agent' = 'streamclone-bootstrap' }
    foreach ($name in @('env.ps1', 'stack-progress.ps1', 'install-upgrade.ps1')) {
        $dest = Join-Path $libDir $name
        $url = "$base/$name"
        Invoke-WebRequest -Uri $url -OutFile $dest -Headers $headers -UseBasicParsing
        $parse = Test-StreamcloneBootstrapScriptParses -Path $dest
        if (-not $parse.Ok) {
            throw "Downloaded $name failed PowerShell parse (encoding?). Errors: $($parse.Errors -join '; ')"
        }
    }
    $script:StreamcloneBootstrapLibDir = $libDir
    . (Join-Path $libDir 'install-upgrade.ps1')
}

Write-StreamcloneAgentDebugLog -HypothesisId 'D' -Location 'bootstrap:lib-loaded' -Message 'bootstrap lib ready' -Data @{
    bootstrapLibDir = $script:StreamcloneBootstrapLibDir
    envExists       = [bool](Resolve-StreamcloneBootstrapEnvScript)
}

function Test-StreamcloneWebOk {
    $envScript = Resolve-StreamcloneBootstrapEnvScript
    Write-StreamcloneAgentDebugLog -HypothesisId 'D' -Location 'bootstrap:Test-StreamcloneWebOk' -Message 'resolved env.ps1 path' -Data @{
        envScript       = $envScript
        bootstrapLibDir = $script:StreamcloneBootstrapLibDir
    }
    if (-not $envScript) {
        throw 'Bootstrap could not locate env.ps1 (lib download may have failed).'
    }
    . $envScript
    return Test-StreamcloneWebReachable -Url (Get-StreamcloneAppUrl)
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

if (-not $Force -and $versions -and $versions.bundleVersion -eq $meta.Tag -and (Test-Path $launcher) -and (Test-Path (Join-Path $InstallDir '.env'))) {
    Write-StreamcloneAgentDebugLog -HypothesisId 'B' -Location 'bootstrap:fast-path' -Message 'same-version reinstall fast path' -Data @{
        tag     = $meta.Tag
        hasEnv  = $true
        hasLauncher = $true
    }
    $webOk = Test-StreamcloneWebOk
    Write-Host ''
    if ($webOk) {
        Write-Host ("  Already on $($meta.Tag) and running at {0}" -f (Get-StreamcloneAppUrl)) -ForegroundColor Green
    } else {
        Write-Host "  Already on $($meta.Tag) (stack not up yet - will start Docker)" -ForegroundColor Green
    }
    Write-Host '  Refreshing install scripts and shortcuts...' -ForegroundColor Yellow
    if ($versions.bundleVersion -ne $versions.imageTag) {
        Write-Host "  Note: bundle $($versions.bundleVersion) but images $($versions.imageTag) - run Manage -> Update." -ForegroundColor Yellow
    }
    Update-StreamcloneBootstrapOverlayFromMaster -Dir $InstallDir -Repo $Repo
    & $launcher -Action install -LauncherRoot $InstallDir
    exit $LASTEXITCODE
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
