#!/usr/bin/env python3
"""Local-only chat benchmark planning scaffold (no network by default).

Reads fixture markdown metadata and prints storage/metrics readiness estimates.
Refuses live/network modes unless --allow-live is explicitly passed (future gate).
"""

from __future__ import annotations

import argparse
import re
import sys
from dataclasses import dataclass, field
from pathlib import Path
from typing import Dict, List, Tuple


DEFAULT_BYTES_PER_ROW = 450
DEFAULT_INDEX_OVERHEAD = 1.35


@dataclass
class FixtureMeta:
    name: str
    path: Path
    fixture_class: str = "unknown"
    message_count: int = 0
    avg_message_bytes: int = 120
    notes: str = ""
    extra: Dict[str, str] = field(default_factory=dict)


def parse_fixture_markdown(path: Path) -> FixtureMeta:
    text = path.read_text(encoding="utf-8")
    meta = FixtureMeta(name=path.stem, path=path)

    title = re.search(r"^#\s+(.+)$", text, re.MULTILINE)
    if title:
        meta.name = title.group(1).strip()

    for key, value in re.findall(r"^\|\s*`?([a-zA-Z0-9_]+)`?\s*\|\s*(.+?)\s*\|$", text, re.MULTILINE):
        key = key.strip().lower()
        value = value.strip().strip("`")
        if key in {"field", "value"}:
            continue
        if key == "fixture_class":
            meta.fixture_class = value
        elif key == "message_count":
            meta.message_count = int(re.sub(r"[^0-9]", "", value) or "0")
        elif key == "avg_message_bytes":
            meta.avg_message_bytes = int(re.sub(r"[^0-9]", "", value) or "0")
        elif key == "notes":
            meta.notes = value
        else:
            meta.extra[key] = value

    return meta


def estimate_storage(message_count: int, avg_message_bytes: int) -> Dict[str, int]:
    raw_jsonl = message_count * max(avg_message_bytes, 1)
    gzip_jsonl = max(int(raw_jsonl * 0.22), 1)
    zstd_jsonl = max(int(raw_jsonl * 0.18), 1)
    postgres_rows = int(message_count * DEFAULT_BYTES_PER_ROW * DEFAULT_INDEX_OVERHEAD)
    return {
        "raw_jsonl_bytes": raw_jsonl,
        "gzip_jsonl_bytes": gzip_jsonl,
        "zstd_jsonl_bytes": zstd_jsonl,
        "postgres_row_estimate_bytes": postgres_rows,
    }


REQUIRED_METRICS = [
    ("analytics_chat_watch_admission_duration_seconds", "histogram", "live_irc", "METRICS-CHAT-001"),
    ("analytics_vod_gql_job_duration_seconds", "histogram", "all GQL lanes", "METRICS-CHAT-001"),
    ("analytics_vod_comments_fetched_total", "counter", "all GQL lanes", "METRICS-CHAT-001"),
    ("analytics_vod_terminal_reason_total", "counter", "terminal classifier", "METRICS-CHAT-002"),
    ("analytics_vod_chat_archive_compressed_bytes_total", "counter", "archive export", "METRICS-CHAT-002"),
    ("analytics_vod_gql_pages_fetched_total", "counter", "existing partial", "exists"),
    ("analytics_vod_gql_throttle_total", "counter", "existing partial", "exists"),
    ("analytics_chat_replay_rows_written_total", "counter", "existing partial", "exists"),
]

BLOCKED_RUNTIME_STEPS = [
    "BENCH-002 live IRC load",
    "BENCH-003 hosted missed-moments GQL",
    "BENCH-004 corpus/gold GQL job",
    "BENCH-STORAGE-001 runtime export/compression",
    "LOAD-CHAT-001 cap benchmark",
    "LOAD-GOLD-001 gold throughput benchmark",
    "Any TwitchTracker / Camoufox / live Helix call",
]


def human_bytes(n: int) -> str:
    for unit in ("B", "KB", "MB", "GB"):
        if n < 1024 or unit == "GB":
            return f"{n:.1f} {unit}" if unit != "B" else f"{n} B"
        n /= 1024
    return f"{n} B"


def print_report(meta: FixtureMeta, no_network: bool) -> None:
    print(f"fixture: {meta.name}")
    print(f"path: {meta.path}")
    print(f"class: {meta.fixture_class}")
    print(f"message_count: {meta.message_count}")
    print(f"avg_message_bytes: {meta.avg_message_bytes}")
    if meta.notes:
        print(f"notes: {meta.notes}")

    if meta.message_count <= 0:
        print("\nwarning: message_count missing or zero — storage estimates skipped")
    else:
        est = estimate_storage(meta.message_count, meta.avg_message_bytes)
        print("\nstorage estimates (planning only):")
        for scale in (100_000, 1_000_000, 10_000_000):
            scaled = estimate_storage(scale, meta.avg_message_bytes)
            print(
                f"  @{scale:,} msgs: raw={human_bytes(scaled['raw_jsonl_bytes'])} "
                f"gzip~={human_bytes(scaled['gzip_jsonl_bytes'])} "
                f"pg_rows~={human_bytes(scaled['postgres_row_estimate_bytes'])}"
            )
        print(
            f"  fixture-sized pg estimate: {human_bytes(est['postgres_row_estimate_bytes'])}"
        )

    print("\nrequired metrics (contract):")
    for name, mtype, lane, status in REQUIRED_METRICS:
        print(f"  - {name} ({mtype}) lane={lane} status={status}")

    print("\nblocked runtime steps:")
    for step in BLOCKED_RUNTIME_STEPS:
        print(f"  - {step}")

    print(f"\nnetwork mode: {'blocked (--no-network default)' if no_network else 'ALLOW-LIVE REQUESTED — not implemented'}")
    if not no_network:
        print("error: live mode is not implemented; use docs-only planning", file=sys.stderr)
        sys.exit(2)


def main(argv: List[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Local chat benchmark planning scaffold")
    parser.add_argument("--fixture", type=Path, required=True, help="Fixture markdown path")
    parser.add_argument(
        "--no-network",
        action="store_true",
        default=True,
        help="Default: refuse network/live operations (default: true)",
    )
    parser.add_argument(
        "--allow-live",
        action="store_true",
        help="Future explicit approval flag; live fetch not implemented",
    )
    args = parser.parse_args(argv)

    if not args.fixture.is_file():
        print(f"fixture not found: {args.fixture}", file=sys.stderr)
        return 1

    no_network = not args.allow_live
    meta = parse_fixture_markdown(args.fixture)
    print_report(meta, no_network=no_network)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
