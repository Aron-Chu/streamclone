#Requires -Version 5.1
# Local-only HTTP helper so the directory status UI can start optional compose profiles.
param(
    [int]$Port = 9191,
    [string]$PidFile = ''
)

$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent $PSScriptRoot
if ([string]::IsNullOrWhiteSpace($PidFile)) {
    $PidFile = Join-Path $Root '.streamclone-setup-control.pid'
}

. (Join-Path $PSScriptRoot 'lib\env.ps1')
. (Join-Path $PSScriptRoot 'lib\stack-progress.ps1')
. (Join-Path $PSScriptRoot 'lib\diagnostics.ps1')

$envPath = Join-Path $Root '.env'
$envValues = if (Test-Path $envPath) { Read-EnvKeyValueFile -Path $envPath } else { @{} }
$setupControlToken = [string]$envValues['SETUP_CONTROL_TOKEN']

function Sync-SetupControlTokenFromEnv {
    if (-not (Test-Path $envPath)) { return }
    $script:envValues = Read-EnvKeyValueFile -Path $envPath
    $script:setupControlToken = [string]$envValues['SETUP_CONTROL_TOKEN']
}

function Write-JsonResponse {
    param(
        [System.Net.HttpListenerResponse]$Response,
        [int]$StatusCode,
        [object]$Body
    )
    $json = ($Body | ConvertTo-Json -Compress -Depth 6)
    $bytes = [System.Text.Encoding]::UTF8.GetBytes($json)
    $Response.StatusCode = $StatusCode
    $Response.ContentType = 'application/json; charset=utf-8'
    $Response.Headers.Add('Access-Control-Allow-Origin', '*')
    $Response.ContentLength64 = $bytes.Length
    $Response.OutputStream.Write($bytes, 0, $bytes.Length)
    $Response.Close()
}

function Test-SetupControlAuthorized {
    param([System.Net.HttpListenerRequest]$Request)
    Sync-SetupControlTokenFromEnv
    if ([string]::IsNullOrWhiteSpace($setupControlToken)) {
        return $false
    }
    $provided = $Request.Headers['X-Streamclone-Setup-Token']
    if ([string]::IsNullOrWhiteSpace($provided)) { return $false }
    return ($provided -eq $setupControlToken)
}

function Invoke-ProfileServiceUp {
    param(
        [ValidateSet('scraper', 'clipper')]
        [string]$Service
    )

    Set-Location $Root
    if (-not (Test-Path $envPath)) {
        throw 'Missing .env — run scripts/setup.ps1 first.'
    }

    $profile = $Service
    if ($Service -eq 'scraper') {
        $sibling = Get-EnvScraperSiblingPath
        $hasRepo = (Test-Path (Join-Path $sibling '.git')) -or (Test-Path (Join-Path $sibling 'Dockerfile'))
        if (-not $hasRepo) {
            Write-Host "Cloning streamclone-scraper to $sibling ..."
            $parent = Split-Path -Parent $sibling
            New-Item -ItemType Directory -Path $parent -Force | Out-Null
            $clone = Invoke-EnvCapturedProcess -FilePath 'git' -ArgumentList @('clone', 'https://github.com/Aron-Chu/streamclone-scraper.git', $sibling) -TimeoutSec 300
            if ($clone.ExitCode -ne 0) {
                $log = ($clone.Output -join [Environment]::NewLine).Trim()
                throw "Could not clone streamclone-scraper: $log"
            }
        }
    }

    $useImages = ($envValues['STREAMCLONE_USE_IMAGES'] -eq '1') -or (-not [string]::IsNullOrWhiteSpace($envValues['IMAGE_TAG']))
    $composeArgs = Get-StreamcloneComposeArgs -Root $Root -Profile $profile -UseImages:$useImages
    $result = Invoke-EnvDockerCaptured -Arguments ($composeArgs + @('up', '-d', '--remove-orphans', $Service))
    $output = ($result.Output -join [Environment]::NewLine).Trim()
    if ($result.ExitCode -ne 0) {
        throw "docker compose failed: $output"
    }
    return $output
}

function Start-ProfileServiceUpAsync {
    param(
        [ValidateSet('scraper', 'clipper')]
        [string]$Service
    )

    Set-Location $Root
    if (-not (Test-Path $envPath)) {
        throw 'Missing .env — run scripts/setup.ps1 first.'
    }

    Sync-SetupControlTokenFromEnv

    if ($Service -eq 'scraper') {
        $sibling = Get-EnvScraperSiblingPath
        $hasRepo = (Test-Path (Join-Path $sibling '.git')) -or (Test-Path (Join-Path $sibling 'Dockerfile'))
        if (-not $hasRepo) {
            Write-Host "Cloning streamclone-scraper to $sibling ..."
            $parent = Split-Path -Parent $sibling
            New-Item -ItemType Directory -Path $parent -Force | Out-Null
            $clone = Invoke-EnvCapturedProcess -FilePath 'git' -ArgumentList @('clone', 'https://github.com/Aron-Chu/streamclone-scraper.git', $sibling) -TimeoutSec 300
            if ($clone.ExitCode -ne 0) {
                $log = ($clone.Output -join [Environment]::NewLine).Trim()
                throw "Could not clone streamclone-scraper: $log"
            }
        }
    }

    $useImages = ($envValues['STREAMCLONE_USE_IMAGES'] -eq '1') -or (-not [string]::IsNullOrWhiteSpace($envValues['IMAGE_TAG']))
    $composeArgs = Get-StreamcloneComposeArgs -Root $Root -Profile $Service -UseImages:$useImages
    $docker = Get-EnvDockerExe
    if (-not $docker) {
        throw "Docker is required. Install Docker Desktop and ensure 'docker.exe' is on PATH."
    }

    $logFile = Join-Path $Root ".streamclone-start-$Service.log"
    $args = $composeArgs + @('up', '-d', '--remove-orphans', $Service)

    $proc = Start-Process -FilePath $docker `
        -ArgumentList (Join-EnvProcessArguments -Arguments $args) `
        -WorkingDirectory $Root `
        -WindowStyle Hidden `
        -RedirectStandardOutput $logFile `
        -RedirectStandardError $logFile `
        -PassThru
    return "compose start initiated (pid $($proc.Id)); see $logFile"
}

function Invoke-SyncClipperAuth {
    Set-Location $Root
    if (-not (Test-Path $envPath)) {
        throw 'Missing .env — run scripts/setup.ps1 first.'
    }
    if (-not (Sync-ClipperAuthFromRuntime -Root $Root -EnvFile $envPath)) {
        return @{ ok = $true; merged = $false; message = 'no runtime clipper auth file yet' }
    }

    $script:envValues = Read-EnvKeyValueFile -Path $envPath
    $useImages = ($envValues['STREAMCLONE_USE_IMAGES'] -eq '1') -or (-not [string]::IsNullOrWhiteSpace($envValues['IMAGE_TAG']))
    $profile = [string]$envValues['STREAMCLONE_PROFILE']
    if ([string]::IsNullOrWhiteSpace($profile)) { $profile = 'core' }

    $clipperRunning = $false
    $prev = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        $psResult = Invoke-EnvDockerCaptured -Arguments @('ps', '--filter', 'name=streamclone-clipper', '--format', '{{.Names}}')
        if ($psResult.ExitCode -eq 0 -and $psResult.Output) {
            $clipperRunning = $true
        }
    } finally {
        $ErrorActionPreference = $prev
    }

    $log = ''
    if ($clipperRunning -or $profile -in @('clipper', 'full')) {
        $composeArgs = Get-StreamcloneComposeArgs -Root $Root -Profile $profile -UseImages:$useImages
        $result = Invoke-EnvDockerCaptured -Arguments ($composeArgs + @('up', '-d', '--no-deps', '--force-recreate', 'clipper'))
        $log = ($result.Output -join [Environment]::NewLine).Trim()
        if ($result.ExitCode -ne 0) {
            throw "clipper recreate failed: $log"
        }
    }

    return @{
        ok = $true
        merged = $true
        recreated = ($clipperRunning -or $profile -in @('clipper', 'full'))
        message = 'clipper credentials merged from sign-in'
        log = $log
    }
}

Set-Content -Path $PidFile -Value $PID -NoNewline

$listener = [System.Net.HttpListener]::new()
$listener.Prefixes.Add("http://127.0.0.1:$Port/")
$listener.Prefixes.Add("http://[::1]:$Port/")
$listener.Start()

try {
    while ($listener.IsListening) {
        $context = $listener.GetContext()
        $request = $context.Request
        $response = $context.Response
        $path = ($request.Url.AbsolutePath -replace '/+$', '')
        if ([string]::IsNullOrWhiteSpace($path)) { $path = '/' }

        if ($request.HttpMethod -eq 'OPTIONS') {
            $response.Headers.Add('Access-Control-Allow-Origin', '*')
            $response.Headers.Add('Access-Control-Allow-Methods', 'GET, POST, OPTIONS')
            $response.Headers.Add('Access-Control-Allow-Headers', 'Content-Type, X-Streamclone-Setup-Token')
            $response.StatusCode = 204
            $response.Close()
            continue
        }

        try {
            if ($request.HttpMethod -eq 'GET' -and ($path -eq '/health' -or $path -eq '/')) {
                Write-JsonResponse -Response $response -StatusCode 200 -Body @{ ok = $true; service = 'setup-control' }
                continue
            }

            if ($request.HttpMethod -eq 'GET' -and $path -eq '/diagnostics') {
                $report = Get-StreamcloneDiagnostics -Root $Root
                Write-JsonResponse -Response $response -StatusCode 200 -Body $report
                continue
            }

            if ($request.HttpMethod -eq 'POST' -and $path -match '^/start/(scraper|clipper)$') {
                if (-not (Test-SetupControlAuthorized -Request $request)) {
                    Write-JsonResponse -Response $response -StatusCode 401 -Body @{ ok = $false; error = 'unauthorized' }
                    continue
                }
                $service = $Matches[1]
                $log = Start-ProfileServiceUpAsync -Service $service
                Write-JsonResponse -Response $response -StatusCode 200 -Body @{ ok = $true; service = $service; message = 'starting'; log = $log }
                continue
            }

            if ($request.HttpMethod -eq 'POST' -and $path -eq '/sync-clipper-auth') {
                if (-not (Test-SetupControlAuthorized -Request $request)) {
                    Write-JsonResponse -Response $response -StatusCode 401 -Body @{ ok = $false; error = 'unauthorized' }
                    continue
                }
                $result = Invoke-SyncClipperAuth
                Write-JsonResponse -Response $response -StatusCode 200 -Body $result
                continue
            }

            Write-JsonResponse -Response $response -StatusCode 404 -Body @{ ok = $false; error = 'not_found' }
        } catch {
            Write-JsonResponse -Response $response -StatusCode 500 -Body @{ ok = $false; error = $_.Exception.Message }
        }
    }
} finally {
    if (Test-Path $PidFile) { Remove-Item $PidFile -Force -ErrorAction SilentlyContinue }
    $listener.Stop()
    $listener.Close()
}
