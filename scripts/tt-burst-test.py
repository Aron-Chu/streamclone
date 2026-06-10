import json, os, time, urllib.request
scraper = "http://127.0.0.1:8000/v2/scrape"
url = "https://twitchtracker.com/xqc/streams/319638832474"
results = []
for i in range(5):
    body = json.dumps({"url": url, "formats": ["rawHtml"], "useProxy": False, "timeout": 120000, "maxAge": 0}).encode()
    req = urllib.request.Request(scraper, data=body, headers={"Content-Type": "application/json"}, method="POST")
    t0 = time.time()
    try:
        with urllib.request.urlopen(req, timeout=150) as r:
            d = json.loads(r.read())
        val = (d.get("data") or {}).get("validation") or {}
        prot = ((d.get("data") or {}).get("protection") or {})
        results.append({"i": i, "ok": d.get("success") and val.get("ok"), "cf": prot.get("cloudflareState"), "ms": int((time.time()-t0)*1000), "points": val.get("chartPointCount")})
    except Exception as e:
        results.append({"i": i, "ok": False, "error": str(e), "ms": int((time.time()-t0)*1000)})
    print(json.dumps(results[-1]))
print("SUMMARY", json.dumps({"success": sum(1 for r in results if r.get("ok")), "total": len(results)}))
