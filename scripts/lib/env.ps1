# Shared env merge + secret generation for bootstrap/setup/validate (PowerShell).

function Get-EnvRepoRoot {
    return (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
}

function Get-EnvRandomHex {
    param([int]$Bytes = 24)
    $buffer = New-Object byte[] $Bytes
    $rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
    try {
        $rng.GetBytes($buffer)
    } finally {
        $rng.Dispose()
    }
    return ([BitConverter]::ToString($buffer) -replace '-', '').ToLower()
}

function Get-EnvProfileFragment {
    param([ValidateSet('core', 'scraper', 'clipper', 'full')][string]$Profile)
    $root = Get-EnvRepoRoot
    return Join-Path $root "deploy\env\profile-$Profile.env"
}

function Get-EnvComposeProfiles {
    param([ValidateSet('core', 'scraper', 'clipper', 'full')][string]$Profile)
    switch ($Profile) {
        'core' { return @() }
        'scraper' { return @('scraper') }
        'clipper' {
            Write-Warning 'STREAMCLONE_PROFILE=clipper is deprecated; ReplayForge runs outside compose. Using core compose profile.'
            return @()
        }
        'full' { return @('scraper') }
    }
}

function Get-EnvFeatureComposeProfiles {
    param([string]$EnvFile = (Join-Path (Get-EnvRepoRoot) '.env'))
    $profiles = @()
    if (-not (Test-Path $EnvFile)) { return $profiles }
    $vals = Read-EnvKeyValueFile -Path $EnvFile
    $scraperKey = [string]$vals['SCRAPER_API_KEY']
    if (-not [string]::IsNullOrWhiteSpace($scraperKey)) {
        $profiles += 'scraper'
    }
    return $profiles
}

function Get-StreamcloneComposeProfiles {
    param(
        [ValidateSet('core', 'scraper', 'clipper', 'full')][string]$Profile = 'core',
        [string]$EnvFile = (Join-Path (Get-EnvRepoRoot) '.env')
    )
    return @((Get-EnvComposeProfiles -Profile $Profile) + (Get-EnvFeatureComposeProfiles -EnvFile $EnvFile) | Select-Object -Unique)
}

function Read-EnvKeyValueFile {
    param([string]$Path)
    $values = @{}
    if (-not (Test-Path $Path)) { return $values }
    foreach ($line in Get-Content $Path) {
        if ($line -match '^(?<key>[A-Z0-9_]+)=(?<value>.*)$') {
            $values[$matches['key']] = $matches['value']
        }
    }
    return $values
}

function Merge-EnvFiles {
    param(
        [string]$OutFile,
        [string[]]$Sources
    )
    $merged = [ordered]@{}
    foreach ($src in $Sources) {
        if (-not (Test-Path $src)) { continue }
        foreach ($line in Get-Content $src) {
            if ($line -match '^(?<key>[A-Z0-9_]+)=(?<value>.*)$') {
                $merged[$matches['key']] = $matches['value']
            }
        }
    }
    $lines = foreach ($key in $merged.Keys) { "$key=$($merged[$key])" }
    Set-Content -Path $OutFile -Value $lines
}

function Set-EnvFileValue {
    param(
        [string]$Path,
        [string]$Key,
        [string]$Value
    )
    $prefix = "$Key="
    $lines = @()
    $found = $false
    if (Test-Path $Path) {
        $lines = [string[]](Get-Content $Path)
        for ($i = 0; $i -lt $lines.Length; $i++) {
            if ($lines[$i].StartsWith($prefix)) {
                $lines[$i] = $prefix + $Value
                $found = $true
            }
        }
    }
    if (-not $found) {
        $lines += ($prefix + $Value)
    }
    Set-Content -Path $Path -Value $lines
}

function Test-EnvPlaceholderValue {
    param(
        [string]$Key,
        [string]$Value
    )
    switch ($Key) {
        'CURATOR_API_TOKEN' { return [string]::IsNullOrWhiteSpace($Value) -or $Value -eq 'change-me' }
        'AUTH_COOKIE_SECRET' { return [string]::IsNullOrWhiteSpace($Value) -or $Value -eq 'dev-insecure-cookie-secret' }
        'SCRAPER_API_KEY' { return [string]::IsNullOrWhiteSpace($Value) -or $Value -eq 'local-dev-key' }
        'CLIPPER_WEBHOOK_TOKEN' { return [string]::IsNullOrWhiteSpace($Value) }
        'VITE_CLIPPER_TOKEN' { return [string]::IsNullOrWhiteSpace($Value) }
        'SETUP_CONTROL_TOKEN' { return [string]::IsNullOrWhiteSpace($Value) }
        default { return $false }
    }
}

function Invoke-EnvGenerateSecrets {
    param([string]$EnvFile)
    $current = Read-EnvKeyValueFile -Path $EnvFile

    if (Test-EnvPlaceholderValue -Key 'CURATOR_API_TOKEN' -Value $current['CURATOR_API_TOKEN']) {
        Set-EnvFileValue -Path $EnvFile -Key 'CURATOR_API_TOKEN' -Value (Get-EnvRandomHex -Bytes 24)
    }
    if (Test-EnvPlaceholderValue -Key 'AUTH_COOKIE_SECRET' -Value $current['AUTH_COOKIE_SECRET']) {
        Set-EnvFileValue -Path $EnvFile -Key 'AUTH_COOKIE_SECRET' -Value (Get-EnvRandomHex -Bytes 32)
    }
    if (Test-EnvPlaceholderValue -Key 'SCRAPER_API_KEY' -Value $current['SCRAPER_API_KEY']) {
        Set-EnvFileValue -Path $EnvFile -Key 'SCRAPER_API_KEY' -Value (Get-EnvRandomHex -Bytes 16)
    }

    $current = Read-EnvKeyValueFile -Path $EnvFile
    if (Test-EnvPlaceholderValue -Key 'CLIPPER_WEBHOOK_TOKEN' -Value $current['CLIPPER_WEBHOOK_TOKEN']) {
        $clipper = Get-EnvRandomHex -Bytes 24
        Set-EnvFileValue -Path $EnvFile -Key 'CLIPPER_WEBHOOK_TOKEN' -Value $clipper
        Set-EnvFileValue -Path $EnvFile -Key 'VITE_CLIPPER_TOKEN' -Value $clipper
    } elseif ([string]::IsNullOrWhiteSpace($current['VITE_CLIPPER_TOKEN']) -and -not [string]::IsNullOrWhiteSpace($current['CLIPPER_WEBHOOK_TOKEN'])) {
        Set-EnvFileValue -Path $EnvFile -Key 'VITE_CLIPPER_TOKEN' -Value $current['CLIPPER_WEBHOOK_TOKEN']
    }

    $current = Read-EnvKeyValueFile -Path $EnvFile
    if (-not [string]::IsNullOrWhiteSpace($current['CLIPPER_WEBHOOK_TOKEN']) -and
        $current['VITE_CLIPPER_TOKEN'] -ne $current['CLIPPER_WEBHOOK_TOKEN']) {
        Set-EnvFileValue -Path $EnvFile -Key 'VITE_CLIPPER_TOKEN' -Value $current['CLIPPER_WEBHOOK_TOKEN']
    }

    $current = Read-EnvKeyValueFile -Path $EnvFile
    if (Test-EnvPlaceholderValue -Key 'SETUP_CONTROL_TOKEN' -Value $current['SETUP_CONTROL_TOKEN']) {
        Set-EnvFileValue -Path $EnvFile -Key 'SETUP_CONTROL_TOKEN' -Value (Get-EnvRandomHex -Bytes 24)
    }
}

function Get-EnvReleaseVersionTag {
    $root = Get-EnvRepoRoot
    $versionFile = Join-Path $root 'VERSION'
    if (-not (Test-Path $versionFile)) { return $null }
    $tag = (Get-Content $versionFile -Raw).Trim()
    if ([string]::IsNullOrWhiteSpace($tag)) { return $null }
    return $tag
}

function Invoke-EnvApplyReleaseImageTag {
    param([string]$EnvFile)
    $current = Read-EnvKeyValueFile -Path $EnvFile
    if (-not [string]::IsNullOrWhiteSpace($current['IMAGE_TAG'])) { return }
    $tag = Get-EnvReleaseVersionTag
    if (-not $tag) { return }
    Set-EnvFileValue -Path $EnvFile -Key 'IMAGE_TAG' -Value $tag
    Set-EnvFileValue -Path $EnvFile -Key 'STREAMCLONE_USE_IMAGES' -Value '1'
}

function Invoke-EnvApplyReleaseDefaults {
    param([string]$EnvFile = (Join-Path (Get-EnvRepoRoot) '.env'))
    $root = Get-EnvRepoRoot
    $bundlePath = Join-Path $root 'deploy\env\release-bundle.env'
    if (-not (Test-Path $bundlePath)) { return $false }
    if (-not (Test-Path $EnvFile)) { return $false }

    $existing = Read-EnvKeyValueFile -Path $EnvFile
    $bundle = Read-EnvKeyValueFile -Path $bundlePath
    $updated = $false

    foreach ($key in $bundle.Keys) {
        if ($key -eq 'IMAGE_TAG') { continue }
        if ([string]::IsNullOrWhiteSpace([string]$existing[$key])) {
            Set-EnvFileValue -Path $EnvFile -Key $key -Value $bundle[$key]
            $updated = $true
        }
    }

    $targetTag = [string]$bundle['IMAGE_TAG']
    if ([string]::IsNullOrWhiteSpace($targetTag)) {
        $targetTag = Get-EnvReleaseVersionTag
    }
    if (-not [string]::IsNullOrWhiteSpace($targetTag)) {
        if ([string]$existing['IMAGE_TAG'] -ne $targetTag) {
            Set-EnvFileValue -Path $EnvFile -Key 'IMAGE_TAG' -Value $targetTag
            $updated = $true
        }
        Set-EnvFileValue -Path $EnvFile -Key 'STREAMCLONE_USE_IMAGES' -Value '1'
    }

    return $updated
}

function Repair-FrontendDockerEntrypointLf {
    $path = Join-Path (Get-EnvRepoRoot) 'frontend\docker-entrypoint.d\40-streamclone-config.sh'
    if (-not (Test-Path $path)) { return }
    $text = [System.IO.File]::ReadAllText($path) -replace "`r`n", "`n" -replace "`r", "`n"
    [System.IO.File]::WriteAllText($path, $text, [System.Text.UTF8Encoding]::new($false))
}

function Invoke-EnvSynthesize {
    param(
        [ValidateSet('core', 'scraper', 'clipper', 'full')][string]$Profile = 'core',
        [string]$OutFile = (Join-Path (Get-EnvRepoRoot) '.env')
    )
    $root = Get-EnvRepoRoot
    $sources = @(
        (Join-Path $root '.env.example'),
        (Get-EnvProfileFragment -Profile $Profile)
    )
    $devProfile = Join-Path $root 'deploy\env\profile-dev.env'
    if (Test-Path $devProfile) { $sources += $devProfile }
    $releaseBundle = Join-Path $root 'deploy\env\release-bundle.env'
    if (Test-Path $releaseBundle) { $sources += $releaseBundle }
    $oauthBundle = Join-Path $root 'deploy\env\oauth-bundle.env'
    if (Test-Path $oauthBundle) { $sources += $oauthBundle }
    $local = Join-Path $root '.env.local'
    if (Test-Path $local) { $sources += $local }
    $priorInstallId = $null
    if (Test-Path $OutFile) {
        $priorVals = Read-EnvKeyValueFile -Path $OutFile
        $priorInstallId = $priorVals['STREAMCLONE_INSTALL_ID']
    }
    Merge-EnvFiles -OutFile $OutFile -Sources $sources
    Set-EnvFileValue -Path $OutFile -Key 'STREAMCLONE_PROFILE' -Value $Profile
    Invoke-EnvGenerateSecrets -EnvFile $OutFile
    Invoke-EnvApplyReleaseImageTag -EnvFile $OutFile
    $installId = if ($priorInstallId) { $priorInstallId } else { [Guid]::NewGuid().ToString('N') }
    Set-EnvFileValue -Path $OutFile -Key 'STREAMCLONE_INSTALL_ID' -Value $installId
    Ensure-LocalhostDevTokenImport -EnvFile $OutFile | Out-Null
    Try-SyncTwitchOAuthToEnv -EnvFile $OutFile | Out-Null
}

function Test-EnvLoopbackPublicOrigin {
    param([hashtable]$EnvValues = @{})
    foreach ($key in @('PUBLIC_ORIGIN', 'FRONTEND_ORIGIN')) {
        $origin = [string]$EnvValues[$key]
        if ([string]::IsNullOrWhiteSpace($origin)) { continue }
        if ($origin -match '^https?://(localhost|127\.0\.0\.1)(:\d+)?(/|$)') {
            return $true
        }
    }
    return $false
}

function Ensure-LocalhostDevTokenImport {
    param([string]$EnvFile = (Join-Path (Get-EnvRepoRoot) '.env'))
    if (-not (Test-Path $EnvFile)) { return $false }
    $vals = Read-EnvKeyValueFile -Path $EnvFile
    if ($vals['TWITCH_DEV_TOKEN_IMPORT_ENABLED'] -eq 'false') { return $false }
    $loopback = Test-EnvLoopbackPublicOrigin -EnvValues $vals
    $before = [string]$vals['TWITCH_DEV_TOKEN_IMPORT_ENABLED']
    if (-not $loopback) { return $false }
    if ($before -eq 'true') { return $false }
    Set-EnvFileValue -Path $EnvFile -Key 'TWITCH_DEV_TOKEN_IMPORT_ENABLED' -Value 'true'
    return $true
}

function Get-StreamcloneWelcomeUrl {
    param([string]$Base = '')
    if ([string]::IsNullOrWhiteSpace($Base)) {
        $Base = Get-StreamcloneAppUrl
    }
    return ($Base.TrimEnd('/') + '/?welcome=1')
}

function Get-StreamcloneAppUrl {
    param([string]$Path = '/')
    if (-not $Path.StartsWith('/')) { $Path = "/$Path" }
    # wslrelay binds [::1]:8090 on Windows and breaks http://localhost:8090 in browsers.
    return "http://127.0.0.1:8090$Path"
}

function Test-StreamcloneWebReachable {
    param(
        [string]$Url,
        [int]$TimeoutSec = 5
    )
    try {
        $resp = Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec $TimeoutSec
        return ($resp.StatusCode -ge 200 -and $resp.StatusCode -lt 500)
    } catch {
        return $false
    }
}

function Ensure-StreamcloneInstallId {
    param([string]$EnvFile = (Join-Path (Get-EnvRepoRoot) '.env'))
    if (-not (Test-Path $EnvFile)) { return $null }
    $vals = Read-EnvKeyValueFile -Path $EnvFile
    if (-not [string]::IsNullOrWhiteSpace($vals['STREAMCLONE_INSTALL_ID'])) {
        return $vals['STREAMCLONE_INSTALL_ID']
    }
    $installId = [Guid]::NewGuid().ToString('N')
    Set-EnvFileValue -Path $EnvFile -Key 'STREAMCLONE_INSTALL_ID' -Value $installId
    return $installId
}

function Get-EnvScraperSiblingPath {
    $root = Get-EnvRepoRoot
    return (Resolve-Path (Join-Path $root '..')).Path + '\streamclone-scraper'
}

function Test-EnvPreflightDocker {
    $docker = Get-EnvDockerExe
    if (-not $docker) {
        throw "Docker is required. Install Docker Desktop and ensure 'docker' is on PATH."
    }
    $info = Invoke-EnvDockerCaptured -Arguments @('info')
    if ($info.ExitCode -ne 0) {
        throw "Docker is installed but the engine is not running. Start Docker Desktop."
    }
    $compose = Invoke-EnvDockerCaptured -Arguments @('compose', 'version')
    if ($compose.ExitCode -ne 0) {
        throw "docker compose is required. Update Docker Desktop."
    }
}

function Get-EnvDockerExe {
    $candidates = @()
    if ($env:ProgramFiles) {
        $candidates += (Join-Path $env:ProgramFiles 'Docker\Docker\resources\bin\docker.exe')
    }
    $cmd = Get-Command docker.exe -ErrorAction SilentlyContinue
    if ($cmd) { $candidates += $cmd.Source }
    $cmd = Get-Command docker -ErrorAction SilentlyContinue
    if ($cmd -and [IO.Path]::GetExtension($cmd.Source) -ieq '.exe') {
        $candidates += $cmd.Source
    }
    foreach ($path in ($candidates | Where-Object { $_ } | Select-Object -Unique)) {
        if (Test-Path $path) {
            $resolved = (Resolve-Path -LiteralPath $path).Path
            $bin = Split-Path -Parent $resolved
            $pathParts = @($env:PATH -split ';' | Where-Object { $_ })
            if (($pathParts | Where-Object { $_ -ieq $bin }).Count -eq 0) {
                $env:PATH = "$bin;$env:PATH"
            }
            if ($env:PATHEXT -notmatch '(?i)\.EXE') {
                $env:PATHEXT = ".COM;.EXE;.BAT;.CMD;$env:PATHEXT"
            }
            return $resolved
        }
    }
    return $null
}

function Join-EnvProcessArguments {
    param([string[]]$Arguments)
    $parts = foreach ($arg in $Arguments) {
        $s = [string]$arg
        if ($s -match '[\s"]') {
            '"' + ($s -replace '"', '\"') + '"'
        } else {
            $s
        }
    }
    return ($parts -join ' ')
}

function Get-EnvProcessWorkingDirectory {
    try {
        $location = Get-Location
        if ($location.Provider.Name -eq 'FileSystem' -and $location.ProviderPath) {
            return $location.ProviderPath
        }
    } catch { }
    return [System.Environment]::CurrentDirectory
}

function Invoke-EnvDocker {
    param([string[]]$Arguments)
    $docker = Get-EnvDockerExe
    if (-not $docker) {
        Write-Error "Docker is required. Install Docker Desktop and ensure 'docker.exe' is on PATH."
        return 127
    }
    $psi = [System.Diagnostics.ProcessStartInfo]::new()
    $psi.FileName = $docker
    $psi.Arguments = Join-EnvProcessArguments -Arguments $Arguments
    $psi.UseShellExecute = $false
    $psi.CreateNoWindow = $true
    $psi.WorkingDirectory = Get-EnvProcessWorkingDirectory
    $proc = [System.Diagnostics.Process]::Start($psi)
    $proc.WaitForExit()
    return [int]$proc.ExitCode
}

function Invoke-EnvCapturedProcess {
    param(
        [Parameter(Mandatory = $true)][string]$FilePath,
        [string[]]$ArgumentList = @(),
        [int]$TimeoutSec = 0
    )
    $proc = $null
    try {
        $psi = [System.Diagnostics.ProcessStartInfo]::new()
        $psi.FileName = $FilePath
        $psi.Arguments = Join-EnvProcessArguments -Arguments $ArgumentList
        $psi.UseShellExecute = $false
        $psi.CreateNoWindow = $true
        $psi.WorkingDirectory = Get-EnvProcessWorkingDirectory
        $psi.RedirectStandardOutput = $true
        $psi.RedirectStandardError = $true

        $proc = [System.Diagnostics.Process]::Start($psi)
        $timedOut = $false
        if ($TimeoutSec -gt 0) {
            $timedOut = -not $proc.WaitForExit($TimeoutSec * 1000)
            if ($timedOut) {
                try { $proc.Kill() } catch { }
                return [pscustomobject]@{
                    ExitCode = 124
                    TimedOut = $true
                    Output   = @("Process timed out after ${TimeoutSec}s")
                }
            }
        } else {
            $proc.WaitForExit()
        }

        $stdout = $proc.StandardOutput.ReadToEnd()
        $stderr = $proc.StandardError.ReadToEnd()

        $lines = [System.Collections.Generic.List[string]]::new()
        foreach ($chunk in @($stdout, $stderr)) {
            if ([string]::IsNullOrEmpty($chunk)) { continue }
            foreach ($line in ($chunk -split "`r?`n")) {
                if ($line -ne '') { [void]$lines.Add($line) }
            }
        }

        return [pscustomobject]@{
            ExitCode = [int]$proc.ExitCode
            TimedOut = $false
            Output   = @($lines)
        }
    } finally {
        if ($proc) {
            $proc.Dispose()
        }
    }
}

function Test-StreamcloneUseWslDockerCli {
    param([string]$Root = '')
    if ($env:WSL_DISTRO_NAME) { return $false }
    if (-not (Get-Command wsl.exe -ErrorAction SilentlyContinue)) { return $false }
    try {
        $wslProxy = (& wsl.exe docker ps --filter 'name=streamclone-local-proxy' --format '{{.Names}}' 2>$null | Select-Object -First 1)
        $winProxy = (& docker ps --filter 'name=streamclone-local-proxy' --format '{{.Names}}' 2>$null | Select-Object -First 1)
        $wslProxy = if ($wslProxy) { "$wslProxy".Trim() } else { '' }
        $winProxy = if ($winProxy) { "$winProxy".Trim() } else { '' }
        if ($wslProxy -and -not $winProxy) { return $true }
        if ($wslProxy -and $winProxy -and $wslProxy -ne $winProxy) { return $true }
    } catch { }
    return $false
}

function Get-StreamcloneWslRootPath {
    param([string]$Root)
    if ([string]::IsNullOrWhiteSpace($Root)) { return $null }
    $full = [System.IO.Path]::GetFullPath($Root)
    if ($full -match '^([A-Za-z]):\\(.*)$') {
        $drive = $Matches[1].ToLower()
        $rest = ($Matches[2] -replace '\\', '/').TrimEnd('/')
        return "/mnt/$drive/$rest"
    }
    $trimmed = (& wsl.exe -- wslpath -a $full 2>$null | Select-Object -First 1)
    if ($trimmed) { return "$trimmed".Trim() }
    return $null
}

function Invoke-EnvDockerCaptured {
    param(
        [string[]]$Arguments,
        [string]$Root = ''
    )
    if ([string]::IsNullOrWhiteSpace($Root)) {
        try {
            $location = Get-Location
            if ($location.Provider.Name -eq 'FileSystem' -and $location.ProviderPath) {
                $Root = $location.ProviderPath
            }
        } catch { }
    }
    if (Test-StreamcloneUseWslDockerCli -Root $Root) {
        $wslRoot = Get-StreamcloneWslRootPath -Root $Root
        if ($wslRoot) {
            $bashCmd = "cd $(($wslRoot -replace "'", "'\\''")) && docker $(Join-EnvProcessArguments -Arguments $Arguments)"
            return Invoke-EnvCapturedProcess -FilePath 'wsl.exe' -ArgumentList @('bash', '-lc', $bashCmd)
        }
    }
    $docker = Get-EnvDockerExe
    if (-not $docker) {
        return [pscustomobject]@{
            ExitCode = 127
            TimedOut = $false
            Output   = @("Docker is required. Install Docker Desktop and ensure 'docker.exe' is on PATH.")
        }
    }
    return Invoke-EnvCapturedProcess -FilePath $docker -ArgumentList $Arguments
}

function Test-StreamcloneDockerPullDisplayLine {
    param([string]$Line)
    $text = "$Line".Trim()
    if ($text -eq '') { return $false }
    # Layer progress spam (no TTY when launched from Install .cmd via PowerShell).
    if ($text -match '^[a-f0-9]{12}\s') { return $false }
    if ($text -match '^[a-f0-9]{64}\s') { return $false }
    return $true
}

function Get-StreamcloneComposePullImageCount {
    param([string[]]$ComposeArgs)
    $result = Invoke-EnvDockerCaptured -Arguments ($ComposeArgs + @('config', '--images'))
    if ($result.ExitCode -ne 0 -or -not $result.Output) { return 13 }
    $count = @($result.Output | Where-Object { -not [string]::IsNullOrWhiteSpace($_) }).Count
    if ($count -lt 1) { return 13 }
    return $count
}

function Write-StreamcloneFriendlyPullBanner {
    Write-Host ''
    Write-Host 'Downloading Streamclone images (~1.5 GB)...' -ForegroundColor Cyan
    Write-Host 'First install usually takes 3-8 minutes. Please wait.' -ForegroundColor DarkGray
    Write-Host ''
}

function Write-StreamcloneFriendlyPullBar {
    param(
        [int]$Percent,
        [string]$Status = 'Downloading'
    )
    $pct = [math]::Max(0, [math]::Min(100, $Percent))
    $width = 30
    $filled = [math]::Floor($width * $pct / 100.0)
    if ($filled -lt 0) { $filled = 0 }
    if ($filled -gt $width) { $filled = $width }
    $bar = ('=' * $filled) + ('>' * [math]::Min(1, $width - $filled)) + (' ' * [math]::Max(0, $width - $filled - 1))
    $label = $Status
    if ($label.Length -gt 36) { $label = $label.Substring(0, 33) + '...' }
    Write-Host ("`r  [{0}] {1,3}%  {2,-36}" -f $bar, $pct, $label) -NoNewline
}

function Update-StreamcloneFriendlyPullFromLine {
    param(
        [string]$Line,
        [hashtable]$State
    )
    $text = "$Line".Trim()
    if ($text -eq '') { return }

    if ($text -match '\[\+\]\s+Pulling\s+(\d+)/(\d+)') {
        $current = [int]$matches[1]
        $total = [int]$matches[2]
        if ($total -gt 0) {
            $State.total = $total
            $State.pulled = $current
            $State.percent = [math]::Min(99, [math]::Floor(100.0 * $current / $total))
            $State.status = "Image $current of $total"
        }
        return
    }

    if ($text -match '(?:streamclone-)?([A-Za-z0-9_.-]+)\s+Pulled\b') {
        $State.pulled = [math]::Min($State.total, $State.pulled + 1)
        $State.status = "Finished $($matches[1])"
        if ($State.total -gt 0) {
            $State.percent = [math]::Min(99, [math]::Floor(100.0 * $State.pulled / $State.total))
        }
        return
    }

    if ($text -match 'already up to date|Image is up to date') {
        $State.pulled = [math]::Min($State.total, $State.pulled + 1)
        $State.status = 'Image up to date'
        if ($State.total -gt 0) {
            $State.percent = [math]::Min(99, [math]::Floor(100.0 * $State.pulled / $State.total))
        }
        return
    }

    if ($text -match 'Pulling|Downloading') {
        $now = Get-Date
        if (($now - $State.lastBump).TotalSeconds -ge 4 -and $State.percent -lt 90) {
            $State.percent = [math]::Min(90, $State.percent + 1)
            $State.lastBump = $now
            $State.status = 'Downloading layers'
        }
    }
}

function Invoke-EnvDockerStreaming {
    param(
        [Parameter(Mandatory = $true)][string[]]$Arguments,
        [scriptblock]$OnLine = $null,
        [ValidateSet('interactive', 'capture', 'summary', 'friendly')][string]$OutputMode = 'capture',
        [hashtable]$FriendlyState = $null
    )
    $docker = Get-EnvDockerExe
    if (-not $docker) {
        return [pscustomobject]@{
            ExitCode = 127
            Output   = @('Docker is required. Install Docker Desktop and ensure docker.exe is on PATH.')
        }
    }

    $prev = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    $lines = [System.Collections.Generic.List[string]]::new()
    $wd = Get-EnvProcessWorkingDirectory
    Push-Location $wd
    try {
        if ($OutputMode -eq 'interactive') {
            & $docker @Arguments
            $code = if ($null -ne $LASTEXITCODE -and $LASTEXITCODE -ne 0) { [int]$LASTEXITCODE } else { 0 }
            if (-not $? -and $code -eq 0) { $code = 1 }
        } else {
            $lastLayerHint = [DateTime]::MinValue
            & $docker @Arguments 2>&1 | ForEach-Object {
                $line = "$_".TrimEnd()
                if ($line -eq '') { return }
                [void]$lines.Add($line)
                $display = $true
                if ($OutputMode -eq 'friendly') {
                    $display = $false
                    if ($FriendlyState) {
                        Update-StreamcloneFriendlyPullFromLine -Line $line -State $FriendlyState
                        Write-StreamcloneFriendlyPullBar -Percent $FriendlyState.percent -Status $FriendlyState.status
                    }
                } elseif ($OutputMode -eq 'summary') {
                    $display = Test-StreamcloneDockerPullDisplayLine -Line $line
                    if (-not $display) {
                        $now = Get-Date
                        if (($now - $lastLayerHint).TotalSeconds -ge 8) {
                            Write-Host '  Downloading image layers...' -ForegroundColor DarkGray
                            $lastLayerHint = $now
                        }
                    }
                }
                if ($display) {
                    Write-Host $line
                }
                if ($OnLine) {
                    try { & $OnLine $line } catch { }
                }
            }
            if ($OutputMode -eq 'friendly' -and $FriendlyState) {
                Write-StreamcloneFriendlyPullBar -Percent 100 -Status 'Download complete'
                Write-Host ''
            }
            $code = if ($null -ne $LASTEXITCODE -and $LASTEXITCODE -ne 0) { [int]$LASTEXITCODE } else { 0 }
            if (-not $? -and $code -eq 0) { $code = 1 }
        }
    } finally {
        Pop-Location
        $ErrorActionPreference = $prev
    }

    return [pscustomobject]@{
        ExitCode = $code
        Output   = @($lines.ToArray())
    }
}

function Invoke-EnvDockerComposePullWithRetry {
    param(
        [Parameter(Mandatory = $true)][string[]]$ComposeArgs,
        [int]$MaxAttempts = 3,
        [int]$RetryDelaySec = 10,
        [scriptblock]$OnLine = $null,
        [ValidateSet('interactive', 'capture', 'summary', 'friendly')][string]$OutputMode = 'friendly'
    )
    $last = $null
    $friendlyState = $null
    if ($OutputMode -eq 'friendly') {
        $friendlyState = @{
            total   = (Get-StreamcloneComposePullImageCount -ComposeArgs $ComposeArgs)
            pulled  = 0
            percent = 0
            status  = 'Starting'
            lastBump = [DateTime]::MinValue
        }
        Write-StreamcloneFriendlyPullBanner
        Write-StreamcloneFriendlyPullBar -Percent 0 -Status 'Starting'
    }
    for ($attempt = 1; $attempt -le $MaxAttempts; $attempt++) {
        if ($attempt -gt 1) {
            Write-Host ''
            Write-Host "Retrying docker compose pull (attempt $attempt/$MaxAttempts)..." -ForegroundColor Yellow
        }
        $last = Invoke-EnvDockerStreaming -Arguments ($ComposeArgs + @('pull')) -OnLine $OnLine -OutputMode $OutputMode -FriendlyState $friendlyState
        if ($last.ExitCode -eq 0) {
            if ($attempt -gt 1) {
                Write-Host "docker compose pull succeeded on attempt $attempt." -ForegroundColor Green
            }
            return $last
        }
        if ($attempt -lt $MaxAttempts) {
            Write-Host "docker compose pull failed (attempt $attempt/$MaxAttempts, exit $($last.ExitCode)). Retrying in ${RetryDelaySec}s..." -ForegroundColor Yellow
            if ($last.Output) {
                $last.Output | Select-Object -Last 5 | ForEach-Object { Write-Host "  $_" -ForegroundColor DarkYellow }
            }
            Start-Sleep -Seconds $RetryDelaySec
        }
    }
    return $last
}

function Invoke-EnvDockerCapturedWithTimeout {
    param(
        [string[]]$Arguments,
        [int]$TimeoutSec = 15
    )
    $docker = Get-EnvDockerExe
    if (-not $docker) {
        return [pscustomobject]@{
            ExitCode = 127
            TimedOut = $false
            Output   = @('Docker CLI not found')
        }
    }
    return Invoke-EnvCapturedProcess -FilePath $docker -ArgumentList $Arguments -TimeoutSec $TimeoutSec
}

function Sync-ClipperAuthFromRuntime {
    param(
        [string]$Root = (Get-EnvRepoRoot),
        [string]$EnvFile = ''
    )
    if ([string]::IsNullOrWhiteSpace($EnvFile)) {
        $EnvFile = Join-Path $Root '.env'
    }
    $syncFile = Join-Path $Root 'runtime\clipper-twitch.env'
    if (-not (Test-Path $syncFile)) {
        return $false
    }
    $syncValues = Read-EnvKeyValueFile -Path $syncFile
    $updated = $false
    foreach ($key in @('CLIPPER_TWITCH_CLIENT_ID', 'CLIPPER_TWITCH_USER_ACCESS_TOKEN', 'CLIPPER_TWITCH_REFRESH_TOKEN')) {
        $value = [string]$syncValues[$key]
        if (-not [string]::IsNullOrWhiteSpace($value)) {
            Set-EnvFileValue -Path $EnvFile -Key $key -Value $value
            $updated = $true
        }
    }
    if ($updated) {
        $oauthId = (Read-EnvKeyValueFile -Path $EnvFile)['TWITCH_OAUTH_CLIENT_ID']
        if (-not [string]::IsNullOrWhiteSpace($oauthId) -and [string]::IsNullOrWhiteSpace($syncValues['CLIPPER_TWITCH_CLIENT_ID'])) {
            Set-EnvFileValue -Path $EnvFile -Key 'CLIPPER_TWITCH_CLIENT_ID' -Value $oauthId
        }
    }
    return $updated
}

function Get-TwitchCliConfigPath {
    $candidates = @(
        (Join-Path $env:APPDATA 'twitch-cli\.twitch-cli.env'),
        (Join-Path $env:USERPROFILE '.config\twitch-cli\.twitch-cli.env')
    )
    foreach ($path in $candidates) {
        if (Test-Path $path) { return $path }
    }
    return $null
}

function Sync-TwitchCliToEnv {
    param([string]$EnvFile = (Join-Path (Get-EnvRepoRoot) '.env'))
    $cliConfig = Get-TwitchCliConfigPath
    if (-not $cliConfig) {
        throw "Twitch CLI config not found. Run: twitch configure"
    }
    $cli = Read-EnvKeyValueFile -Path $cliConfig
    if ([string]::IsNullOrWhiteSpace($cli['CLIENTID']) -or [string]::IsNullOrWhiteSpace($cli['CLIENTSECRET'])) {
        throw "Twitch CLI config missing CLIENTID or CLIENTSECRET."
    }
    Set-EnvFileValue -Path $EnvFile -Key 'TWITCH_OAUTH_CLIENT_ID' -Value $cli['CLIENTID']
    Set-EnvFileValue -Path $EnvFile -Key 'TWITCH_OAUTH_CLIENT_SECRET' -Value $cli['CLIENTSECRET']
}

function Try-SyncTwitchOAuthToEnv {
    param([string]$EnvFile = (Join-Path (Get-EnvRepoRoot) '.env'))
    if (-not (Test-Path $EnvFile)) { return $false }
    $vals = Read-EnvKeyValueFile -Path $EnvFile
    if (-not [string]::IsNullOrWhiteSpace($vals['TWITCH_OAUTH_CLIENT_ID']) -and
        -not [string]::IsNullOrWhiteSpace($vals['TWITCH_OAUTH_CLIENT_SECRET'])) {
        return $false
    }
    try {
        Sync-TwitchCliToEnv -EnvFile $EnvFile
        return $true
    } catch {
        return $false
    }
}

function Invoke-EnsureFrontendClipperConfig {
    param(
        [string]$EnvFile = (Join-Path (Get-EnvRepoRoot) '.env'),
        [switch]$SkipRecreate
    )

    if (-not (Test-Path $EnvFile)) {
        Write-Host 'ensure-frontend-config: missing .env - skip'
        return $false
    }

    . (Join-Path $PSScriptRoot 'stack-progress.ps1')

    Invoke-EnvGenerateSecrets -EnvFile $EnvFile
    Repair-FrontendDockerEntrypointLf

    $envValues = Read-EnvKeyValueFile -Path $EnvFile
    $desiredToken = [string]$envValues['VITE_CLIPPER_TOKEN']
    if ([string]::IsNullOrWhiteSpace($desiredToken)) {
        $desiredToken = [string]$envValues['CLIPPER_WEBHOOK_TOKEN']
    }
    if ([string]::IsNullOrWhiteSpace($desiredToken)) {
        Write-Host 'ensure-frontend-config: CLIPPER_WEBHOOK_TOKEN missing - run setup or validate-env.ps1 -Fix'
        return $false
    }

    $profile = [string]$envValues['STREAMCLONE_PROFILE']
    if ([string]::IsNullOrWhiteSpace($profile)) { $profile = 'core' }
    $useImages = ($envValues['STREAMCLONE_USE_IMAGES'] -eq '1') -or (-not [string]::IsNullOrWhiteSpace($envValues['IMAGE_TAG']))
    $root = Get-EnvRepoRoot
    $composeArgs = Get-StreamcloneComposeArgs -Root $root -Profile $profile -UseImages:$useImages

    $containerName = $null
    $psResult = Invoke-EnvDockerCaptured -Arguments ($composeArgs + @('ps', '--filter', 'name=streamclone-frontend', '--format', '{{.Names}}'))
    if ($psResult.ExitCode -eq 0 -and $psResult.Output) {
        $containerName = ($psResult.Output | Select-Object -First 1).Trim()
    }

    if ([string]::IsNullOrWhiteSpace($containerName)) {
        Write-Host 'ensure-frontend-config: frontend container not running - skip'
        return $true
    }

    $containerToken = ''
    $inspect = Invoke-EnvDockerCaptured -Arguments @(
        'inspect', $containerName, '--format', '{{range .Config.Env}}{{println .}}{{end}}'
    )
    if ($inspect.ExitCode -eq 0) {
        foreach ($line in $inspect.Output) {
            if ($line.StartsWith('VITE_CLIPPER_TOKEN=')) {
                $containerToken = $line.Substring('VITE_CLIPPER_TOKEN='.Length)
                break
            }
        }
    }

    $configHasToken = $false
    $configResult = Invoke-EnvDockerCaptured -Arguments @(
        'exec', $containerName, 'grep', '-q', 'clipperToken:', '/usr/share/nginx/html/config.js'
    )
    if ($configResult.ExitCode -eq 0) {
        $readConfig = Invoke-EnvDockerCaptured -Arguments @(
            'exec', $containerName, 'grep', 'clipperToken:', '/usr/share/nginx/html/config.js'
        )
        if ($readConfig.ExitCode -eq 0 -and $readConfig.Output) {
            $configLine = ($readConfig.Output | Select-Object -First 1)
            $configHasToken = ($configLine -match 'clipperToken:\s*"(.+)"') -and (-not [string]::IsNullOrWhiteSpace($Matches[1]))
        }
    }

    $needsRecreate = ($containerToken -ne $desiredToken) -or (-not $configHasToken)
    if ($SkipRecreate -or -not $needsRecreate) {
        if (-not $needsRecreate) {
            Write-Host 'ensure-frontend-config: frontend clipper token and config.js match .env'
        }
        return (-not $needsRecreate)
    }

    Write-Host 'ensure-frontend-config: recreating frontend to refresh config.js and clipper token...'
    $code = Invoke-EnvDocker -Arguments ($composeArgs + @('up', '-d', '--no-deps', '--force-recreate', 'frontend', 'local-proxy'))
    if ($code -ne 0) {
        Write-Host "ensure-frontend-config: frontend recreate failed (exit $code)" -ForegroundColor Red
        return $false
    }

    Write-Host 'ensure-frontend-config: frontend recreated (hard-refresh the browser if it was already open)'
    return $true
}

function Test-StreamcloneCaddyfileLocalTunnelPath {
    param([string]$Root = '')
    if ([string]::IsNullOrWhiteSpace($Root)) { $Root = Get-EnvRepoRoot }
    $path = Join-Path $Root 'deploy\Caddyfile.local-tunnel'
    if (-not (Test-Path -LiteralPath $path)) {
        return [pscustomobject]@{ ok = $false; reason = 'missing'; path = $path }
    }
    $item = Get-Item -LiteralPath $path -Force
    if ($item.PSIsContainer) {
        return [pscustomobject]@{ ok = $false; reason = 'directory'; path = $path }
    }
    return [pscustomobject]@{ ok = $true; reason = 'file'; path = $path }
}

function Repair-StreamcloneCaddyfileLocalTunnel {
    param(
        [string]$Root = '',
        [string]$Repo = 'Aron-Chu/streamclone'
    )
    if ([string]::IsNullOrWhiteSpace($Root)) { $Root = Get-EnvRepoRoot }
    $state = Test-StreamcloneCaddyfileLocalTunnelPath -Root $Root
    if ($state.ok) { return $true }

    Write-Host "Repairing deploy/Caddyfile.local-tunnel ($($state.reason) - breaks Caddy on :8090)..." -ForegroundColor Yellow
    if (Test-Path -LiteralPath $state.path) {
        Remove-Item -LiteralPath $state.path -Recurse -Force
    }
    $deployDir = Split-Path $state.path -Parent
    if (-not (Test-Path $deployDir)) {
        New-Item -ItemType Directory -Force -Path $deployDir | Out-Null
    }

    $headers = @{ 'User-Agent' = 'streamclone-bootstrap' }
    $sha = (Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/commits/master" -Headers $headers).sha
    $url = "https://raw.githubusercontent.com/$Repo/$sha/deploy/Caddyfile.local-tunnel"
    Invoke-WebRequest -Uri $url -OutFile $state.path -Headers $headers -UseBasicParsing

    $after = Test-StreamcloneCaddyfileLocalTunnelPath -Root $Root
    if (-not $after.ok) {
        throw 'Caddyfile.local-tunnel repair failed - proxy on :8090 will not start.'
    }
    return $true
}
