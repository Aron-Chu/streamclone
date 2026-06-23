#Requires -Version 5.1
# Remove Windows Task Scheduler job for Grafana tunnel watchdog.
$ErrorActionPreference = 'Stop'

$taskName = 'StreamcloneGrafanaTunnelWatch'
$taskNameLogon = "${taskName}Logon"
$removed = $false

foreach ($name in @($taskName, $taskNameLogon)) {
    $out = schtasks.exe /Delete /TN $name /F 2>&1
    if ($LASTEXITCODE -eq 0) {
        Write-Host "Removed scheduled task: $name"
        $removed = $true
        continue
    }
    $existing = Get-ScheduledTask -TaskName $name -ErrorAction SilentlyContinue
    if ($existing) {
        Unregister-ScheduledTask -TaskName $name -Confirm:$false
        Write-Host "Removed scheduled task: $name"
        $removed = $true
    }
}

. (Join-Path $PSScriptRoot 'wsl-path.ps1')
$paths = Get-RepoWslPath -StartPath $PSScriptRoot
wsl bash -lc "cd '$($paths.WslRepo)' && bash scripts/bearhost-grafana-tunnel-watch-uninstall-cron.sh" 2>$null

if (-not $removed) {
    Write-Host "No scheduled task named $taskName (or $taskNameLogon)"
}
Write-Host "SSH tunnel is still running until you run: make grafana-stop"
