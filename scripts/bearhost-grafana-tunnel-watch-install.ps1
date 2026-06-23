#Requires -Version 5.1
# Register Windows Task Scheduler job to keep BearHost Grafana tunnel (:3001) healthy.
param(
    [int]$IntervalMinutes = 5
)

$ErrorActionPreference = 'Stop'

if ($IntervalMinutes -lt 1 -or $IntervalMinutes -gt 60) {
    Write-Error "IntervalMinutes must be between 1 and 60"
}

$taskName = 'StreamcloneGrafanaTunnelWatch'
$taskNameLogon = "${taskName}Logon"
$watchPs1 = Join-Path $PSScriptRoot 'bearhost-grafana-tunnel-watch.ps1'
if (-not (Test-Path -LiteralPath $watchPs1)) {
    Write-Error "Missing script: $watchPs1"
}

$taskCmd = "powershell.exe -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File `"$watchPs1`""

function Remove-WatchTasks {
    $prevEap = $ErrorActionPreference
    $ErrorActionPreference = 'SilentlyContinue'
    foreach ($name in @($taskName, $taskNameLogon)) {
        schtasks.exe /Delete /TN $name /F *>$null
        Unregister-ScheduledTask -TaskName $name -Confirm:$false -ErrorAction SilentlyContinue | Out-Null
    }
    $ErrorActionPreference = $prevEap
}

function Install-WithSchtasks {
    $prevEap = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    $createRepeat = schtasks.exe /Create /TN $taskName /TR $taskCmd /SC MINUTE /MO $IntervalMinutes /RL LIMITED /F 2>&1
    $repeatCode = $LASTEXITCODE
    $createLogon = schtasks.exe /Create /TN $taskNameLogon /TR $taskCmd /SC ONLOGON /RL LIMITED /F 2>&1
    $logonCode = $LASTEXITCODE
    $ErrorActionPreference = $prevEap
    if ($repeatCode -ne 0) {
        throw "schtasks repeat failed: $createRepeat"
    }
    if ($logonCode -ne 0) {
        Write-Host "Logon trigger not registered (repeat task is enough): $createLogon" -ForegroundColor Yellow
    }
}

Remove-WatchTasks

$installed = $false
try {
    Install-WithSchtasks
    $installed = $true
    Write-Host "Registered scheduled tasks via schtasks:"
    Write-Host "  $taskName - every $IntervalMinutes minutes"
    Write-Host "  $taskNameLogon - at logon (if permitted)"
}
catch {
    Write-Host "schtasks install failed: $($_.Exception.Message)" -ForegroundColor Yellow
    Write-Host "Trying Register-ScheduledTask..."
    try {
        $action = New-ScheduledTaskAction -Execute 'powershell.exe' -Argument "-NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File `"$watchPs1`""
        $startAt = (Get-Date).AddMinutes(1)
        $repeatTrigger = New-ScheduledTaskTrigger -Once -At $startAt -RepetitionInterval (New-TimeSpan -Minutes $IntervalMinutes) -RepetitionDuration (New-TimeSpan -Days 3650)
        $logonTrigger = New-ScheduledTaskTrigger -AtLogOn
        $settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable -MultipleInstances IgnoreNew -ExecutionTimeLimit (New-TimeSpan -Minutes 3)
        $principal = New-ScheduledTaskPrincipal -UserId $env:USERNAME -LogonType Interactive -RunLevel Limited
        Register-ScheduledTask -TaskName $taskName -Action $action -Trigger @($repeatTrigger, $logonTrigger) -Settings $settings -Principal $principal -Description 'Keeps Streamclone BearHost Grafana SSH tunnel on localhost:3001 healthy.' | Out-Null
        $installed = $true
        Write-Host "Registered scheduled task: $taskName (Register-ScheduledTask)"
    }
    catch {
        Write-Host ""
        Write-Host "Could not register a scheduled task (access denied or policy block)." -ForegroundColor Red
        Write-Host "Manual options:"
        Write-Host "  1. Re-run PowerShell as your user (not elevated) from the repo:"
        Write-Host "       make grafana-watch-install"
        Write-Host "  2. Run a one-off check anytime:"
        Write-Host "       make grafana-watch"
        Write-Host "  3. WSL cron (recommended when Task Scheduler is blocked):"
        Write-Host "       make grafana-watch-install-cron"
        Write-Host "  4. Manual cron line:"
        Write-Host "       */5 * * * * cd <repo> && bash scripts/bearhost-grafana-tunnel-watch.sh"
        exit 1
    }
}

Write-Host "  Action: $watchPs1"
Write-Host "  Log (WSL): ~/.streamclone/grafana-tunnel-watch.log"
Write-Host "  Runs hidden (no console flash); WSL log only when tunnel restarts."
Write-Host ""
Write-Host "Running one health check now..."
$exitCode = Start-Process -FilePath 'powershell.exe' `
    -ArgumentList @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-WindowStyle', 'Hidden', '-File', $watchPs1) `
    -Wait -PassThru -WindowStyle Hidden `
    | Select-Object -ExpandProperty ExitCode
if ($exitCode -ne 0) {
    Write-Host "Initial watch failed (exit $exitCode). Check log and: make grafana-setup" -ForegroundColor Yellow
    if ($installed) {
        Write-Host "Scheduled task is installed; it will retry on the next interval."
    }
    exit $exitCode
}
Write-Host "Tunnel healthy. Dashboard: http://localhost:3001/d/streamclone-archive/streamclone-archive"
