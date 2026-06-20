"""Probe Reddit and X ingest paths used by Pulse Wire / storygraph."""
from __future__ import annotations

import json
import os
import sys
import time
import urllib.request

from urllib.parse import urlparse

USER_AGENT = os.environ.get("SOCIAL_PROBE_UA", "StreamcloneSocialProbe/1.0")


def is_reddit_json_url(target: str) -> bool:
    return "reddit.com" in target and urlparse(target).path.rstrip("/").endswith(".json")



def fetch_direct_json(url: str, probe_id: str, timeout: int = 20) -> dict:
    req = urllib.request.Request(
        url,
        headers={"User-Agent": "Mozilla/5.0 (compatible; StreamcloneSocialProbe/1.0)"},
    )
    started = time.perf_counter()
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read()
        data = json.loads(raw)
        children = data.get("data", {}).get("children") or []
        ok = isinstance(children, list) and len(children) > 0
        return {
            "id": probe_id,
            "mode": "direct_json",
            "url": url,
            "success": ok,
            "durationMs": int((time.perf_counter() - started) * 1000),
            "childCount": len(children),
            "error": "" if ok else "empty listing",
        }
    except Exception as exc:  # noqa: BLE001
        return {
            "id": probe_id,
            "mode": "direct_json",
            "url": url,
            "success": False,
            "durationMs": int((time.perf_counter() - started) * 1000),
            "childCount": 0,
            "error": str(exc),
        }


def scrape_html(url: str, probe_id: str, site_profile: str, use_proxy: bool, timeout_ms: int) -> dict:
    scraper = os.environ.get("SCRAPER_URL", "http://127.0.0.1:8000/v2/scrape")
    body = json.dumps(
        {
            "url": url,
            "formats": ["html"],
            "siteProfile": site_profile,
            "useProxy": use_proxy,
            "timeout": timeout_ms,
        }
    ).encode()
    req = urllib.request.Request(
        scraper,
        data=body,
        headers={
            "Content-Type": "application/json",
            "Authorization": f"Bearer {os.environ.get('SCRAPER_API_KEY', 'local-dev-key')}",
        },
        method="POST",
    )
    started = time.perf_counter()
    try:
        with urllib.request.urlopen(req, timeout=max(90, timeout_ms // 1000 + 30)) as resp:
            payload = json.loads(resp.read())
        html = (payload.get("data") or {}).get("html") or (payload.get("data") or {}).get("rawHtml") or ""
        lower = html.lower()
        has_posts = any(
            marker in lower
            for marker in (
                "shreddit-post",
                "post-container",
                "thing id-t3",
                'data-testid="post"',
                "search-result",
            )
        )
        blocked = "blocked by network security" in lower
        ok = bool(payload.get("success")) and has_posts and not blocked
        return {
            "id": probe_id,
            "mode": "scraper_html",
            "url": url,
            "siteProfile": site_profile,
            "useProxy": use_proxy,
            "success": ok,
            "durationMs": int((time.perf_counter() - started) * 1000),
            "htmlLen": len(html),
            "hasPosts": has_posts,
            "blocked": blocked,
            "error": str(payload.get("error") or ""),
        }
    except Exception as exc:  # noqa: BLE001
        return {
            "id": probe_id,
            "mode": "scraper_html",
            "url": url,
            "siteProfile": site_profile,
            "useProxy": use_proxy,
            "success": False,
            "durationMs": int((time.perf_counter() - started) * 1000),
            "htmlLen": 0,
            "hasPosts": False,
            "blocked": False,
            "error": str(exc),
        }


def probe_x_ingest(base_url: str) -> dict:
    base = base_url.rstrip("/")
    started = time.perf_counter()
    out = {
        "id": "x_ingest_streamerbans",
        "mode": "x_ingest",
        "baseUrl": base,
        "success": False,
        "healthOk": False,
        "timelineOk": False,
        "itemCount": 0,
        "error": "",
        "durationMs": 0,
    }
    try:
        with urllib.request.urlopen(urllib.request.Request(f"{base}/healthz"), timeout=15) as resp:
            body = resp.read().decode().strip()
            out["healthOk"] = resp.status == 200 and (body == "ok" or '"ok":true' in body.replace(" ", ""))
    except Exception as exc:  # noqa: BLE001
        out["error"] = f"healthz: {exc}"
        out["durationMs"] = int((time.perf_counter() - started) * 1000)
        return out
    try:
        with urllib.request.urlopen(urllib.request.Request(f"{base}/users/StreamerBans/timeline"), timeout=30) as resp:
            data = json.loads(resp.read())
        items = data.get("items") or []
        out["timelineOk"] = isinstance(items, list) and len(items) > 0
        out["itemCount"] = len(items)
    except Exception as exc:  # noqa: BLE001
        out["error"] = f"timeline: {exc}"
    out["success"] = out["healthOk"] and out["timelineOk"]
    out["durationMs"] = int((time.perf_counter() - started) * 1000)
    return out


def main() -> int:
    login = os.environ.get("SOCIAL_PROBE_LOGIN", "xqc")
    use_proxy = os.environ.get("USE_PROXY", "false").lower() in ("1", "true", "yes")
    timeout_ms = int(os.environ.get("SCRAPE_TIMEOUT_MS", "90000"))
    x_url = os.environ.get("X_INGEST_URL", "http://127.0.0.1:8098")
    require_x = os.environ.get("REQUIRE_X_INGEST", "false").lower() in ("1", "true", "yes")

    probes = [
        fetch_direct_json(
            "https://old.reddit.com/r/LivestreamFail/hot.json?limit=5&raw_json=1",
            "reddit_old_json_direct",
        ),
        fetch_direct_json(
            "https://www.reddit.com/r/livestreamfail/hot.json?limit=5",
            "reddit_www_json_direct",
        ),
        scrape_html(
            f"https://www.reddit.com/r/LivestreamFail/search?q={login}&restrict_sr=1&sort=new&t=all&limit=8",
            "reddit_search_html",
            "social_public",
            use_proxy,
            timeout_ms,
        ),
        scrape_html(
            "https://old.reddit.com/r/livestreamfail/hot/",
            "reddit_old_html",
            "reddit_search",
            use_proxy,
            timeout_ms,
        ),
        probe_x_ingest(x_url),
    ]

    print(json.dumps({"runAt": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()), "probes": probes}, indent=2))

    reddit_ok = any(p.get("success") for p in probes if p["mode"] != "x_ingest")
    x_probe = next(p for p in probes if p["mode"] == "x_ingest")
    if not reddit_ok:
        return 1
    if require_x and not x_probe.get("success"):
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
