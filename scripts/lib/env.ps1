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
        'clipper' { return @('clipper') }
        'full' { return @('scraper', 'clipper') }
    }
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
        (Join-Path $root '.env.dev'),
        (Get-EnvProfileFragment -Profile $Profile)
    )
    $releaseBundle = Join-Path $root 'deploy\env\release-bundle.env'
    if (Test-Path $releaseBundle) { $sources += $releaseBundle }
    $oauthBundle = Join-Path $root 'deploy\env\oauth-bundle.env'
    if (Test-Path $oauthBundle) { $sources += $oauthBundle }
    $local = Join-Path $root '.env.local'
    if (Test-Path $local) { $sources += $local }
    Merge-EnvFiles -OutFile $OutFile -Sources $sources
    Set-EnvFileValue -Path $OutFile -Key 'STREAMCLONE_PROFILE' -Value $Profile
    Invoke-EnvGenerateSecrets -EnvFile $OutFile
    Invoke-EnvApplyReleaseImageTag -EnvFile $OutFile
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

function Invoke-EnvDockerCaptured {
    param([string[]]$Arguments)
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

function Invoke-EnvDockerStreaming {
    param(
        [Parameter(Mandatory = $true)][string[]]$Arguments,
        [scriptblock]$OnLine = $null,
        [ValidateSet('interactive', 'capture', 'summary')][string]$OutputMode = 'capture'
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
                if ($OutputMode -eq 'summary') {
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
        [ValidateSet('interactive', 'capture', 'summary')][string]$OutputMode = 'summary'
    )
    $last = $null
    for ($attempt = 1; $attempt -le $MaxAttempts; $attempt++) {
        if ($attempt -gt 1) {
            Write-Host "Retrying docker compose pull (attempt $attempt/$MaxAttempts)..." -ForegroundColor Yellow
        }
        $last = Invoke-EnvDockerStreaming -Arguments ($ComposeArgs + @('pull')) -OnLine $OnLine -OutputMode $OutputMode
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
