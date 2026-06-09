from __future__ import annotations

import shutil
import threading
import time
from pathlib import Path

from .db import Store
from .timeutil import now_ms


class CleanupService:
    def __init__(self, store: Store, output_dir: Path, retention_hours: int, interval_seconds: int):
        self.store = store
        self.output_dir = output_dir
        self.retention_hours = retention_hours
        self.interval_seconds = interval_seconds
        self._stop = threading.Event()
        self._thread: threading.Thread | None = None

    def start(self) -> None:
        if self._thread and self._thread.is_alive():
            return
        self._thread = threading.Thread(target=self._loop, name="clipper-cleanup", daemon=True)
        self._thread.start()

    def stop(self) -> None:
        self._stop.set()
        if self._thread:
            self._thread.join(timeout=2)

    def run_once(self) -> None:
        cutoff = now_ms() - self.retention_hours * 60 * 60 * 1000
        for job in self.store.list_purge_candidates(cutoff):
            final_path = Path(job["final_path"] or "")
            if final_path.exists():
                final_path.unlink()
            raw_path = Path(job.get("raw_path") or "")
            if raw_path.exists():
                raw_path.unlink()
            self.store.mark_purged(job["id"])
        temp_root = self.output_dir / "tmp"
        if temp_root.exists():
            for child in temp_root.iterdir():
                if child.is_dir() and child.stat().st_mtime < time.time() - 6 * 60 * 60:
                    shutil.rmtree(child, ignore_errors=True)

    def _loop(self) -> None:
        while not self._stop.wait(self.interval_seconds):
            self.run_once()
