# One-time Camoufox profile warmup: pass Cloudflare on TwitchTracker, persist cookies for headless scrapes.
param(
    [string]$Login = "jynxzi",
    [string]$ProfileDir = "$env:USERPROFILE\.streamclone\camoufox-profile"
)

$repoRoot = Split-Path $PSScriptRoot -Parent
$scraperRoot = Join-Path (Split-Path $repoRoot -Parent) "streamclone-scraper"
if (-not (Test-Path $scraperRoot)) {
    $scraperRoot = Join-Path $repoRoot "..\streamclone-scraper"
}

$warmScript = @"
import asyncio
import os
import sys

PROFILE = r"$ProfileDir"
URL = "https://twitchtracker.com/$Login/streams"

async def main():
    try:
        from camoufox.async_api import AsyncCamoufox
    except ImportError:
        print("Install camoufox: pip install camoufox && python -m camoufox fetch")
        sys.exit(1)
    os.makedirs(PROFILE, exist_ok=True)
    print(f"Opening {URL} in headful Camoufox...")
    print(f"Profile: {PROFILE}")
    print("Pass any Cloudflare challenge manually, then press Enter in this terminal.")
    async with AsyncCamoufox(
        headless=False,
        persistent_context=True,
        user_data_dir=PROFILE,
        geoip=False,
    ) as context:
        page = context.pages[0] if context.pages else await context.new_page()
        await page.goto(URL, wait_until="domcontentloaded", timeout=120000)
        await asyncio.get_event_loop().run_in_executor(None, input, "Press Enter after TwitchTracker loads... ")
        html = await page.content()
        ok = "streams-table" in html or "table-responsive" in html
        print("streams_table=", ok, "cf=", "just a moment" in html.lower())
    print("Profile warmed. Headless scrapes will reuse cookies from:", PROFILE)

asyncio.run(main())
"@

$tempPy = Join-Path $env:TEMP "warm-camoufox.py"
Set-Content -Path $tempPy -Value $warmScript -Encoding UTF8

Write-Host "Warming Camoufox profile for login: $Login"
python $tempPy
Remove-Item $tempPy -ErrorAction SilentlyContinue
