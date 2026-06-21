#Requires -Version 5.1
# SSH tunnel to BearHost Grafana (Archive Corpus dashboard).
#
# Usage:
#   powershell -ExecutionPolicy Bypass -File scripts/bearhost-grafana-tunnel.ps1
#
# Default local port 3001 (avoids local Pulse Grafana on :3000).
# Override: $env:BEARHOST_GRAFANA_LOCAL_PORT = '13000'

$ErrorActionPreference = 'Stop'

$hostAddr = if ($env:BEARHOST_HOST) { $env:BEARHOST_HOST } else { '141.11.243.103' }
$user = if ($env:BEARHOST_USER) { $env:BEARHOST_USER } else { 'streamclone' }
$key = if ($env:BEARHOST_SSH_KEY) { $env:BEARHOST_SSH_KEY } else { Join-Path $env:USERPROFILE '.ssh\id_ed25519_bearhost_streamclone' }
$localPort = if ($env:BEARHOST_GRAFANA_LOCAL_PORT) { $env:BEARHOST_GRAFANA_LOCAL_PORT } else { '3001' }
$dashUrl = "http://localhost:${localPort}/d/streamclone-archive/streamclone-archive"

if (-not (Test-Path $key)) {
    Write-Error "SSH key not found: $key"
}

try {
    $h = @{ Authorization = 'Basic ' + [Convert]::ToBase64String([Text.Encoding]::ASCII.GetBytes('admin:streampulse')) }
    $ds = Invoke-RestMethod -Uri 'http://127.0.0.1:3000/api/datasources' -Headers $h -TimeoutSec 2
    if ($ds.type -contains 'influxdb' -or ($ds | Where-Object { $_.type -eq 'influxdb' })) {
        Write-Host ""
        Write-Host "NOTE: local Pulse Grafana is on http://localhost:3000 (InfluxDB — archive panels show no data)."
        Write-Host "      VPS archive dashboard: $dashUrl"
    }
} catch {
    # No local Grafana on :3000 — fine.
}

Write-Host "==> Grafana tunnel: http://localhost:${localPort} (Ctrl+C to stop)"
Write-Host "    Dashboard: $dashUrl"
Write-Host "    Login: admin / streampulse"
ssh -i $key -N -L "${localPort}:127.0.0.1:3000" "${user}@${hostAddr}"
