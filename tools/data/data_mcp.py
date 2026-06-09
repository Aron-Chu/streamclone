#!/usr/bin/env python3
"""Read-only Postgres and Redis MCP tools for Streamclone local debugging."""

from __future__ import annotations

import argparse
import json
import re
import socket
import urllib.parse
from typing import Any

from mcp.server.fastmcp import FastMCP

DEFAULT_PG = "postgres://app:app@127.0.0.1:5432/streamclone?sslmode=disable"
DEFAULT_REDIS = "redis://127.0.0.1:6379/0"
LOGIN_RE = re.compile(r"^[a-z0-9][a-z0-9_]{2,24}$")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Serve read-only Streamclone data diagnostics over MCP.")
    parser.add_argument("--postgres-url", default=DEFAULT_PG, help="Postgres connection URL (localhost).")
    parser.add_argument("--redis-url", default=DEFAULT_REDIS, help="Redis connection URL (localhost).")
    return parser.parse_args()


args = parse_args()
PG_URL = args.postgres_url
REDIS_URL = args.redis_url

mcp = FastMCP(
    "streamclone-data",
    instructions=(
        "Read-only Postgres and Redis inspection for Streamclone emotes, analytics, and chat cache. "
        "Connects to localhost compose services (5432 / 6379)."
    ),
    log_level="ERROR",
)


def pg_connect():
    try:
        import psycopg
    except ImportError as exc:
        raise RuntimeError("psycopg not installed. Run: make codegraph-install") from exc
    return psycopg.connect(PG_URL, connect_timeout=5)


def redis_client():
    try:
        import redis
    except ImportError as exc:
        raise RuntimeError("redis package not installed. Run: make codegraph-install") from exc
    return redis.from_url(REDIS_URL, decode_responses=True, socket_connect_timeout=5)


def service_reachable(host: str, port: int) -> bool:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.settimeout(1.5)
        return sock.connect_ex((host, port)) == 0


@mcp.tool()
def data_status() -> dict[str, Any]:
    """Check whether local Postgres and Redis ports are reachable."""
    pg_host = urllib.parse.urlparse(PG_URL).hostname or "127.0.0.1"
    pg_port = urllib.parse.urlparse(PG_URL).port or 5432
    redis_host = urllib.parse.urlparse(REDIS_URL).hostname or "127.0.0.1"
    redis_port = urllib.parse.urlparse(REDIS_URL).port or 6379
    return {
        "postgres_url": PG_URL.split("@")[-1],
        "redis_url": REDIS_URL.replace(redis_host, redis_host),
        "postgres_reachable": service_reachable(pg_host, pg_port),
        "redis_reachable": service_reachable(redis_host, redis_port),
    }


@mcp.tool()
def postgres_query(query: str, limit: int = 50) -> dict[str, Any]:
    """Run a read-only SQL query (SELECT or WITH only)."""
    sql = query.strip()
    if not sql:
        return {"error": "query must not be empty"}
    head = sql.lstrip().lower()
    if not (head.startswith("select") or head.startswith("with")):
        return {"error": "only SELECT / WITH queries are allowed"}
    if ";" in sql.rstrip(";"):
        return {"error": "multiple statements are not allowed"}
    max_rows = max(1, min(int(limit), 200))
    limited = f"SELECT * FROM ({sql.rstrip(';')}) AS q LIMIT {max_rows}"
    try:
        with pg_connect() as conn:
            with conn.cursor() as cur:
                cur.execute(limited)
                columns = [desc[0] for desc in cur.description or []]
                rows = [dict(zip(columns, row, strict=False)) for row in cur.fetchall()]
        return {"columns": columns, "rows": rows, "row_count": len(rows), "limit": max_rows}
    except Exception as exc:
        return {"error": str(exc), "query": sql}


@mcp.tool()
def emote_jobs(limit: int = 20) -> dict[str, Any]:
    """List recent emote processing_jobs with status counts."""
    max_rows = max(1, min(int(limit), 100))
    try:
        with pg_connect() as conn:
            with conn.cursor() as cur:
                cur.execute(
                    """
                    SELECT status, COUNT(*) AS count
                    FROM processing_jobs
                    GROUP BY status
                    ORDER BY status
                    """
                )
                status_rows = [{"status": r[0], "count": r[1]} for r in cur.fetchall()]
                cur.execute(
                    """
                    SELECT id, emote_id, status, attempts, last_error, updated_at
                    FROM processing_jobs
                    ORDER BY updated_at DESC
                    LIMIT %s
                    """,
                    (max_rows,),
                )
                cols = [d[0] for d in cur.description or []]
                recent = [dict(zip(cols, row, strict=False)) for row in cur.fetchall()]
        return {"status_counts": status_rows, "recent_jobs": recent}
    except Exception as exc:
        return {"error": str(exc)}


@mcp.tool()
def redis_get(key: str) -> dict[str, Any]:
    """Get a Redis key value (string or hash preview)."""
    key = key.strip()
    if not key:
        return {"error": "key must not be empty"}
    try:
        client = redis_client()
        key_type = client.type(key)
        if key_type == "none":
            return {"key": key, "exists": False}
        if key_type == "hash":
            fields = client.hgetall(key)
            preview = dict(list(fields.items())[:20])
            return {"key": key, "type": key_type, "field_count": len(fields), "preview": preview}
        if key_type == "string":
            value = client.get(key)
            return {"key": key, "type": key_type, "value": value[:4000] if value else value}
        return {"key": key, "type": key_type, "note": "unsupported type for full dump; use redis-cli for lists/streams"}
    except Exception as exc:
        return {"error": str(exc), "key": key}


@mcp.tool()
def redis_channel_emotes(login: str, sample: int = 10) -> dict[str, Any]:
    """Inspect channel:emotes:{login} Redis hash (emote dictionary)."""
    channel = login.strip().lower()
    if not LOGIN_RE.fullmatch(channel):
        return {"error": "invalid_channel", "login": login}
    key = f"channel:emotes:{channel}"
    try:
        client = redis_client()
        count = client.hlen(key)
        if count == 0:
            return {"key": key, "exists": False, "field_count": 0}
        names = client.hkeys(key)[: max(1, min(int(sample), 50))]
        preview: dict[str, Any] = {}
        for name in names:
            raw = client.hget(key, name)
            try:
                preview[name] = json.loads(raw) if raw else raw
            except json.JSONDecodeError:
                preview[name] = raw
        return {"key": key, "field_count": count, "sample_fields": preview}
    except Exception as exc:
        return {"error": str(exc), "key": key}


if __name__ == "__main__":
    mcp.run()
