#Requires -Version 5.1
# Stop SSH tunnels to BearHost Grafana (ports 3000/3001).
$ErrorActionPreference = 'SilentlyContinue'
Get-CimInstance Win32_Process -Filter "name='ssh.exe'" |
  Where-Object { $_.CommandLine -match '300[01]:127\.0\.0\.1:3000' } |
  ForEach-Object {
    Write-Host "Stopping ssh pid $($_.ProcessId): $($_.CommandLine)"
    Stop-Process -Id $_.ProcessId -Force
  }
$remaining = Get-CimInstance Win32_Process -Filter "name='ssh.exe'" |
  Where-Object { $_.CommandLine -match '300[01]:127\.0\.0\.1:3000' }
if ($remaining) {
  Write-Host "Remaining tunnels:"
  $remaining | ForEach-Object { Write-Host "  pid $($_.ProcessId)" }
  exit 1
}
Write-Host "No Grafana tunnel processes on :3000 or :3001"
