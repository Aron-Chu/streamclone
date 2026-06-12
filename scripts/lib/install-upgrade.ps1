#Requires -Version 5.1
# Version detection, release bundle merge, upgrade pull/recreate for Streamclone installs.

. (Join-Path $PSScriptRoot 'env.ps1')
. (Join-Path $PSScriptRoot 'stack-progress.ps1')

$Script:StreamcloneCoreImageRepos = @(
    'metadata', 'video', 'chat', 'analytics', 'emote', 'frontend', 'clipper'
)

$Script:StreamcloneGhcrPrefix = 'ghcr.io/aron-chu/streamclone'

$Script:StreamcloneBaseImages = @(
    'postgres:16-alpine',
    'redis:7-alpine',
    'minio/minio:latest',
    'caddy:2',
    'bluenviron/mediamtx:latest',
    'migrate/migrate:latest'
)

$Script:StreamcloneBootstrapOverlayPaths = @(
    'scripts/setup.ps1',
    'scripts/lib/env.ps1',
    'scripts/lib/install-upgrade.ps1',
    'scripts/lib/diagnostics.ps1',
    'scripts/install-setup-progress.ps1',
    'scripts/streamclone-manager.ps1',
    'scripts/check-streamclone.ps1',
    'scripts/install-desktop-shortcut.ps1',
    'scripts/preflight-deps.ps1',
    'scripts/start-streamclone.ps1',
    'scripts/bootstrap-windows-install.ps1',
    'launchers/install-streamclone-launcher.ps1'
)

function Get-StreamcloneCoreImageRefs {
    param([string]$Tag)
    if ([string]::IsNullOrWhiteSpace($Tag)) { $Tag = 'latest' }
    foreach ($repo in $Script:StreamcloneCoreImageRepos) {
        "${Script:StreamcloneGhcrPrefix}/${repo}:$Tag"
    }
}

function Get-StreamcloneInstallVersions {
    param(
        [string]$Root = '',
        [switch]$FetchLatest,
        [string]$Repo = 'Aron-Chu/streamclone'
    )
    if ([string]::IsNullOrWhiteSpace($Root)) {
        $Root = Get-EnvRepoRoot
    } else {
        $Root = (Resolve-Path -LiteralPath $Root).Path
    }

    $bundleVersion = ''
    $versionFile = Join-Path $Root 'VERSION'
    if (Test-Path $versionFile) {
        $bundleVersion = (Get-Content $versionFile -Raw).Trim()
    }

    $imageTag = ''
    $envFile = Join-Path $Root '.env'
    if (Test-Path $envFile) {
        $vals = Read-EnvKeyValueFile -Path $envFile
        $imageTag = [string]$vals['IMAGE_TAG']
    }
    if ([string]::IsNullOrWhiteSpace($imageTag)) {
        $imageTag = $bundleVersion
    }

    $latestRelease = $null
    if ($FetchLatest) {
        try {
            $headers = @{ 'User-Agent' = 'streamclone-install' }
            $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -Headers $headers -TimeoutSec 15
            $latestRelease = [string]$release.tag_name
        } catch {
            $latestRelease = $null
        }
    }

    return [pscustomobject]@{
        root          = $Root
        bundleVersion = $bundleVersion
        imageTag      = $imageTag
        latestRelease = $latestRelease
    }
}

function Test-StreamcloneUpgradeNeeded {
    param(
        [string]$Root = '',
        [switch]$IncludeLatestRelease
    )
    $versions = Get-StreamcloneInstallVersions -Root $Root -FetchLatest:$IncludeLatestRelease
    if ($versions.bundleVersion -and $versions.imageTag -and ($versions.bundleVersion -ne $versions.imageTag)) {
        return $true
    }
    if ($IncludeLatestRelease -and $versions.latestRelease) {
        if ($versions.imageTag -and ($versions.imageTag -ne $versions.latestRelease)) {
            return $true
        }
        if ($versions.bundleVersion -and ($versions.bundleVersion -ne $versions.latestRelease)) {
            return $true
        }
    }
    return $false
}

function Sync-StreamcloneImageTag {
    param(
        [string]$Root = '',
        [string]$Tag = '',
        [string]$EnvFile = ''
    )
    if ([string]::IsNullOrWhiteSpace($Root)) {
        $Root = Get-EnvRepoRoot
    }
    if ([string]::IsNullOrWhiteSpace($EnvFile)) {
        $EnvFile = Join-Path $Root '.env'
    }
    if ([string]::IsNullOrWhiteSpace($Tag)) {
        $Tag = (Get-StreamcloneInstallVersions -Root $Root).bundleVersion
    }
    if ([string]::IsNullOrWhiteSpace($Tag)) {
        throw 'Cannot sync IMAGE_TAG - VERSION file missing and no tag specified.'
    }
    if (-not (Test-Path $EnvFile)) {
        throw "Cannot sync IMAGE_TAG - missing $EnvFile"
    }
    Set-EnvFileValue -Path $EnvFile -Key 'IMAGE_TAG' -Value $Tag
    Set-EnvFileValue -Path $EnvFile -Key 'STREAMCLONE_USE_IMAGES' -Value '1'
    return $Tag
}

function Test-StreamcloneLocalImagePresent {
    param([string]$ImageRef)
    $result = Invoke-EnvDockerCapturedWithTimeout -Arguments @('image', 'inspect', '-f', '{{.Id}}', $ImageRef) -TimeoutSec 10
    return ($result.ExitCode -eq 0 -and $result.Output.Count -gt 0)
}

function Get-StreamcloneCoreImageStatus {
    param(
        [string]$Root = '',
        [string]$Tag = ''
    )
    if ([string]::IsNullOrWhiteSpace($Root)) {
        $Root = Get-EnvRepoRoot
    }
    if ([string]::IsNullOrWhiteSpace($Tag)) {
        $Tag = (Get-StreamcloneInstallVersions -Root $Root).imageTag
    }
    if ([string]::IsNullOrWhiteSpace($Tag)) { $Tag = 'latest' }

    $present = [System.Collections.Generic.List[string]]::new()
    $missing = [System.Collections.Generic.List[string]]::new()
    foreach ($repo in $Script:StreamcloneCoreImageRepos) {
        $ref = "${Script:StreamcloneGhcrPrefix}/${repo}:$Tag"
        if (Test-StreamcloneLocalImagePresent -ImageRef $ref) {
            [void]$present.Add($repo)
        } else {
            [void]$missing.Add($repo)
        }
    }

    return [pscustomobject]@{
        tag     = $Tag
        present = $present.Count
        total   = $Script:StreamcloneCoreImageRepos.Count
        missing = @($missing)
        repos   = @($Script:StreamcloneCoreImageRepos)
    }
}

function Get-StreamcloneDockerReclaimEstimate {
    param(
        [string]$Root = '',
        [string]$Tag = '',
        [switch]$IncludeBaseImages
    )
    if ([string]::IsNullOrWhiteSpace($Root)) {
        $Root = Get-EnvRepoRoot
    }
    if ([string]::IsNullOrWhiteSpace($Tag)) {
        $Tag = (Get-StreamcloneInstallVersions -Root $Root).imageTag
    }
    if ([string]::IsNullOrWhiteSpace($Tag)) { $Tag = 'latest' }

    $refs = @(Get-StreamcloneCoreImageRefs -Tag $Tag)
    if ($IncludeBaseImages) {
        $refs += $Script:StreamcloneBaseImages
    }

    $totalBytes = [long]0
    $imageCount = 0
    foreach ($ref in ($refs | Select-Object -Unique)) {
        $result = Invoke-EnvDockerCapturedWithTimeout -Arguments @('image', 'inspect', '-f', '{{.Size}}', $ref) -TimeoutSec 10
        if ($result.ExitCode -ne 0 -or -not $result.Output) { continue }
        $sizeText = ($result.Output | Select-Object -First 1).Trim()
        if ($sizeText -match '^\d+$') {
            $totalBytes += [long]$sizeText
            $imageCount++
        }
    }

    $gb = [math]::Round($totalBytes / 1GB, 2)
    return [pscustomobject]@{
        tag        = $Tag
        imageCount = $imageCount
        bytes      = $totalBytes
        gigabytes  = $gb
        label      = if ($gb -ge 0.1) { "~$gb GB" } else { '<0.1 GB' }
    }
}

function Invoke-StreamcloneReleaseDownload {
    param(
        [Parameter(Mandatory = $true)][string]$Url,
        [Parameter(Mandatory = $true)][string]$OutFile,
        [scriptblock]$OnProgress = $null
    )

    $request = [System.Net.HttpWebRequest]::Create($Url)
    $request.UserAgent = 'streamclone-install'
    $request.Method = 'GET'
    $request.Timeout = 600000
    $request.ReadWriteTimeout = 600000

    $response = $request.GetResponse()
    try {
        $total = $response.ContentLength
        $stream = $response.GetResponseStream()
        $fileStream = [System.IO.File]::Create($OutFile)
        try {
            $buffer = New-Object byte[] 65536
            $read = 0L
            while (($count = $stream.Read($buffer, 0, $buffer.Length)) -gt 0) {
                $fileStream.Write($buffer, 0, $count)
                $read += $count
                if ($OnProgress) {
                    try {
                        & $OnProgress $read $total
                    } catch { }
                }
            }
        } finally {
            $fileStream.Close()
            $stream.Close()
        }
    } finally {
        $response.Close()
    }
}

function Test-StreamcloneReleaseBundlePreservePath {
    param([string]$RelativePath)
    $name = Split-Path -Leaf $RelativePath
    if ($name -eq '.env' -or $name -eq '.env.local') { return $true }
    if ($name -like '.streamclone-*') { return $true }
    if ($RelativePath -replace '\\', '/' -like 'logs/*') { return $true }
    if ($RelativePath -eq 'logs') { return $true }
    return $false
}

function Sync-StreamcloneReleaseBundle {
    param(
        [Parameter(Mandatory = $true)][string]$ZipPath,
        [Parameter(Mandatory = $true)][string]$InstallDir,
        [string]$ExpectedTag = ''
    )

    $InstallDir = (Resolve-Path -LiteralPath $InstallDir -ErrorAction SilentlyContinue).Path
    if (-not $InstallDir) {
        New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
        $InstallDir = (Resolve-Path -LiteralPath $InstallDir).Path
    }

    $tempExtract = Join-Path ([System.IO.Path]::GetTempPath()) "streamclone-extract-$([Guid]::NewGuid().N)"
    New-Item -ItemType Directory -Path $tempExtract -Force | Out-Null
    try {
        Expand-Archive -Path $ZipPath -DestinationPath $tempExtract -Force

        $sourceRoot = $tempExtract
        if ($ExpectedTag) {
            $nested = Join-Path $tempExtract ("streamclone-" + $ExpectedTag)
            if ((Test-Path $nested) -and -not (Test-Path (Join-Path $tempExtract 'VERSION'))) {
                $sourceRoot = $nested
            }
        } else {
            $nestedDirs = Get-ChildItem $tempExtract -Directory -ErrorAction SilentlyContinue |
                Where-Object { $_.Name -like 'streamclone-*' }
            if ($nestedDirs.Count -eq 1 -and -not (Test-Path (Join-Path $tempExtract 'VERSION'))) {
                $sourceRoot = $nestedDirs[0].FullName
            }
        }

        if (-not (Test-Path (Join-Path $sourceRoot 'VERSION'))) {
            throw "Release extract failed - VERSION missing in archive."
        }

        $files = Get-ChildItem -LiteralPath $sourceRoot -Recurse -Force -File
        foreach ($file in $files) {
            $relative = $file.FullName.Substring($sourceRoot.Length).TrimStart('\', '/')
            if (Test-StreamcloneReleaseBundlePreservePath -RelativePath $relative) {
                $destPath = Join-Path $InstallDir ($relative -replace '/', '\')
                if (Test-Path $destPath) { continue }
            }
            $destPath = Join-Path $InstallDir ($relative -replace '/', '\')
            $destDir = Split-Path $destPath -Parent
            if (-not (Test-Path $destDir)) {
                New-Item -ItemType Directory -Path $destDir -Force | Out-Null
            }
            Copy-Item -LiteralPath $file.FullName -Destination $destPath -Force
        }
    } finally {
        Remove-Item -LiteralPath $tempExtract -Recurse -Force -ErrorAction SilentlyContinue
    }

    if (-not (Test-Path (Join-Path $InstallDir 'VERSION'))) {
        throw "Release merge failed - VERSION missing in $InstallDir"
    }
}

function Get-StreamcloneReleaseZipMeta {
    param(
        [string]$Version = '',
        [string]$Repo = 'Aron-Chu/streamclone'
    )
    $headers = @{ 'User-Agent' = 'streamclone-install' }
    if ($Version) {
        $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/tags/$Version" -Headers $headers
    } else {
        $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -Headers $headers
    }
    $asset = $release.assets | Where-Object { $_.name -like '*-windows.zip' } | Select-Object -First 1
    if (-not $asset) {
        throw "No windows.zip asset in release $($release.tag_name)"
    }
    return [pscustomobject]@{
        Url  = $asset.browser_download_url
        Tag  = $release.tag_name
        Name = $asset.name
    }
}

function Invoke-StreamcloneReleaseDownloadWithProgress {
    param(
        [string]$Version = '',
        [string]$Repo = 'Aron-Chu/streamclone',
        [string]$OutFile = '',
        [scriptblock]$OnProgress = $null
    )
    $meta = Get-StreamcloneReleaseZipMeta -Version $Version -Repo $Repo
    if ([string]::IsNullOrWhiteSpace($OutFile)) {
        $OutFile = Join-Path ([System.IO.Path]::GetTempPath()) "streamclone-$([Guid]::NewGuid().N).zip"
    }
    $lastPct = -1
    $progressBlock = {
        param($Read, $Total)
        if ($Total -gt 0) {
            $pct = [math]::Min(100, [math]::Floor(100.0 * $Read / $Total))
            if ($pct -ne $script:lastPct) {
                $script:lastPct = $pct
                Write-Host ("Downloading... {0}%" -f $pct) -ForegroundColor Cyan
            }
        } elseif ($OnProgress) {
            & $OnProgress $Read $Total
        }
    }.GetNewClosure()
    Invoke-StreamcloneReleaseDownload -Url $meta.Url -OutFile $OutFile -OnProgress $progressBlock
    return [pscustomobject]@{
        ZipPath = $OutFile
        Meta    = $meta
    }
}

function Update-StreamcloneBootstrapOverlayFromMaster {
    param(
        [string]$Dir,
        [string]$Repo = 'Aron-Chu/streamclone',
        [string[]]$OverlayPaths = $Script:StreamcloneBootstrapOverlayPaths
    )
    $masterRawBase = "https://raw.githubusercontent.com/$Repo/master"
    $headers = @{ 'User-Agent' = 'streamclone-bootstrap' }
    $cacheBust = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
    foreach ($rel in $OverlayPaths) {
        $dest = Join-Path $Dir ($rel -replace '/', '\')
        $destDir = Split-Path $dest -Parent
        if (-not (Test-Path $destDir)) {
            New-Item -ItemType Directory -Path $destDir -Force | Out-Null
        }
        try {
            $url = "$masterRawBase/$rel`?t=$cacheBust"
            Invoke-WebRequest -Uri $url -OutFile $dest -Headers $headers -UseBasicParsing
        } catch {
            Write-Host "  script overlay skipped: $rel ($($_.Exception.Message))" -ForegroundColor DarkYellow
        }
    }
}

function Invoke-StreamcloneUpgrade {
    param(
        [string]$Root = '',
        [string]$TargetTag = '',
        [scriptblock]$OnLine = $null,
        [int]$WaitTimeoutSec = 300,
        [ValidateSet('interactive', 'capture', 'summary')][string]$PullOutputMode = 'interactive'
    )
    if ([string]::IsNullOrWhiteSpace($Root)) {
        $Root = Get-EnvRepoRoot
    }
    $Root = (Resolve-Path -LiteralPath $Root).Path
    if ([string]::IsNullOrWhiteSpace($TargetTag)) {
        $TargetTag = (Get-StreamcloneInstallVersions -Root $Root).bundleVersion
    }
    if ([string]::IsNullOrWhiteSpace($TargetTag)) {
        throw 'Cannot upgrade - VERSION file missing.'
    }

    Sync-StreamcloneImageTag -Root $Root -Tag $TargetTag
    Set-Location $Root

    $profile = Get-StreamcloneProfileFromRoot -Root $Root
    $composeArgs = Get-StreamcloneComposeArgs -Root $Root -Profile $profile

    Write-Host "Upgrading Streamclone images to $TargetTag..." -ForegroundColor Cyan
    Write-Host 'Pulling Docker images (~1.5 GB, 3-8 min on first install)...' -ForegroundColor Cyan
    Write-Host ''
    $pull = Invoke-EnvDockerComposePullWithRetry -ComposeArgs $composeArgs -OnLine $OnLine -OutputMode $PullOutputMode
    if ($pull.ExitCode -ne 0) {
        throw "docker compose pull failed (exit $($pull.ExitCode))."
    }

    Write-Host 'Recreating containers with updated images...' -ForegroundColor Cyan
    $up = Invoke-EnvDockerStreaming -Arguments ($composeArgs + @('up', '-d', '--remove-orphans', '--force-recreate', '--pull', 'always')) -OnLine $OnLine
    if ($up.ExitCode -ne 0) {
        throw "docker compose up failed (exit $($up.ExitCode))."
    }

    $waitScript = Join-Path $PSScriptRoot 'wait-stack.ps1'
    if (Test-Path $waitScript) {
        & $waitScript -TimeoutSec $WaitTimeoutSec
    }
    Write-Host 'Upgrade complete.' -ForegroundColor Green
}

function Remove-StreamcloneBaseImages {
    $prev = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        foreach ($ref in $Script:StreamcloneBaseImages) {
            Invoke-EnvDockerCaptured -Arguments @('image', 'rm', '-f', $ref) | Out-Null
        }
    } finally {
        $ErrorActionPreference = $prev
    }
}

function Test-StreamcloneGhcrManifestReachable {
    param([string]$Tag)
    if ([string]::IsNullOrWhiteSpace($Tag)) { $Tag = 'latest' }
    $ref = "${Script:StreamcloneGhcrPrefix}/metadata:$Tag"
    $result = Invoke-EnvDockerCapturedWithTimeout -Arguments @('manifest', 'inspect', $ref) -TimeoutSec 20
    if ($result.ExitCode -eq 0) { return 'ok' }
    $text = ($result.Output -join ' ').ToLowerInvariant()
    if ($text -match '401|unauthorized|denied') { return 'blocked' }
    if ($text -match '404|not found|manifest unknown') { return 'blocked' }
    if ($result.TimedOut) { return 'timeout' }
    return 'error'
}
