from __future__ import annotations

import json
import sqlite3
import threading
import uuid
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from .timeutil import now_ms


RUNNING_STATES = {
    "creating_clip",
    "waiting_for_clip",
    "downloading",
    "transcribing",
    "rendering",
}


ACTIVE_STATES = RUNNING_STATES | {"queued"}


@dataclass(frozen=True)
class JobInsertResult:
    job_id: str | None
    suppressed: bool
    existing_job_id: str | None = None
    reason: str = ""


class Store:
    def __init__(self, path: Path):
        self.path = path
        self._lock = threading.RLock()
        self._conn = sqlite3.connect(path, check_same_thread=False)
        self._conn.row_factory = sqlite3.Row
        self._conn.execute("PRAGMA journal_mode=WAL")
        self._conn.execute("PRAGMA foreign_keys=ON")

    def close(self) -> None:
        with self._lock:
            self._conn.close()

    def init(self) -> None:
        with self._lock:
            self._conn.executescript(
                """
                CREATE TABLE IF NOT EXISTS watched_channels (
                    login TEXT PRIMARY KEY,
                    broadcaster_id TEXT,
                    enabled INTEGER NOT NULL DEFAULT 1,
                    created_at INTEGER NOT NULL,
                    updated_at INTEGER NOT NULL,
                    last_connected_at INTEGER,
                    last_error TEXT
                );

                CREATE TABLE IF NOT EXISTS jobs (
                    id TEXT PRIMARY KEY,
                    channel TEXT NOT NULL,
                    broadcaster_id TEXT,
                    trigger_type TEXT NOT NULL,
                    reason TEXT,
                    title TEXT,
                    requested_duration REAL,
                    source_duration REAL NOT NULL,
                    final_duration REAL NOT NULL,
                    event_latency_offset REAL NOT NULL,
                    trigger_detected_at INTEGER NOT NULL,
                    peak_chat_ts INTEGER,
                    message_count INTEGER,
                    twitch_clip_id TEXT,
                    twitch_edit_url TEXT,
                    twitch_clip_url TEXT,
                    twitch_clip_duration REAL,
                    raw_path TEXT,
                    captions_path TEXT,
                    final_path TEXT,
                    captions TEXT,
                    state TEXT NOT NULL,
                    failure_code TEXT,
                    error_message TEXT,
                    warnings TEXT NOT NULL DEFAULT '[]',
                    suppressed_count INTEGER NOT NULL DEFAULT 0,
                    artifact_available INTEGER NOT NULL DEFAULT 0,
                    created_at INTEGER NOT NULL,
                    updated_at INTEGER NOT NULL,
                    started_at INTEGER,
                    finished_at INTEGER
                );

                CREATE INDEX IF NOT EXISTS idx_jobs_state_created ON jobs(state, created_at);
                CREATE INDEX IF NOT EXISTS idx_jobs_broadcaster_created ON jobs(broadcaster_id, created_at);
                CREATE INDEX IF NOT EXISTS idx_jobs_channel_created ON jobs(channel, created_at);

                CREATE TABLE IF NOT EXISTS job_events (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    job_id TEXT NOT NULL,
                    state TEXT NOT NULL,
                    message TEXT,
                    created_at INTEGER NOT NULL,
                    FOREIGN KEY(job_id) REFERENCES jobs(id) ON DELETE CASCADE
                );

                CREATE TABLE IF NOT EXISTS suppressed_triggers (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    channel TEXT NOT NULL,
                    broadcaster_id TEXT,
                    existing_job_id TEXT,
                    reason TEXT NOT NULL,
                    created_at INTEGER NOT NULL
                );
                """
            )
            try:
                self._conn.execute("ALTER TABLE jobs ADD COLUMN captions TEXT")
            except sqlite3.OperationalError:
                pass
            self._conn.commit()

    def upsert_watched_channel(self, login: str, broadcaster_id: str = "") -> dict[str, Any]:
        ts = now_ms()
        login = login.lower()
        with self._lock:
            self._conn.execute(
                """
                INSERT INTO watched_channels (login, broadcaster_id, enabled, created_at, updated_at)
                VALUES (?, ?, 1, ?, ?)
                ON CONFLICT(login) DO UPDATE SET
                    broadcaster_id=COALESCE(NULLIF(excluded.broadcaster_id, ''), watched_channels.broadcaster_id),
                    enabled=1,
                    updated_at=excluded.updated_at
                """,
                (login, broadcaster_id, ts, ts),
            )
            self._conn.commit()
            return self.get_watched_channel(login) or {}

    def disable_watched_channel(self, login: str) -> None:
        with self._lock:
            self._conn.execute(
                "UPDATE watched_channels SET enabled=0, updated_at=? WHERE login=?",
                (now_ms(), login.lower()),
            )
            self._conn.commit()

    def list_watched_channels(self) -> list[dict[str, Any]]:
        with self._lock:
            rows = self._conn.execute(
                "SELECT * FROM watched_channels ORDER BY login"
            ).fetchall()
            return [dict(row) for row in rows]

    def get_watched_channel(self, login: str) -> dict[str, Any] | None:
        with self._lock:
            row = self._conn.execute(
                "SELECT * FROM watched_channels WHERE login=?",
                (login.lower(),),
            ).fetchone()
            return dict(row) if row else None

    def note_channel_error(self, login: str, error: str) -> None:
        with self._lock:
            self._conn.execute(
                "UPDATE watched_channels SET last_error=?, updated_at=? WHERE login=?",
                (error, now_ms(), login.lower()),
            )
            self._conn.commit()

    def insert_job(
        self,
        *,
        channel: str,
        broadcaster_id: str,
        trigger_type: str,
        reason: str,
        title: str,
        requested_duration: float | None,
        source_duration: float,
        final_duration: float,
        event_latency_offset: float,
        trigger_detected_at: int,
        peak_chat_ts: int | None,
        message_count: int | None,
        duplicate_window_seconds: int,
    ) -> JobInsertResult:
        ts = now_ms()
        cutoff = ts - duplicate_window_seconds * 1000
        channel = channel.lower()
        with self._lock:
            existing = self._find_duplicate(channel, broadcaster_id, cutoff)
            if existing:
                existing_id = existing["id"]
                self._conn.execute(
                    "UPDATE jobs SET suppressed_count=suppressed_count+1, updated_at=? WHERE id=?",
                    (ts, existing_id),
                )
                self._conn.execute(
                    """
                    INSERT INTO suppressed_triggers (channel, broadcaster_id, existing_job_id, reason, created_at)
                    VALUES (?, ?, ?, ?, ?)
                    """,
                    (channel, broadcaster_id, existing_id, "duplicate_window", ts),
                )
                self._conn.commit()
                return JobInsertResult(None, True, existing_id, "duplicate_window")

            job_id = uuid.uuid4().hex
            self._conn.execute(
                """
                INSERT INTO jobs (
                    id, channel, broadcaster_id, trigger_type, reason, title, requested_duration,
                    source_duration, final_duration, event_latency_offset, trigger_detected_at,
                    peak_chat_ts, message_count, state, created_at, updated_at
                )
                VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'queued', ?, ?)
                """,
                (
                    job_id,
                    channel,
                    broadcaster_id,
                    trigger_type,
                    reason,
                    title,
                    requested_duration,
                    source_duration,
                    final_duration,
                    event_latency_offset,
                    trigger_detected_at,
                    peak_chat_ts,
                    message_count,
                    ts,
                    ts,
                ),
            )
            self._insert_event_locked(job_id, "queued", reason or trigger_type, ts)
            self._conn.commit()
            return JobInsertResult(job_id, False)

    def _find_duplicate(self, channel: str, broadcaster_id: str, cutoff: int) -> sqlite3.Row | None:
        if broadcaster_id:
            return self._conn.execute(
                """
                SELECT * FROM jobs
                WHERE broadcaster_id=?
                  AND (
                    state IN ('queued', 'creating_clip', 'waiting_for_clip', 'downloading', 'transcribing', 'rendering')
                    OR (state='ready' AND created_at>=?)
                  )
                ORDER BY created_at DESC
                LIMIT 1
                """,
                (broadcaster_id, cutoff),
            ).fetchone()
        return self._conn.execute(
            """
            SELECT * FROM jobs
            WHERE channel=?
              AND (
                state IN ('queued', 'creating_clip', 'waiting_for_clip', 'downloading', 'transcribing', 'rendering')
                OR (state='ready' AND created_at>=?)
              )
            ORDER BY created_at DESC
            LIMIT 1
            """,
            (channel, cutoff),
        ).fetchone()

    def claim_next_job(self, stale_after_ms: int = 120_000) -> dict[str, Any] | None:
        ts = now_ms()
        with self._lock:
            row = self._conn.execute(
                "SELECT * FROM jobs WHERE state='queued' ORDER BY created_at LIMIT 1"
            ).fetchone()
            if not row:
                stale_cutoff = ts - stale_after_ms
                row = self._conn.execute(
                    """
                    SELECT * FROM jobs
                    WHERE state IN ('creating_clip', 'waiting_for_clip', 'downloading', 'transcribing', 'rendering')
                      AND updated_at < ?
                    ORDER BY started_at, created_at
                    LIMIT 1
                    """,
                    (stale_cutoff,),
                ).fetchone()
                if not row:
                    return None
                self._conn.execute(
                    "UPDATE jobs SET updated_at=? WHERE id=?",
                    (ts, row["id"]),
                )
                self._insert_event_locked(row["id"], row["state"], "worker reclaimed stale job", ts)
                self._conn.commit()
                return self.get_job(row["id"])
            self._conn.execute(
                "UPDATE jobs SET state='creating_clip', started_at=COALESCE(started_at, ?), updated_at=? WHERE id=?",
                (ts, ts, row["id"]),
            )
            self._insert_event_locked(row["id"], "creating_clip", "worker claimed job", ts)
            self._conn.commit()
            return self.get_job(row["id"])

    def set_state(self, job_id: str, state: str, message: str = "", **fields: Any) -> None:
        ts = now_ms()
        assignments = ["state=?", "updated_at=?"]
        values: list[Any] = [state, ts]
        if state in {"ready", "failed"}:
            assignments.append("finished_at=?")
            values.append(ts)
        for key, value in fields.items():
            assignments.append(f"{key}=?")
            if isinstance(value, (list, dict)):
                value = json.dumps(value)
            values.append(value)
        values.append(job_id)
        with self._lock:
            self._conn.execute(
                f"UPDATE jobs SET {', '.join(assignments)} WHERE id=?",
                values,
            )
            self._insert_event_locked(job_id, state, message, ts)
            self._conn.commit()

    def update_job(self, job_id: str, **fields: Any) -> None:
        if not fields:
            return
        ts = now_ms()
        assignments = ["updated_at=?"]
        values: list[Any] = [ts]
        for key, value in fields.items():
            assignments.append(f"{key}=?")
            if isinstance(value, (list, dict)):
                value = json.dumps(value)
            values.append(value)
        values.append(job_id)
        with self._lock:
            self._conn.execute(
                f"UPDATE jobs SET {', '.join(assignments)} WHERE id=?",
                values,
            )
            self._conn.commit()

    def add_warning(self, job_id: str, warning: str) -> None:
        with self._lock:
            row = self._conn.execute("SELECT warnings FROM jobs WHERE id=?", (job_id,)).fetchone()
            warnings = json.loads(row["warnings"] or "[]") if row else []
            warnings.append(warning)
            self._conn.execute(
                "UPDATE jobs SET warnings=?, updated_at=? WHERE id=?",
                (json.dumps(warnings), now_ms(), job_id),
            )
            self._conn.commit()

    def retry_job(self, job_id: str) -> dict[str, Any] | None:
        with self._lock:
            row = self._conn.execute("SELECT * FROM jobs WHERE id=?", (job_id,)).fetchone()
            if not row:
                return None
            new_id = uuid.uuid4().hex
            ts = now_ms()
            self._conn.execute(
                """
                INSERT INTO jobs (
                    id, channel, broadcaster_id, trigger_type, reason, title, requested_duration,
                    source_duration, final_duration, event_latency_offset, trigger_detected_at,
                    peak_chat_ts, message_count, state, created_at, updated_at
                )
                VALUES (?, ?, ?, 'retry', ?, ?, ?, ?, ?, ?, ?, ?, ?, 'queued', ?, ?)
                """,
                (
                    new_id,
                    row["channel"],
                    row["broadcaster_id"],
                    "retry of " + job_id,
                    row["title"],
                    row["requested_duration"],
                    row["source_duration"],
                    row["final_duration"],
                    row["event_latency_offset"],
                    now_ms(),
                    row["peak_chat_ts"],
                    row["message_count"],
                    ts,
                    ts,
                ),
            )
            self._insert_event_locked(new_id, "queued", "retry of " + job_id, ts)
            self._conn.commit()
            return self.get_job(new_id)

    def mark_purged(self, job_id: str) -> None:
        self.update_job(job_id, artifact_available=0, state="purged")

    def get_job(self, job_id: str) -> dict[str, Any] | None:
        with self._lock:
            row = self._conn.execute("SELECT * FROM jobs WHERE id=?", (job_id,)).fetchone()
            return self._decode_job(row) if row else None

    def list_jobs(self, limit: int = 100, channel: str | None = None) -> list[dict[str, Any]]:
        with self._lock:
            if channel:
                rows = self._conn.execute(
                    "SELECT * FROM jobs WHERE channel=? ORDER BY created_at DESC LIMIT ?",
                    (channel.lower(), limit),
                ).fetchall()
            else:
                rows = self._conn.execute(
                    "SELECT * FROM jobs ORDER BY created_at DESC LIMIT ?",
                    (limit,),
                ).fetchall()
            return [self._decode_job(row) for row in rows]

    def list_events(self, job_id: str) -> list[dict[str, Any]]:
        with self._lock:
            rows = self._conn.execute(
                "SELECT * FROM job_events WHERE job_id=? ORDER BY created_at",
                (job_id,),
            ).fetchall()
            return [dict(row) for row in rows]

    def list_purge_candidates(self, cutoff_ms: int) -> list[dict[str, Any]]:
        with self._lock:
            rows = self._conn.execute(
                """
                SELECT * FROM jobs
                WHERE artifact_available=1
                  AND final_path IS NOT NULL
                  AND state='ready'
                  AND finished_at IS NOT NULL
                  AND finished_at < ?
                """,
                (cutoff_ms,),
            ).fetchall()
            return [self._decode_job(row) for row in rows]

    def _insert_event_locked(self, job_id: str, state: str, message: str, ts: int) -> None:
        self._conn.execute(
            "INSERT INTO job_events (job_id, state, message, created_at) VALUES (?, ?, ?, ?)",
            (job_id, state, message, ts),
        )

    def _decode_job(self, row: sqlite3.Row) -> dict[str, Any]:
        out = dict(row)
        out["warnings"] = json.loads(out.get("warnings") or "[]")
        return out
