#!/usr/bin/env python3
"""Heuristic: list pulse-core export surface and common pulse JSON keys in Go BFF."""

from __future__ import annotations

import os
import re
import sys
from pathlib import Path


def resolve_streamclone_root(start: Path) -> Path:
    env_root = os.environ.get("STREAMCLONE_ROOT", "").strip()
    if env_root:
        root = Path(env_root).resolve()
        _assert_streamclone_layout(root)
        return root

    for parent in (start, *start.parents):
        if (parent / "go.mod").is_file() and (parent / "internal" / "analytics").is_dir():
            root = parent.resolve()
            _assert_streamclone_layout(root)
            return root

    # streamclone-pulse checkout: sibling streamclone repo (folder may be twitch-7tv-clone).
    for parent in (start, *start.parents):
        if (parent / "package.json").is_file() and (parent / "src" / "background").is_dir():
            for sibling_name in ("twitch-7tv-clone", "streamclone"):
                sibling = (parent.parent / sibling_name).resolve()
                if (sibling / "go.mod").is_file():
                    _assert_streamclone_layout(sibling)
                    return sibling

    raise SystemExit(f"Could not resolve streamclone root from {start}")


def _assert_streamclone_layout(root: Path) -> None:
    pulse_core = root / "packages" / "pulse-core" / "src"
    analytics = root / "internal" / "analytics"
    if not pulse_core.is_dir() or not analytics.is_dir():
        raise SystemExit(
            f"STREAMCLONE_ROOT {root} is missing packages/pulse-core or internal/analytics",
        )


REPO = resolve_streamclone_root(Path(__file__).resolve())
PULSE_CORE = REPO / "packages" / "pulse-core" / "src"
EXT_API = REPO / "internal" / "analytics" / "extension_api.go"
PULSE_COVERAGE = REPO / "internal" / "analytics" / "pulse_coverage.go"


def exports_from_ts(root: Path) -> list[str]:
    names: list[str] = []
    if not root.exists():
        return names
    for path in root.rglob("*.ts"):
        text = path.read_text(encoding="utf-8", errors="replace")
        names.extend(re.findall(r"export (?:type|interface|function|const) (\w+)", text))
    return sorted(set(names))


def json_keys_from_go(path: Path) -> list[str]:
    if not path.exists():
        return []
    text = path.read_text(encoding="utf-8", errors="replace")
    return sorted(set(re.findall(r'json:"(\w+)', text)))


def main() -> None:
    print("repo:", REPO)
    print("pulse-core exports:", ", ".join(exports_from_ts(PULSE_CORE)) or "(missing)")
    print("extension_api json keys:", ", ".join(json_keys_from_go(EXT_API)[:50]) or "(missing)")
    print("pulse_coverage json keys:", ", ".join(json_keys_from_go(PULSE_COVERAGE)[:30]) or "(missing)")


if __name__ == "__main__":
    try:
        main()
    except SystemExit as exc:
        print(exc, file=sys.stderr)
        raise
