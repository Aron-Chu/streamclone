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

$envPath = Join-Path $Root '.env'
$envValues = if (Test-Path $envPath) { Read-EnvKeyValueFile -Path $envPath } else { @{} }
$setupControlToken = [string]$envValues['SETUP_CONTROL_TOKEN']

function Write-JsonResponse {
    param(
        [System.Net.HttpListenerResponse]$Response,
        [int]$StatusCode,
        [object]$Body
    )
    $json = ($Body | ConvertTo-Json -Compress)
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
        if (-not ((Test-Path (Join-Path $sibling '.git')) -or (Test-Path (Join-Path $sibling 'Dockerfile')))) {
            throw "streamclone-scraper sibling repo missing at $sibling"
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

            if ($request.HttpMethod -eq 'POST' -and $path -match '^/start/(scraper|clipper)$') {
                if (-not (Test-SetupControlAuthorized -Request $request)) {
                    Write-JsonResponse -Response $response -StatusCode 401 -Body @{ ok = $false; error = 'unauthorized' }
                    continue
                }
                $service = $Matches[1]
                $log = Invoke-ProfileServiceUp -Service $service
                Write-JsonResponse -Response $response -StatusCode 200 -Body @{ ok = $true; service = $service; message = 'started'; log = $log }
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
