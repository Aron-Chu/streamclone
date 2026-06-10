import json, os, sys, time, urllib.request

scraper = os.environ.get("SCRAPER_URL", "http://127.0.0.1:8000/v2/scrape")
url = os.environ["SCRAPE_URL"]
use_proxy = os.environ.get("USE_PROXY", "false").lower() in ("1", "true", "yes")
timeout_ms = int(os.environ.get("SCRAPE_TIMEOUT_MS", "120000"))
label = os.environ.get("LABEL", "test")

body = json.dumps({
    "url": url,
    "formats": ["rawHtml"],
    "useProxy": use_proxy,
    "timeout": timeout_ms,
    "maxAge": 0,
}).encode()
req = urllib.request.Request(scraper, data=body, headers={"Content-Type": "application/json"}, method="POST")
start = time.time()
try:
    with urllib.request.urlopen(req, timeout=max(150, timeout_ms//1000 + 45)) as r:
        d = json.loads(r.read())
except Exception as e:
    print(json.dumps({"label": label, "useProxy": use_proxy, "error": str(e), "elapsed_ms": int((time.time()-start)*1000)}))
    sys.exit(1)

data = d.get("data") or {}
val = data.get("validation") or {}
ve = val.get("viewerExtraction") or {}
prot = data.get("protection") or {}
timing = data.get("timing") or {}
html = data.get("rawHtml") or ""
out = {
    "label": label,
    "url": url,
    "useProxy": use_proxy,
    "requestedProxy": use_proxy,
    "usedProxy": data.get("usedProxy"),
    "success": d.get("success"),
    "error": d.get("error"),
    "elapsed_ms": data.get("responseTimeMs") or int((time.time()-start)*1000),
    "cloudflare": prot.get("cloudflareState") or val.get("cloudflareState"),
    "validation_ok": val.get("ok"),
    "ecs": val.get("ecsPresent"),
    "chartPoints": val.get("chartPointCount"),
    "chartSeries": val.get("chartSeries"),
    "viewerExtraction": ve,
    "html_len": len(html),
    "queue_wait_ms": timing.get("queueWaitMs"),
    "profile_wait_ms": timing.get("profileWaitMs"),
    "navigation_ms": timing.get("navigationMs"),
}
print(json.dumps(out))
sys.exit(0 if d.get("success") and val.get("ok") else 1)
