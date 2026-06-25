#!/usr/bin/env python3
"""Thin shim — preserve Makefile/CI entrypoint for code graph ingest."""
from __future__ import annotations

import sys
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]
if str(REPO) not in sys.path:
    sys.path.insert(0, str(REPO))

from tools.codegraph.ingest import main  # noqa: E402

if __name__ == "__main__":
    raise SystemExit(main())
