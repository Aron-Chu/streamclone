# Windows wrapper — runs pulse-emote-smoke.sh via WSL/bash (same as Makefile targets).
param(
    [ValidateSet('core', 'gold', 'gold-fail', 'pick-stream')]
    [string]$Mode = 'core',
    [string]$Login = $env:LOGIN,
    [string]$StreamId = $env:STREAM_ID,
    [string]$TwitchId = $env:TWITCH_ID,
    [switch]$SkipUnit
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'wsl-path.ps1')
$paths = Get-RepoWslPath -StartPath $PSScriptRoot
$wslRepo = $paths.WslRepo

$argsList = @()
switch ($Mode) {
    'gold' { $argsList += '--gold' }
    'gold-fail' { $argsList += '--gold-fail' }
    'pick-stream' { $argsList += '--pick-stream' }
}
if ($SkipUnit) { $argsList += '--skip-unit' }

$envExports = @()
if ($Login) { $envExports += "LOGIN='$Login'" }
if ($StreamId) { $envExports += "STREAM_ID='$StreamId'" }
if ($TwitchId) { $envExports += "TWITCH_ID='$TwitchId'" }
if ($env:SKIP_UNIT_TESTS) { $envExports += "SKIP_UNIT_TESTS='$($env:SKIP_UNIT_TESTS)'" }

$argStr = ($argsList -join ' ')
$exportStr = if ($envExports.Count) { ($envExports -join ' ') + ' ' } else { '' }
$cmd = "cd '$wslRepo' && ${exportStr}bash scripts/pulse-emote-smoke.sh $argStr"

if (Get-Command bash -ErrorAction SilentlyContinue) {
    bash -lc $cmd
    exit $LASTEXITCODE
}

$exit = Invoke-WslBashLcSilent -Command $cmd
exit $exit
