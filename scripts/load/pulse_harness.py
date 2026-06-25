#!/usr/bin/env python3
"""LOAD-001 synthetic Pulse load harness — watch + poll with guardrails."""
from __future__ import annotations

import argparse
import json
import os
import re
import statistics
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass, field
from typing import Any

BETA_HEADER = "X-Streamclone-Beta-Key"
PRODUCTION_HOSTS = frozenset({"api.streampulse.stream"})
LOGIN_RE = re.compile(r"^[a-z0-9][a-z0-9_]{2,24}$")


@dataclass
class RequestResult:
    login: str
    kind: str  # watch | poll
    status: int
    latency_ms: float
    error: str = ""
    body: dict[str, Any] | None = None


@dataclass
class HarnessReport:
    mode: str
    target: str
    channels_attempted: int = 0
    watches_ok: int = 0
    watches_rejected: int = 0
    polls_ok: int = 0
    auth_failures: int = 0
    client_errors: int = 0
    server_errors: int = 0
    latencies_ms: list[float] = field(default_factory=list)
    watch_active_max: int = 0
    watch_cap_max: int = 0
    prometheus: dict[str, Any] = field(default_factory=dict)
    notes: list[str] = field(default_factory=list)
    results: list[RequestResult] = field(default_factory=list)

    def record(self, r: RequestResult) -> None:
        self.results.append(r)
        self.latencies_ms.append(r.latency_ms)
        if r.status == 401 or r.status == 403:
            self.auth_failures += 1
        elif 400 <= r.status < 500:
            self.client_errors += 1
        elif r.status >= 500:
            self.server_errors += 1
        if r.kind == "watch":
            if r.status in (200, 202) and r.body and r.body.get("tracking") is True:
                self.watches_ok += 1
            elif r.status in (200, 202, 409):
                self.watches_rejected += 1
            if r.body:
                self.watch_active_max = max(self.watch_active_max, int(r.body.get("active") or 0))
                self.watch_cap_max = max(self.watch_cap_max, int(r.body.get("max") or 0))
        elif r.kind == "poll" and r.status == 200:
            self.polls_ok += 1

    def latency_summary(self) -> dict[str, float | None]:
        if not self.latencies_ms:
            return {"p50": None, "p95": None, "count": 0}
        xs = sorted(self.latencies_ms)
        n = len(xs)

        def pct(p: float) -> float:
            idx = min(n - 1, max(0, int(round(p * (n - 1)))))
            return xs[idx]

        return {"p50": pct(0.50), "p95": pct(0.95), "count": n}

    def to_dict(self) -> dict[str, Any]:
        return {
            "mode": self.mode,
            "target": self.target,
            "channels_attempted": self.channels_attempted,
            "watches_ok": self.watches_ok,
            "watches_rejected": self.watches_rejected,
            "polls_ok": self.polls_ok,
            "auth_failures": self.auth_failures,
            "client_errors_4xx": self.client_errors,
            "server_errors_5xx": self.server_errors,
            "latency_ms": self.latency_summary(),
            "watch_active_max": self.watch_active_max,
            "watch_cap_max": self.watch_cap_max,
            "prometheus": self.prometheus,
            "notes": self.notes,
        }


def parse_host(url: str) -> str:
    parsed = urllib.parse.urlparse(url)
    return (parsed.hostname or "").lower()


def is_production_target(url: str) -> bool:
    host = parse_host(url)
    return host in PRODUCTION_HOSTS


def is_isolated_target(url: str) -> bool:
    host = parse_host(url)
    if host in ("localhost", "127.0.0.1", "::1"):
        return True
    if host.endswith(".local"):
        return True
    if "staging" in host:
        return True
    return False


def load_channels(path: str, limit: int | None) -> list[str]:
    channels: list[str] = []
    with open(path, encoding="utf-8") as f:
        for line in f:
            login = line.strip().lower()
            if not login or login.startswith("#"):
                continue
            if not LOGIN_RE.match(login):
                raise SystemExit(f"invalid channel login in roster: {login!r}")
            channels.append(login)
    if not channels:
        raise SystemExit(f"no channels in roster: {path}")
    if limit is not None:
        return channels[:limit]
    return channels


def load_beta_keys() -> list[str]:
    keys: list[str] = []
    multi = os.environ.get("PULSE_LOAD_BETA_KEYS", "").strip()
    single = os.environ.get("PULSE_LOAD_BETA_KEY", "").strip()
    if multi:
        keys = [k.strip() for k in multi.split(",") if k.strip()]
    elif single:
        keys = [single]
    return keys


def beta_key_present() -> bool:
    return bool(load_beta_keys())


def http_json(
    method: str,
    url: str,
    beta_key: str,
    timeout: float = 30.0,
) -> tuple[int, dict[str, Any] | None, float, str]:
    req = urllib.request.Request(url, method=method)
    req.add_header("Accept", "application/json")
    req.add_header(BETA_HEADER, beta_key)
    if method == "POST":
        req.add_header("Content-Type", "application/json")
        req.data = b"{}"
    start = time.perf_counter()
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read()
            elapsed_ms = (time.perf_counter() - start) * 1000
            try:
                body = json.loads(raw.decode("utf-8")) if raw else None
            except json.JSONDecodeError:
                body = None
            return resp.status, body, elapsed_ms, ""
    except urllib.error.HTTPError as e:
        elapsed_ms = (time.perf_counter() - start) * 1000
        raw = e.read()
        try:
            body = json.loads(raw.decode("utf-8")) if raw else None
        except json.JSONDecodeError:
            body = None
        return e.code, body, elapsed_ms, str(e)
    except Exception as e:  # noqa: BLE001
        elapsed_ms = (time.perf_counter() - start) * 1000
        return 0, None, elapsed_ms, str(e)


def prom_query(prom_url: str, query: str) -> Any:
    enc = urllib.parse.quote(query)
    url = f"{prom_url.rstrip('/')}/api/v1/query?query={enc}"
    try:
        with urllib.request.urlopen(url, timeout=10) as resp:
            data = json.loads(resp.read().decode("utf-8"))
        if data.get("status") != "success":
            return None
        results = data.get("data", {}).get("result", [])
        if not results:
            return None
        return results[0].get("value", [None, None])[1]
    except Exception:
        return None


def snapshot_prometheus(prom_url: str | None) -> dict[str, Any]:
    if not prom_url:
        return {"status": "skipped", "reason": "PULSE_LOAD_PROMETHEUS_URL unset"}
    out: dict[str, Any] = {"status": "ok", "url": prom_url}
    for name, q in {
        "pulse_active_tracked_channels": 'max(pulse_active_tracked_channels{job="analytics"})',
        "pulse_backfill_active_jobs": 'max(pulse_backfill_active_jobs{job="analytics"})',
        "up_analytics": 'max(up{job="analytics"})',
        "http_5xx_rate": 'sum(rate(http_requests_total{job="analytics",status=~"5.."}[5m]))',
    }.items():
        val = prom_query(prom_url, q)
        out[name] = val if val is not None else "missing"
    return out


def validate_target(mode: str, target: str) -> None:
    if not target:
        raise SystemExit("PULSE_LOAD_TARGET is required")
    parsed = urllib.parse.urlparse(target)
    if parsed.scheme not in ("http", "https") or not parsed.netloc:
        raise SystemExit(f"PULSE_LOAD_TARGET must be http(s) URL, got {target!r}")

    if mode == "staging-25":
        if is_production_target(target):
            raise SystemExit(
                "staging-25 refused: production host api.streampulse.stream — "
                "use localhost/staging target only"
            )
        if not is_isolated_target(target):
            raise SystemExit(
                "staging-25 refused: target does not look isolated/staging "
                "(localhost, 127.0.0.1, *.local, *staging*)"
            )
        if os.environ.get("PULSE_LOAD_STAGING_CONFIRM", "").strip() not in ("1", "true", "yes"):
            raise SystemExit(
                "staging-25 requires PULSE_LOAD_STAGING_CONFIRM=1 for isolated targets"
            )


def dry_run(args: argparse.Namespace) -> HarnessReport:
    target = args.target.rstrip("/")
    report = HarnessReport(mode="dry-run", target=target)
    validate_target("smoke", target)  # basic URL validation only

    keys = load_beta_keys()
    if not keys:
        report.notes.append("WARN: no beta key in PULSE_LOAD_BETA_KEY / PULSE_LOAD_BETA_KEYS")
    else:
        report.notes.append(f"beta keys configured: {len(keys)} (values not logged)")

    channels = load_channels(args.channel_file, args.limit)
    report.channels_attempted = len(channels)
    report.notes.append(f"roster: {args.channel_file} ({len(channels)} channels)")

    if is_production_target(target):
        report.notes.append("target class: production (public API)")
    elif is_isolated_target(target):
        report.notes.append("target class: isolated/staging/local")
    else:
        report.notes.append("target class: other")

    if args.mode == "staging-25":
        try:
            validate_target("staging-25", target)
            report.notes.append("staging-25 guardrails: would PASS")
        except SystemExit as e:
            report.notes.append(f"staging-25 guardrails: would FAIL — {e}")

    # Health probe only — no watch mutations.
    health_url = f"{target}/v1/extension/health"
    req = urllib.request.Request(health_url, method="GET")
    req.add_header("Accept", "application/json")
    req.add_header("User-Agent", "Streamclone-LOAD-001-harness/1.0")
    try:
        with urllib.request.urlopen(req, timeout=15) as resp:
            body = json.loads(resp.read().decode("utf-8"))
            report.notes.append(f"health: HTTP {resp.status} version={body.get('version')!r} hostedMode={body.get('hostedMode')!r}")
            if body.get("hostedMode") and not keys:
                report.notes.append("FAIL: hostedMode=true but no beta key for future smoke/staging runs")
    except urllib.error.HTTPError as e:
        report.notes.append(f"health: HTTP {e.code} — {e.reason}")
        if e.code == 403:
            report.notes.append(
                "hint: public health may be blocked off-VPS (Cloudflare); re-run dry-run on BearHost localhost"
            )
    except Exception as e:  # noqa: BLE001
        report.notes.append(f"health: FAIL — {e}")

    report.prometheus = snapshot_prometheus(args.prometheus_url)
    return report


def run_load(args: argparse.Namespace) -> HarnessReport:
    target = args.target.rstrip("/")
    mode = args.mode
    validate_target(mode, target)

    keys = load_beta_keys()
    if not keys:
        raise SystemExit("PULSE_LOAD_BETA_KEY or PULSE_LOAD_BETA_KEYS required for smoke/staging-25")

    limit = args.limit
    if mode == "smoke" and limit is None:
        limit = int(os.environ.get("PULSE_LOAD_CHANNEL_COUNT", "3"))
    if mode == "staging-25":
        limit = 25

    channels = load_channels(args.channel_file, limit)
    report = HarnessReport(mode=mode, target=target, channels_attempted=len(channels))
    stagger = max(0, args.stagger_ms) / 1000.0

    for i, login in enumerate(channels):
        beta_key = keys[i % len(keys)]
        watch_url = f"{target}/v1/analytics/channels/{login}/watch"
        poll_url = f"{target}/v1/extension/pulse/channels/{login}"

        status, body, latency_ms, err = http_json("POST", watch_url, beta_key)
        report.record(RequestResult(login, "watch", status, latency_ms, err, body))

        if stagger and i + 1 < len(channels):
            time.sleep(stagger)

        status, body, latency_ms, err = http_json("GET", poll_url, beta_key)
        report.record(RequestResult(login, "poll", status, latency_ms, err, body))

        if stagger and i + 1 < len(channels):
            time.sleep(stagger)

    report.prometheus = snapshot_prometheus(args.prometheus_url)
    return report


def evaluate_exit(report: HarnessReport, production_cap: int) -> int:
    code = 0
    if report.auth_failures > 0 and report.watches_ok == 0 and report.polls_ok == 0:
        print("FAIL: auth failures on all requests", file=sys.stderr)
        code = 2

    if report.server_errors >= 3:
        print(f"FAIL: sustained 5xx/server errors ({report.server_errors})", file=sys.stderr)
        code = 3

    prom = report.prometheus
    if prom.get("status") == "ok":
        active = prom.get("pulse_active_tracked_channels")
        try:
            active_f = float(active)
            if active_f > production_cap:
                print(
                    f"FAIL: prometheus pulse_active_tracked_channels={active_f} > cap {production_cap}",
                    file=sys.stderr,
                )
                code = 4
        except (TypeError, ValueError):
            report.notes.append("prometheus active channels: missing or non-numeric")

    if report.watch_cap_max and report.watch_active_max > report.watch_cap_max:
        print(
            f"FAIL: watch response active {report.watch_active_max} > max {report.watch_cap_max}",
            file=sys.stderr,
        )
        code = 5

    up = prom.get("up_analytics") if prom.get("status") == "ok" else None
    if up is not None and str(up) not in ("1", "1.0"):
        print(f"FAIL: analytics scrape not up (up={up})", file=sys.stderr)
        code = 6

    return code


def main() -> int:
    parser = argparse.ArgumentParser(description="LOAD-001 Pulse load harness")
    parser.add_argument(
        "--mode",
        choices=("dry-run", "smoke", "staging-25"),
        default=os.environ.get("PULSE_LOAD_MODE", "dry-run"),
    )
    parser.add_argument(
        "--target",
        default=os.environ.get("PULSE_LOAD_TARGET", ""),
        help="Base URL (required except when set via PULSE_LOAD_TARGET)",
    )
    parser.add_argument(
        "--channel-file",
        default=os.environ.get(
            "PULSE_LOAD_CHANNEL_FILE",
            os.path.join(os.path.dirname(__file__), "pulse-load-channels.txt"),
        ),
    )
    parser.add_argument("--limit", type=int, default=None)
    parser.add_argument(
        "--stagger-ms",
        type=int,
        default=int(os.environ.get("PULSE_LOAD_STAGGER_MS", "2000")),
    )
    parser.add_argument(
        "--prometheus-url",
        default=os.environ.get("PULSE_LOAD_PROMETHEUS_URL", ""),
    )
    parser.add_argument(
        "--production-cap",
        type=int,
        default=int(os.environ.get("PULSE_LOAD_PRODUCTION_CAP", "10")),
    )
    parser.add_argument(
        "--evidence-file",
        default=os.environ.get("PULSE_LOAD_EVIDENCE_FILE", ""),
    )
    args = parser.parse_args()
    if not args.prometheus_url:
        args.prometheus_url = None

    if args.mode == "dry-run":
        report = dry_run(args)
        exit_code = 0
        if any(n.startswith("FAIL:") for n in report.notes):
            exit_code = 1
    else:
        report = run_load(args)
        exit_code = evaluate_exit(report, args.production_cap)

    payload = report.to_dict()
    text = json.dumps(payload, indent=2)
    print(text)

    if args.evidence_file:
        with open(args.evidence_file, "w", encoding="utf-8") as f:
            f.write(f"# LOAD-001 harness evidence — mode={args.mode}\n")
            f.write(text)
            f.write("\n")

    return exit_code


if __name__ == "__main__":
    raise SystemExit(main())
