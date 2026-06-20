import json
import os
import re
import sys
import time
import urllib.error
import urllib.request
from urllib.parse import urlparse

url = os.environ.get(
    "SCRAPE_URL",
    "https://twitchtracker.com/jynxzi/streams/318832886110",
)
probe_id = os.environ.get("SCRAPE_PROBE_ID", "")
scraper = os.environ.get("SCRAPER_URL", "http://127.0.0.1:8000/v2/scrape")
use_proxy = os.environ.get("USE_PROXY", "false").lower() in ("1", "true", "yes")
json_out = os.environ.get("SCRAPE_JSON", "false").lower() in ("1", "true", "yes")
timeout_ms = int(os.environ.get("SCRAPE_TIMEOUT_MS", "120000"))
client_timeout = max(90, timeout_ms // 1000 + 30)

def is_reddit_json_url(target: str) -> bool:
    return "reddit.com" in target and urlparse(target).path.rstrip("/").endswith(".json")


USER_AGENT = os.environ.get("SCRAPE_PROBE_UA", "StreamcloneProbe/1.0")


def build_url_opener(use_proxy_flag: bool) -> urllib.request.OpenerDirector:
    if not use_proxy_flag:
        return urllib.request.build_opener()
    proxy_server = os.environ.get("PROXY_SERVER", "").strip()
    if not proxy_server:
        return urllib.request.build_opener()
    proxy_user = os.environ.get("PROXY_USERNAME", "")
    proxy_pass = os.environ.get("PROXY_PASSWORD", "")
    parsed = urlparse(proxy_server)
    if proxy_user and proxy_pass:
        proxy_url = f"{parsed.scheme}://{proxy_user}:{proxy_pass}@{parsed.netloc}"
    else:
        proxy_url = proxy_server
    return urllib.request.build_opener(
        urllib.request.ProxyHandler({"http": proxy_url, "https": proxy_url})
    )


def reddit_json_fallback_url(url: str) -> str | None:
    if "www.reddit.com" not in url:
        return None
    fallback = url.replace("www.reddit.com", "old.reddit.com")
    if "raw_json=1" not in fallback:
        fallback += "&raw_json=1" if "?" in fallback else "?raw_json=1"
    return fallback


def fetch_reddit_json_direct(url: str, timeout: int, use_proxy_flag: bool) -> tuple[str, bool, str]:
    """Production Reddit JSON uses direct HTTP, not Camoufox."""
    opener = build_url_opener(use_proxy_flag)
    last_error = ""
    for candidate in [url, reddit_json_fallback_url(url)]:
        if not candidate:
            continue
        try:
            req = urllib.request.Request(candidate, headers={"User-Agent": USER_AGENT})
            with opener.open(req, timeout=timeout) as resp:
                text = resp.read().decode("utf-8", errors="replace")
            data = json.loads(text)
            children = data.get("data", {}).get("children") or []
            ok = isinstance(children, list) and len(children) > 0
            return text, ok, "" if ok else "empty or blocked json listing"
        except Exception as exc:  # noqa: BLE001 — probe script
            last_error = str(exc)
    return "", False, last_error or "empty or blocked json listing"


def reddit_json_hot_html_url(json_url: str) -> str:
    parsed = urlparse(json_url)
    path = parsed.path.rstrip("/")
    if path.endswith(".json"):
        path = path[:-5]
    return f"https://old.reddit.com{path}/"


def fetch_reddit_listing_via_scraper(
    json_url: str, use_proxy_flag: bool, scrape_timeout_ms: int
) -> tuple[str, bool, str]:
    """When Reddit blocks JSON from container egress, use scraper HTML listing fallback."""
    scrape_body = {
        "url": reddit_json_hot_html_url(json_url),
        "formats": ["html"],
        "siteProfile": "reddit_search",
        "useProxy": use_proxy_flag,
        "timeout": scrape_timeout_ms,
    }
    body = json.dumps(scrape_body).encode()
    req = urllib.request.Request(
        scraper,
        data=body,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=client_timeout) as resp:
            payload = json.loads(resp.read())
        html = (payload.get("data") or {}).get("html") or (payload.get("data") or {}).get("rawHtml") or ""
        ok = bool(payload.get("success")) and reddit_html_ok(html)
        err = "" if ok else str(payload.get("error") or "listing html unavailable")
        return html, ok, err
    except Exception as exc:  # noqa: BLE001 — probe script
        return "", False, str(exc)


def reddit_site_profile(target: str) -> str | None:
    if "reddit.com" not in target or target.rstrip("/").endswith(".json"):
        return None
    parsed = urlparse(target)
    # Production scrapeRedditListingURL uses social_public for search pages.
    if parsed.netloc == "old.reddit.com" and "/search" not in parsed.path:
        return "reddit_search"
    return "social_public"


def reddit_html_ok(html: str) -> bool:
    lower = html.lower()
    if "blocked by network security" in lower:
        return False
    return any(
        marker in lower
        for marker in (
            "shreddit-post",
            "post-container",
            "thing id-t3",
            'data-testid="post"',
            "search-result",
        )
    )


started = time.perf_counter()
result = {
    "id": probe_id,
    "url": url,
    "useProxy": use_proxy,
    "success": False,
    "durationMs": 0,
    "htmlLen": 0,
    "hasEcs": False,
    "hasInjected": False,
    "hasChart": False,
    "hasStreamList": False,
    "hasRedditJson": False,
    "hasRedditPosts": False,
    "cloudflare": False,
    "error": "",
    "exitCode": 1,
}

try:
    if is_reddit_json_url(url):
        html, ok, err = fetch_reddit_json_direct(
            url, timeout=min(30, client_timeout), use_proxy_flag=use_proxy
        )
        has_reddit_posts = False
        if not ok and use_proxy:
            html, ok, err = fetch_reddit_listing_via_scraper(url, use_proxy, timeout_ms)
            has_reddit_posts = reddit_html_ok(html) if html else False
        result.update({
            "success": ok,
            "htmlLen": len(html),
            "hasRedditJson": ok and html.lstrip().startswith("{"),
            "hasRedditPosts": has_reddit_posts,
            "cloudflare": False,
            "error": err,
            "exitCode": 0 if ok else 1,
        })
    else:
        scrape_body: dict = {
            "url": url,
            "formats": ["rawHtml"] if "reddit.com" not in url else ["html"],
            "useProxy": use_proxy,
            "timeout": timeout_ms,
        }
        site_profile = reddit_site_profile(url)
        if site_profile:
            scrape_body["siteProfile"] = site_profile
        body = json.dumps(scrape_body).encode()
        req = urllib.request.Request(
            scraper,
            data=body,
            headers={"Content-Type": "application/json"},
            method="POST",
        )
        with urllib.request.urlopen(req, timeout=client_timeout) as r:
            d = json.loads(r.read())
            html = (d.get("data") or {}).get("rawHtml") or (d.get("data") or {}).get("html") or ""
            lower = html.lower()
            has_ecs = 'id="ecs"' in html
            has_injected = 'id="streamclone-viewer-chart"' in html
            has_chart = has_ecs or has_injected or (
                "stream-timestamp-dt" in html and "highcharts" in lower and "<svg" in html
            )
            has_stream_list = (
                'id="streams"' in lower
                and re.search(r"/streams/\d{6,}", html) is not None
            )
            has_reddit_json = url.endswith(".json") and html.lstrip().startswith("{")
            has_reddit_posts = "reddit.com" in url and reddit_html_ok(html)
            cloudflare = "just a moment" in lower or "checking your browser" in lower

            ok = bool(d.get("success"))
            if "reddit.com" in url and url.endswith(".json"):
                ok = has_reddit_json and not cloudflare
            elif "reddit.com" in url:
                ok = ok and has_reddit_posts
            elif "/streams/" in url and not url.rstrip("/").endswith("/streams"):
                ok = ok and has_chart
            elif url.rstrip("/").endswith("/streams"):
                ok = ok and has_stream_list
            else:
                ok = ok and has_chart

            result.update({
                "success": ok,
                "htmlLen": len(html),
                "hasEcs": has_ecs,
                "hasInjected": has_injected,
                "hasChart": has_chart,
                "hasStreamList": has_stream_list,
                "hasRedditJson": has_reddit_json,
                "hasRedditPosts": has_reddit_posts,
                "cloudflare": cloudflare,
                "error": str(d.get("error") or ""),
                "exitCode": 0 if ok else 1,
            })
except urllib.error.HTTPError as exc:
    result["error"] = f"http {exc.code}"
except Exception as exc:  # noqa: BLE001 — probe script
    result["error"] = str(exc)
finally:
    result["durationMs"] = int((time.perf_counter() - started) * 1000)

if json_out:
    print(json.dumps(result))
else:
    print("success", result["success"], "error", result["error"])
    print(
        "len", result["htmlLen"],
        "ecs", result["hasEcs"],
        "injected", result["hasInjected"],
        "chart", result["hasChart"],
        "list", result["hasStreamList"],
        "reddit", result.get("hasRedditJson"),
        "redditPosts", result.get("hasRedditPosts"),
        "cf", result["cloudflare"],
        "ms", result["durationMs"],
    )

sys.exit(result["exitCode"])
