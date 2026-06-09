#!/usr/bin/env python3
"""Read-only MCP tools for Streamclone local stack diagnostics."""

from __future__ import annotations

import argparse
import json
import platform
import re
import socket
import subprocess
import sys
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any

from mcp.server.fastmcp import FastMCP

DEFAULT_BASE = "http://localhost:8090"
DEFAULT_SCRAPER = "http://localhost:8000/v2/scrape"
DEFAULT_CLIP_LOGIN = "sodapoppin"
DEFAULT_TRACKER_URL = "https://twitchtracker.com/jynxzi/streams/318832886110"
COMPOSE_FILES = ["deploy/docker-compose.yml", "deploy/docker-compose.local-tunnel.yml"]
SERVICE_PORTS = {
    "metadata": 8081,
    "video": 8082,
    "chat": 8083,
    "emote": 8084,
    "analytics": 8086,
    "frontend": 5174,
    "clipper": 8095,
    "scraper": 8000,
    "proxy": 8090,
    "mediamtx_hls": 8888,
}
WATCH_PORTS = [8090, 5174, 8081, 8082, 8083, 8084, 8086, 8095, 1935, 8888, 5432, 6379, 9000, 8000]


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Serve Streamclone stack diagnostics over MCP.")
    parser.add_argument("--repo", type=Path, default=Path.cwd(), help="Repository root.")
    parser.add_argument("--base-url", default=DEFAULT_BASE, help="Local Caddy proxy base URL.")
    return parser.parse_args()


args = parse_args()
REPO = args.repo.resolve()
BASE_URL = args.base_url.rstrip("/")

mcp = FastMCP(
    "streamclone-stack",
    instructions=(
        "Read-only diagnostics for the Streamclone local stack at http://localhost:8090. "
        "Use before guessing about wslrelay, HLS 401, auth, or scraper failures."
    ),
    log_level="ERROR",
)


def http_request(
    method: str,
    url: str,
    *,
    body: dict[str, Any] | None = None,
    timeout: float = 15.0,
) -> dict[str, Any]:
    data = None
    headers = {"User-Agent": "streamclone-stack-mcp/1.0", "Accept": "application/json"}
    if body is not None:
        data = json.dumps(body).encode("utf-8")
        headers["Content-Type"] = "application/json"
    req = urllib.request.Request(url, data=data, headers=headers, method=method.upper())
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read()
            status = resp.status
            content_type = resp.headers.get("Content-Type", "")
    except urllib.error.HTTPError as exc:
        raw = exc.read()
        return {
            "ok": False,
            "status": exc.code,
            "url": url,
            "error": exc.reason,
            "body": _decode_body(raw, exc.headers.get("Content-Type", "")),
        }
    except urllib.error.URLError as exc:
        return {"ok": False, "url": url, "error": str(exc.reason)}
    return {
        "ok": True,
        "status": status,
        "url": url,
        "body": _decode_body(raw, content_type),
    }


def _decode_body(raw: bytes, content_type: str) -> Any:
    text = raw.decode("utf-8", "replace")
    if "json" in content_type or (text.startswith("{") or text.startswith("[")):
        try:
            return json.loads(text)
        except json.JSONDecodeError:
            pass
    return text[:8000]


def compose_cmd(*extra: str) -> list[str]:
    cmd = ["docker", "compose", "--env-file", str(REPO / ".env"), "-f", str(REPO / COMPOSE_FILES[0]), "-f", str(REPO / COMPOSE_FILES[1])]
    cmd.extend(extra)
    return cmd


def run_cmd(cmd: list[str], *, timeout: float = 60.0) -> dict[str, Any]:
    try:
        proc = subprocess.run(
            cmd,
            cwd=REPO,
            capture_output=True,
            text=True,
            timeout=timeout,
            check=False,
        )
        return {
            "command": cmd,
            "exit_code": proc.returncode,
            "stdout": proc.stdout[-12000:],
            "stderr": proc.stderr[-4000:],
        }
    except FileNotFoundError:
        return {"command": cmd, "exit_code": -1, "error": "command not found"}
    except subprocess.TimeoutExpired:
        return {"command": cmd, "exit_code": -1, "error": "timeout"}


def port_listeners(port: int) -> list[dict[str, Any]]:
    listeners: list[dict[str, Any]] = []
    if platform.system() == "Windows":
        ps = run_cmd(
            [
                "powershell",
                "-NoProfile",
                "-Command",
                f"Get-NetTCPConnection -LocalPort {port} -State Listen -ErrorAction SilentlyContinue | "
                "ForEach-Object { $p = Get-Process -Id $_.OwningProcess -ErrorAction SilentlyContinue; "
                "[PSCustomObject]@{ port = $_.LocalPort; pid = $_.OwningProcess; process = $(if ($p) { $p.ProcessName } else { 'unknown' }) } } | ConvertTo-Json -Compress",
            ],
            timeout=20.0,
        )
        if ps.get("exit_code") == 0 and ps.get("stdout", "").strip():
            try:
                parsed = json.loads(ps["stdout"])
                if isinstance(parsed, dict):
                    parsed = [parsed]
                for row in parsed:
                    listeners.append({"port": port, "pid": row.get("pid"), "process": row.get("process")})
            except json.JSONDecodeError:
                pass
    else:
        proc = run_cmd(["ss", "-ltnp", f"sport = :{port}"], timeout=10.0)
        if proc.get("exit_code") == 0:
            for line in proc.get("stdout", "").splitlines()[1:]:
                listeners.append({"port": port, "raw": line.strip()})
        if not listeners:
            with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
                sock.settimeout(0.3)
                if sock.connect_ex(("127.0.0.1", port)) == 0:
                    listeners.append({"port": port, "listening": True})
    return listeners


@mcp.tool()
def stack_health(base_url: str = BASE_URL) -> dict[str, Any]:
    """Snapshot auth debug, proxy reachability, and direct service health endpoints."""
    base = base_url.rstrip("/")
    auth = http_request("GET", f"{base}/v1/auth/debug")
    proxy_root = http_request("GET", f"{base}/")
    services: dict[str, Any] = {}
    for name, port in SERVICE_PORTS.items():
        if name in {"proxy", "frontend"}:
            continue
        url = f"http://127.0.0.1:{port}/healthz"
        if name == "scraper":
            url = f"http://127.0.0.1:{port}/"
        elif name == "frontend":
            url = f"http://127.0.0.1:{port}/"
        services[name] = http_request("GET", url, timeout=8.0)
    containers = run_cmd(
        ["docker", "ps", "-a", "--filter", "name=streamclone", "--format", "{{json .}}"],
        timeout=20.0,
    )
    container_rows: list[dict[str, Any]] = []
    if containers.get("exit_code") == 0:
        for line in containers.get("stdout", "").splitlines():
            line = line.strip()
            if line:
                try:
                    container_rows.append(json.loads(line))
                except json.JSONDecodeError:
                    container_rows.append({"raw": line})
    warnings: list[str] = []
    if auth.get("ok") and isinstance(auth.get("body"), dict):
        warnings.extend(auth["body"].get("warnings") or [])
    proxy_port = port_listeners(8090)
    if any(row.get("process") == "wslrelay" for row in proxy_port):
        warnings.append("Port 8090 is owned by wslrelay — localhost may be stale. See .kiro/steering/windows-dev.md")
    return {
        "base_url": base,
        "auth_debug": auth,
        "proxy_root": proxy_root,
        "service_health": services,
        "containers": container_rows,
        "warnings": warnings,
    }


@mcp.tool()
def stack_ports() -> dict[str, Any]:
    """List processes listening on Streamclone-related host ports (detect wslrelay vs Docker)."""
    ports: dict[str, Any] = {}
    for port in WATCH_PORTS:
        rows = port_listeners(port)
        if rows:
            ports[str(port)] = rows
    containers = run_cmd(
        ["docker", "ps", "-a", "--filter", "name=streamclone", "--format", "table {{.Names}}\t{{.Status}}\t{{.Ports}}"],
        timeout=20.0,
    )
    hints: list[str] = []
    relay_ports = [p for p, rows in ports.items() if any(r.get("process") == "wslrelay" for r in rows)]
    if relay_ports:
        hints.append(
            "wslrelay detected on port(s) "
            + ", ".join(relay_ports)
            + ". Run: wsl --shutdown, then recreate the compose stack."
        )
    return {"ports": ports, "docker_ps": containers, "hints": hints}


@mcp.tool()
def playback_probe(channel: str, base_url: str = BASE_URL) -> dict[str, Any]:
    """Probe video diagnostics and HLS manifest through the local proxy."""
    login = channel.strip().lower()
    if not re.fullmatch(r"[a-z0-9][a-z0-9_]{2,24}", login):
        return {"error": "invalid_channel", "channel": channel}
    base = base_url.rstrip("/")
    diagnostics = http_request("GET", f"{base}/v1/stream/diagnostics?channel={login}", timeout=20.0)
    hls_urls = [
        f"{base}/live/{login}/index.m3u8",
        f"{base}/live/{login}/main_stream.m3u8",
    ]
    hls: dict[str, Any] = {}
    for url in hls_urls:
        hls[url] = http_request("GET", url, timeout=15.0)
    hints: list[str] = []
    for url, result in hls.items():
        status = result.get("status")
        if status == 401:
            hints.append(
                "401 on HLS — verify deploy/mediamtx.yml hlsCDNSecret matches Caddy Bearer on /live/* "
                "(see .kiro/steering/playback.md)."
            )
        elif status == 404:
            hints.append(f"404 on {url} — stream may not be started yet.")
    return {"channel": login, "diagnostics": diagnostics, "hls": hls, "hints": list(dict.fromkeys(hints))}


@mcp.tool()
def twitch_auth_status(
    base_url: str = BASE_URL,
    clip_login: str = DEFAULT_CLIP_LOGIN,
) -> dict[str, Any]:
    """Check auth debug, session /v1/me, and a minimal clips probe (like make twitch-debug)."""
    base = base_url.rstrip("/")
    auth = http_request("GET", f"{base}/v1/auth/debug")
    me = http_request("GET", f"{base}/v1/me")
    clips = http_request("GET", f"{base}/v1/channels/{clip_login.strip().lower()}/clips?limit=1", timeout=20.0)
    hints: list[str] = []
    if me.get("status") == 401:
        hints.append("No session cookie — run make twitch-local-auth or import a token via the UI.")
    if auth.get("ok") and isinstance(auth.get("body"), dict) and not auth["body"].get("clientIDConfigured"):
        hints.append("TWITCH_CLIENT_ID not configured in backend.")
    return {"auth_debug": auth, "me": me, "clips_probe": clips, "clip_login": clip_login, "hints": hints}


@mcp.tool()
def scraper_probe(
    scraper_url: str = DEFAULT_SCRAPER,
    test_url: str = DEFAULT_TRACKER_URL,
) -> dict[str, Any]:
    """POST a TwitchTracker scrape (direct vs proxy) and detect meta#ecs / Cloudflare."""
    results: dict[str, Any] = {}
    for use_proxy in (False, True):
        body = {"url": test_url, "formats": ["rawHtml"], "useProxy": use_proxy, "timeout": 60000}
        resp = http_request("POST", scraper_url, body=body, timeout=90.0)
        analysis: dict[str, Any] = {"response": resp}
        body = resp.get("body")
        if resp.get("ok") and isinstance(body, dict):
            if body.get("success") is False:
                analysis["error"] = body.get("error")
            data = body.get("data") or {}
            html = data.get("rawHtml") or data.get("html") or ""
            if isinstance(html, str):
                analysis["html_len"] = len(html)
                analysis["meta_ecs"] = 'id="ecs"' in html
                analysis["cloudflare"] = any(
                    token in html for token in ("just a moment", "cf_chl_opt", "performing security verification")
                )
                analysis["used_proxy"] = data.get("usedProxy")
        results[f"use_proxy_{use_proxy}"] = analysis
    hints: list[str] = []
    direct = results.get("use_proxy_False", {})
    if direct.get("meta_ecs") is False:
        hints.append("Direct scrape lacks meta#ecs — minute-level viewer charts will be flat.")
    if direct.get("cloudflare"):
        hints.append("Cloudflare challenge detected — try proxy or host CDP (scripts/scraper-cdp.ps1).")
    return {"test_url": test_url, "scraper_url": scraper_url, "results": results, "hints": hints}


@mcp.tool()
def compose_logs(service: str, tail: int = 80) -> dict[str, Any]:
    """Fetch bounded docker compose logs for a stack service."""
    allowed = {
        "metadata",
        "video",
        "chat",
        "analytics",
        "emote",
        "frontend",
        "clipper",
        "scraper",
        "local-proxy",
        "redis",
        "postgres",
        "mediamtx",
        "minio",
        "migrate",
    }
    name = service.strip().lower()
    if name not in allowed:
        return {"error": f"unknown service {service!r}", "allowed": sorted(allowed)}
    tail_n = max(10, min(int(tail), 500))
    result = run_cmd(compose_cmd("logs", "--no-color", "--tail", str(tail_n), name), timeout=45.0)
    result["service"] = name
    result["tail"] = tail_n
    return result


if __name__ == "__main__":
    mcp.run()
