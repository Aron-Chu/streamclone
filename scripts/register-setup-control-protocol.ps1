#Requires -Version 5.1
# Register streamclone:// URL handler (HKCU) — idempotent wake-on-click for setup-control.
param(
    [string]$Root = ''
)

$ErrorActionPreference = 'Stop'
if ([string]::IsNullOrWhiteSpace($Root)) {
    $Root = Split-Path -Parent $PSScriptRoot
}
$resolvedRoot = (Resolve-Path -LiteralPath $Root).Path
$ensureScript = Join-Path $resolvedRoot 'scripts\ensure-setup-control.ps1'
if (-not (Test-Path $ensureScript)) {
    throw "Missing ensure-setup-control.ps1 under $resolvedRoot"
}

$iconPath = Join-Path $resolvedRoot 'deploy\installer\icon.ico'
$command = "powershell.exe -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File `"$ensureScript`" -Root `"$resolvedRoot`""

$protocolKey = 'HKCU:\Software\Classes\streamclone'
New-Item -Path $protocolKey -Force | Out-Null
Set-ItemProperty -LiteralPath $protocolKey -Name '(Default)' -Value 'URL:Streamclone Protocol'
New-ItemProperty -LiteralPath $protocolKey -Name 'URL Protocol' -Value '' -PropertyType String -Force | Out-Null

$iconKey = Join-Path $protocolKey 'DefaultIcon'
New-Item -Path $iconKey -Force | Out-Null
if (Test-Path $iconPath) {
    Set-ItemProperty -LiteralPath $iconKey -Name '(Default)' -Value "$iconPath,0"
} else {
    Set-ItemProperty -LiteralPath $iconKey -Name '(Default)' -Value "$env:SystemRoot\System32\shell32.dll,13"
}

$commandKey = Join-Path $protocolKey 'shell\open\command'
New-Item -Path $commandKey -Force | Out-Null
Set-ItemProperty -LiteralPath $commandKey -Name '(Default)' -Value $command

. (Join-Path $PSScriptRoot 'lib\env.ps1')
$envFile = Join-Path $resolvedRoot '.env'
if (Test-Path $envFile) {
    Set-EnvFileValue -Path $envFile -Key 'SETUP_CONTROL_WAKE_ENABLED' -Value '1'
    try {
        $profile = (Read-EnvKeyValueFile -Path $envFile)['STREAMCLONE_PROFILE']
        if ([string]::IsNullOrWhiteSpace($profile)) { $profile = 'core' }
        $useImages = (Test-Path (Join-Path $resolvedRoot 'VERSION'))
        $composeArgs = Get-StreamcloneComposeArgs -Root $resolvedRoot -Profile $profile -UseImages:$useImages
        $null = Invoke-EnvDocker -Arguments ($composeArgs + @('up', '-d', '--no-deps', '--force-recreate', 'frontend'))
    } catch {
        Write-Host "  frontend config refresh skipped: $($_.Exception.Message)" -ForegroundColor DarkYellow
    }
}

Write-Host "Registered streamclone:// handler -> $ensureScript"
