import asyncio
import os

PROFILE = os.environ.get("CAMOUFOX_PERSISTENT_PROFILE", "/data/camoufox-profile")
URL = os.environ.get("SCRAPE_URL", "https://twitchtracker.com/jynxzi/streams/318832886110")


async def main():
    from camoufox.async_api import AsyncCamoufox

    os.makedirs(PROFILE, exist_ok=True)
    async with AsyncCamoufox(
        headless=True,
        persistent_context=True,
        user_data_dir=PROFILE,
        geoip=False,
    ) as ctx:
        page = ctx.pages[0] if ctx.pages else await ctx.new_page()
        resp = await page.goto(URL, wait_until="domcontentloaded", timeout=90000)
        html = await page.content()
        print("status", resp.status if resp else None)
        print("len", len(html))
        markers = [
            'id="ecs"',
            "stream-timestamp-dt",
            "just a moment",
            "Peak viewers",
            "cf_chl_opt",
        ]
        for m in markers:
            print(f"{m}:", m in html or m.lower() in html.lower())


if __name__ == "__main__":
    asyncio.run(main())
