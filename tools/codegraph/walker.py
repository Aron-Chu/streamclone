"""Repository file walking for code graph indexing."""

from __future__ import annotations

import os
from pathlib import Path
from typing import Iterable

EXTENSION_LANGUAGES = {
    ".go": "go",
    ".ts": "tsx",
    ".tsx": "tsx",
    ".js": "tsx",
    ".jsx": "tsx",
    ".mjs": "tsx",
    ".cjs": "tsx",
    ".py": "python",
    ".sql": "sql",
    ".sh": "bash",
    ".bash": "bash",
}

SKIP_DIRS = {
    ".codegraph",
    ".git",
    ".tmp",
    ".uv-cache",
    ".uv-python",
    ".venv",
    ".gocache",
    ".gomodcache",
    ".mypy_cache",
    ".pytest_cache",
    ".ruff_cache",
    "bin",
    "clipper-data",
    "dist",
    "node_modules",
    "vendor",
}


def to_posix(path: Path) -> str:
    return path.as_posix()


def iter_source_files(repo: Path) -> Iterable[Path]:
    for root, dirs, files in os.walk(repo):
        root_path = Path(root)
        dirs[:] = [d for d in dirs if d not in SKIP_DIRS]
        for name in files:
            path = root_path / name
            if path.suffix.lower() in EXTENSION_LANGUAGES:
                yield path
