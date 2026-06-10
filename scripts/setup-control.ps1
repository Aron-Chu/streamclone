#Requires -Version 5.1
# Local-only HTTP helper so the welcome/status UI can start optional compose profiles.
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

function Invoke-ProfileServiceUp {
    param(
        [ValidateSet('scraper', 'clipper')]
        [string]$Service
    )

    Set-Location $Root
    if (-not (Test-Path (Join-Path $Root '.env'))) {
        throw 'Missing .env — run scripts/setup.ps1 first.'
    }

    $profile = $Service
    if ($Service -eq 'scraper') {
        $sibling = Get-EnvScraperSiblingPath
        if (-not ((Test-Path (Join-Path $sibling '.git')) -or (Test-Path (Join-Path $sibling 'Dockerfile')))) {
            throw "streamclone-scraper sibling repo missing at $sibling"
        }
    }

    $composeArgs = @(
        'compose', '--env-file', '.env',
        '-f', 'deploy/docker-compose.yml',
        '-f', 'deploy/docker-compose.local-tunnel.yml',
        '--profile', $profile,
        'up', '-d', '--remove-orphans', $Service
    )
    $output = & docker @composeArgs 2>&1 | Out-String
    if ($LASTEXITCODE -ne 0) {
        throw "docker compose failed: $output"
    }
    return $output.Trim()
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
            $response.Headers.Add('Access-Control-Allow-Headers', 'Content-Type')
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
