"""Experimental incremental code graph indexing via sqlite manifest."""

from __future__ import annotations

import argparse
import hashlib
import json
import sqlite3
import sys
import time
from pathlib import Path

_REPO = Path(__file__).resolve().parents[2]
if str(_REPO) not in sys.path:
    sys.path.insert(0, str(_REPO))

from tools.codegraph.ingest import CodeGraphBuilder, configure_tree_sitter_cache
from tools.codegraph.walker import iter_source_files, to_posix


def manifest_path_for(db_path: Path) -> Path:
    return db_path.parent / "index.sqlite"


def open_manifest(path: Path) -> sqlite3.Connection:
    path.parent.mkdir(parents=True, exist_ok=True)
    conn = sqlite3.connect(path)
    conn.execute(
        """
        CREATE TABLE IF NOT EXISTS file_manifest (
            path TEXT PRIMARY KEY,
            sha256 TEXT NOT NULL,
            indexed_at INTEGER NOT NULL
        )
        """
    )
    return conn


def current_file_hashes(repo: Path) -> dict[str, str]:
    hashes: dict[str, str] = {}
    for path in iter_source_files(repo):
        rel_path = to_posix(path.relative_to(repo))
        try:
            content = path.read_bytes()
            content.decode("utf-8")
        except (UnicodeDecodeError, OSError):
            continue
        hashes[rel_path] = hashlib.sha256(content).hexdigest()
    return hashes


def incremental_build(repo: Path, db_path: Path) -> dict[str, int | str]:
    """Experimental: detect changed files, then fall back to full rebuild when any changed."""
    configure_tree_sitter_cache(repo.resolve())
    manifest_db = manifest_path_for(db_path)
    manifest = open_manifest(manifest_db)
    current = current_file_hashes(repo.resolve())
    stored = {row[0]: row[1] for row in manifest.execute("SELECT path, sha256 FROM file_manifest")}
    changed = [path for path, digest in current.items() if stored.get(path) != digest]
    removed = [path for path in stored if path not in current]

    if not stored or changed or removed:
        builder = CodeGraphBuilder(repo, db_path)
        summary = builder.build()
        now = int(time.time())
        manifest.execute("DELETE FROM file_manifest")
        for path, digest in current.items():
            manifest.execute(
                "INSERT INTO file_manifest(path, sha256, indexed_at) VALUES (?, ?, ?)",
                (path, digest, now),
            )
        manifest.commit()
        summary["mode"] = "full-rebuild-fallback"
        summary["changed_files"] = len(changed)
        summary["removed_files"] = len(removed)
        return summary

    return {
        "repo": str(repo.resolve()),
        "db": str(db_path.resolve()),
        "mode": "noop",
        "changed_files": 0,
        "removed_files": 0,
        "files": len(current),
    }


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Experimental incremental code graph indexing.")
    parser.add_argument("--repo", type=Path, default=Path.cwd())
    parser.add_argument("--db", type=Path, default=Path(".codegraph/streamclone.kuzu"))
    parser.add_argument("--json", action="store_true")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    summary = incremental_build(args.repo, args.db)
    if args.json:
        print(json.dumps(summary, indent=2, sort_keys=True))
    else:
        print(
            "Incremental codegraph mode={mode} changed={changed_files} removed={removed_files} files={files}".format(
                **{k: summary.get(k, "") for k in ("mode", "changed_files", "removed_files", "files")}
            )
        )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
