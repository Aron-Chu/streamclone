import json
import os
import sys
import urllib.request

url = os.environ.get(
    "SCRAPE_URL",
    "https://twitchtracker.com/jynxzi/streams/318832886110",
)
scraper = os.environ.get("SCRAPER_URL", "http://127.0.0.1:8000/v2/scrape")
use_proxy = os.environ.get("USE_PROXY", "false").lower() in ("1", "true", "yes")

timeout_ms = int(os.environ.get("SCRAPE_TIMEOUT_MS", "120000"))
client_timeout = max(90, timeout_ms // 1000 + 30)
body = json.dumps({
    "url": url,
    "formats": ["rawHtml"],
    "useProxy": use_proxy,
    "timeout": timeout_ms,
}).encode()
req = urllib.request.Request(
    scraper,
    data=body,
    headers={"Content-Type": "application/json"},
    method="POST",
)
with urllib.request.urlopen(req, timeout=client_timeout) as r:
    d = json.loads(r.read())
    html = (d.get("data") or {}).get("rawHtml") or ""
    has_ecs = 'id="ecs"' in html
    has_injected = 'id="streamclone-viewer-chart"' in html
    has_chart = has_ecs or has_injected or (
        "stream-timestamp-dt" in html and "highcharts" in html.lower() and "<svg" in html
    )
    print("success", d.get("success"), "error", d.get("error"))
    print("len", len(html), "ecs", has_ecs, "injected", has_injected, "chart", has_chart, "cf", "just a moment" in html.lower())
    sys.exit(0 if d.get("success") and has_chart else 1)
