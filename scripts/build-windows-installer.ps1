#Requires -Version 5.1
param(
    [string]$Version = ''
)

$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent $PSScriptRoot
. (Join-Path $PSScriptRoot 'lib\env.ps1')
if ([string]::IsNullOrWhiteSpace($Version)) {
    $Version = Get-EnvReleaseVersionTag
    if (-not $Version) { throw 'Version not set. Pass -Version or add a VERSION file at repo root.' }
}
$Stage = Join-Path $Root "dist\streamclone-$Version"
$Iss = Join-Path $Root 'deploy\installer\streamclone-setup.iss'

if (-not (Test-Path $Stage)) {
    throw "Release stage not found: $Stage. Run scripts/package-release.sh first."
}
if (-not (Test-Path (Join-Path $Stage 'VERSION'))) {
    throw "Invalid release stage (missing VERSION): $Stage"
}

& (Join-Path $Root 'scripts\build-installer-icon.ps1')

$iscc = $env:ISCC
if (-not $iscc) {
    $candidates = @(
        "$env:LOCALAPPDATA\Programs\Inno Setup 6\ISCC.exe",
        "${env:ProgramFiles(x86)}\Inno Setup 6\ISCC.exe",
        "$env:ProgramFiles\Inno Setup 6\ISCC.exe"
    )
    foreach ($path in $candidates) {
        if (Test-Path $path) { $iscc = $path; break }
    }
}
if (-not $iscc -or -not (Test-Path $iscc)) {
    throw 'Inno Setup 6 not found. Install from https://jrsoftware.org/isinfo.php or set ISCC to ISCC.exe path.'
}

$stageForIss = $Stage -replace '\\', '/'
& $iscc "/DVersion=$Version" "/DSourceDir=$stageForIss" $Iss
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

$exe = Join-Path $Root "dist\Streamclone-Setup-$Version.exe"
if (-not (Test-Path $exe)) {
    throw "Expected installer not created: $exe"
}

Write-Host "Created $exe" -ForegroundColor Green
