# Launch headless Chrome with CDP on the Windows host for the Docker scraper.
# Usage: .\scripts\scraper-cdp.ps1 [-Port 9222] [-Stop]
param(
    [int]$Port = 9222,
    [switch]$Stop,
    [switch]$Headful
)

$ChromePath = "C:\Program Files\Google\Chrome\Application\chrome.exe"
$ProfileDir = Join-Path $env:USERPROFILE ".streamclone\chrome-cdp-profile"

if ($Stop) {
    Get-Process chrome -ErrorAction SilentlyContinue |
        Where-Object { $_.Path -like "*Chrome*" } |
        Stop-Process -Force -ErrorAction SilentlyContinue
    Write-Host "Stopped Chrome processes."
    exit 0
}

if (-not (Test-Path $ChromePath)) {
    Write-Error "Chrome not found at $ChromePath. Set CDP_CHROME_PATH or install Google Chrome."
    exit 1
}

$listening = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue
if ($listening) {
    Write-Host "Port $Port already listening - assuming Chrome CDP is running."
    exit 0
}

New-Item -ItemType Directory -Force -Path $ProfileDir | Out-Null

$args = @(
    "--remote-debugging-port=$Port",
    "--user-data-dir=$ProfileDir",
    "--no-first-run",
    "--disable-gpu"
)
if (-not $Headful) {
    $args = @("--headless=new") + $args
}
$mode = if ($Headful) { "headful" } else { "headless" }
Write-Host "Starting $mode Chrome CDP on port $Port (profile: $ProfileDir)"
$style = if ($Headful) { "Normal" } else { "Hidden" }
Start-Process -FilePath $ChromePath -ArgumentList $args -WindowStyle $style

Start-Sleep -Seconds 2
Write-Host "Set CDP_URL=http://host.docker.internal:$Port in .env and recreate the scraper container."
