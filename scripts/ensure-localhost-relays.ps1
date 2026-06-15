param(
    [string]$Ports = '8090',
    [string]$ConnectHost = ''
)

$ErrorActionPreference = 'Stop'

$Root = Resolve-Path (Join-Path $PSScriptRoot '..')
$RelayScript = Join-Path $PSScriptRoot 'tcp-relay.ps1'
$PidDir = Join-Path $Root '.tmp\localhost-relays'
New-Item -ItemType Directory -Force -Path $PidDir | Out-Null

function Get-WslPrimaryIp {
    try {
        $raw = & wsl.exe hostname -I 2>$null
        if ($LASTEXITCODE -eq 0 -and $raw) {
            return (($raw -split '\s+') | Where-Object { $_ -match '^\d+\.\d+\.\d+\.\d+$' } | Select-Object -First 1)
        }
    } catch {}
    return ''
}

function Test-HttpUrl {
    param([string]$Url)
    try {
        $response = Invoke-WebRequest -UseBasicParsing -Uri $Url -TimeoutSec 3
        return [int]$response.StatusCode
    } catch {
        if ($_.Exception.Response -and $_.Exception.Response.StatusCode) {
            return [int]$_.Exception.Response.StatusCode
        }
        return 0
    }
}

function Stop-Relay {
    param(
        [int]$Port,
        [ValidateSet('v4', 'v6')]
        [string]$Family = 'v4'
    )
    $pidFiles = @((Join-Path $PidDir "$Port-$Family.pid"))
    if ($Family -eq 'v4') {
        $pidFiles += (Join-Path $PidDir "$Port.pid")
    }
    foreach ($pidFile in $pidFiles) {
        if (Test-Path $pidFile) {
            $pidText = (Get-Content $pidFile -ErrorAction SilentlyContinue | Select-Object -First 1)
            if ($pidText -match '^\d+$') {
                Stop-Process -Id ([int]$pidText) -Force -ErrorAction SilentlyContinue
            }
            Remove-Item $pidFile -Force -ErrorAction SilentlyContinue
        }
    }
}

function Start-Relay {
    param(
        [int]$Port,
        [ValidateSet('v4', 'v6')]
        [string]$Family,
        [string]$ListenHost,
        [string]$ConnectHost
    )
    Stop-Relay -Port $Port -Family $Family

    $pidFile = Join-Path $PidDir "$Port-$Family.pid"
    $args = @(
        '-NoProfile',
        '-ExecutionPolicy', 'Bypass',
        '-File', $RelayScript,
        '-ListenHost', $ListenHost,
        '-ListenPort', "$Port",
        '-ConnectHost', $ConnectHost,
        '-ConnectPort', "$Port"
    )
    $process = Start-Process -WindowStyle Hidden -FilePath powershell.exe -ArgumentList $args -PassThru
    Set-Content -Path $pidFile -Value $process.Id -Encoding ASCII
}

function Get-PathForPort {
    param([int]$Port)
    switch ($Port) {
        3000 { return '/login' }
        18086 { return '/health' }
        default { return '/' }
    }
}

if (-not $ConnectHost) {
    $ConnectHost = Get-WslPrimaryIp
}

if (-not $ConnectHost) {
    Write-Warning 'Could not determine the WSL IP for localhost relay.'
    exit 0
}

$portList = $Ports -split ',' | ForEach-Object { $_.Trim() } | Where-Object { $_ } | ForEach-Object { [int]$_ }

foreach ($port in $portList) {
    $path = Get-PathForPort -Port $port
    $localV4Url = "http://127.0.0.1:$port$path"
    $localV6Url = "http://[::1]:$port$path"
    $remoteUrl = "http://${ConnectHost}:$port$path"

    $localV4Code = Test-HttpUrl -Url $localV4Url
    $localV6Code = Test-HttpUrl -Url $localV6Url
    if ($localV4Code -ge 200 -and $localV4Code -lt 500 -and $localV6Code -ge 200 -and $localV6Code -lt 500) {
        Write-Host "localhost:$port ok"
        continue
    }

    $remoteCode = Test-HttpUrl -Url $remoteUrl
    if ($remoteCode -lt 200 -or $remoteCode -ge 500) {
        Write-Warning "localhost:$port unavailable and WSL target ${ConnectHost}:$port is not reachable (HTTP $remoteCode)."
        continue
    }

    if ($localV4Code -lt 200 -or $localV4Code -ge 500) {
        Start-Relay -Port $port -Family 'v4' -ListenHost '127.0.0.1' -ConnectHost $ConnectHost

        Start-Sleep -Milliseconds 500
        $relayCode = Test-HttpUrl -Url $localV4Url
        if ($relayCode -ge 200 -and $relayCode -lt 500) {
            Write-Host "localhost:$port IPv4 relayed to ${ConnectHost}:$port"
        } else {
            Write-Warning "Started localhost:$port IPv4 relay, but probe still returned HTTP $relayCode. Fallback: http://${ConnectHost}:$port"
        }
    } else {
        Write-Host "localhost:$port IPv4 ok"
    }

    if ($localV6Code -lt 200 -or $localV6Code -ge 500) {
        Start-Relay -Port $port -Family 'v6' -ListenHost '::1' -ConnectHost $ConnectHost

        Start-Sleep -Milliseconds 500
        $relayCode = Test-HttpUrl -Url $localV6Url
        if ($relayCode -ge 200 -and $relayCode -lt 500) {
            Write-Host "localhost:$port IPv6 relayed to ${ConnectHost}:$port"
        } else {
            Write-Warning "Started localhost:$port IPv6 relay, but probe still returned HTTP $relayCode. Fallback: http://${ConnectHost}:$port"
        }
    } else {
        Write-Host "localhost:$port IPv6 ok"
    }
}
