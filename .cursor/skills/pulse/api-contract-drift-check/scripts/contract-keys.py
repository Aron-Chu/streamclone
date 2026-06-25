#!/usr/bin/env python3
"""Heuristic: list pulse-core export surface and common pulse JSON keys in Go BFF."""

from __future__ import annotations

import re
from pathlib import Path

# streamclone repo root: .../.cursor/skills/pulse/api-contract-drift-check/scripts/ → parents[4]
REPO = Path(__file__).resolve().parents[4]
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
    main()
