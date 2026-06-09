"""Check old VOD loading: insights list, analytics detail, and optional full sync."""
import json
import os
import sys
import urllib.error
import urllib.request

login = os.environ.get("CHANNEL", "jynxzi")
stream_id = os.environ.get("STREAM_ID", "318832886110")
do_sync = os.environ.get("DO_SYNC", "false").lower() in ("1", "true", "yes")
metadata = os.environ.get("METADATA_URL", "http://metadata:8080")
analytics = os.environ.get("ANALYTICS_URL", "http://analytics:8080")


def get(url: str, timeout: int = 90):
    with urllib.request.urlopen(url, timeout=timeout) as r:
        return json.load(r)


def post(url: str, timeout: int = 600):
    req = urllib.request.Request(url, method="POST")
    with urllib.request.urlopen(req, timeout=timeout) as r:
        return r.status, r.read().decode()


def main() -> int:
    insights = get(f"{metadata}/v1/channels/{login}/insights?period=30d")
    history = insights.get("streamHistory") or []
    print(f"insights_vods={len(history)}")
    match = next((s for s in history if str(s.get("id")) == stream_id), None)
    if not match:
        print(f"stream {stream_id} not in insights history")
        return 1
    print(
        "insights_row",
        stream_id,
        match.get("startedAt", "")[:19],
        f"dur_min={match.get('durationMinutes')}",
        f"avg={match.get('avgViewers')}",
        f"peak={match.get('peakViewers')}",
    )

    try:
        detail = get(f"{analytics}/v1/analytics/streams/{stream_id}")
        stream = detail.get("stream") or {}
        rollups = detail.get("rollups") or []
        nz = sum(1 for r in rollups if (r.get("viewerAvg") or 0) > 0)
        cz = sum(1 for r in rollups if (r.get("chatCount") or 0) > 0)
        print(
            "analytics_detail",
            f"vodId={detail.get('vodId')}",
            f"viewerSamples={stream.get('viewerSamples')}",
            f"chatMessages={stream.get('chatMessages')}",
            f"rollups={len(rollups)}",
            f"nonzero_viewer={nz}",
            f"nonzero_chat={cz}",
        )
    except urllib.error.HTTPError as e:
        if e.code == 404:
            print("analytics_detail=not_in_db_yet")
        else:
            print("analytics_detail_error", e.code, e.read().decode())
            return 1

    if do_sync:
        url = f"{analytics}/v1/analytics/streams/{stream_id}/sync?channel={login}"
        print("sync_start", url)
        status, body = post(url)
        print("sync_status", status, body)
        detail = get(f"{analytics}/v1/analytics/streams/{stream_id}")
        stream = detail.get("stream") or {}
        rollups = detail.get("rollups") or []
        cz = sum(1 for r in rollups if (r.get("chatCount") or 0) > 0)
        print(
            "post_sync",
            f"vodId={detail.get('vodId')}",
            f"viewerSamples={stream.get('viewerSamples')}",
            f"chatMessages={stream.get('chatMessages')}",
            f"rollups={len(rollups)}",
            f"nonzero_chat={cz}",
        )
    return 0


if __name__ == "__main__":
    sys.exit(main())
