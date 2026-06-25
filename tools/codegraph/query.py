"""Shared Kuzu query helpers for smoke tests and MCP tools."""

from __future__ import annotations

from pathlib import Path
from typing import Any

import kuzu

NODE_TABLES = ("Function", "Class", "Interface", "ImportModule")


def open_connection(db_path: Path, read_only: bool = True) -> kuzu.Connection:
    if not db_path.exists():
        raise FileNotFoundError(f"Code graph database not found at {db_path}")
    return kuzu.Connection(kuzu.Database(str(db_path), read_only=read_only))


def query_all(conn: kuzu.Connection, query: str, params: dict[str, Any] | None = None) -> list[dict[str, Any]]:
    result = conn.execute(query, params or {})
    columns = result.get_column_names()
    return [dict(zip(columns, row, strict=False)) for row in result.get_all()]


def find_symbols(
    conn: kuzu.Connection,
    symbol_name: str,
    tables: tuple[str, ...] = NODE_TABLES,
) -> list[dict[str, Any]]:
    matches: list[dict[str, Any]] = []
    for table in tables:
        if table == "ImportModule":
            rows = query_all(
                conn,
                """
                MATCH (n:ImportModule)
                WHERE n.id = $symbol OR n.name = $symbol OR n.path = $symbol OR n.local_path = $symbol
                RETURN 'ImportModule' AS kind, n.id AS id, n.name AS name, n.path AS qualified_name,
                       n.local_path AS file_path, 0 AS start_line, 0 AS end_line,
                       0 AS start_byte, 0 AS end_byte
                """,
                {"symbol": symbol_name},
            )
        else:
            rows = query_all(
                conn,
                f"""
                MATCH (n:{table})
                WHERE n.id = $symbol OR n.name = $symbol OR n.qualified_name = $symbol
                RETURN '{table}' AS kind, n.id AS id, n.name AS name, n.qualified_name AS qualified_name,
                       n.file_path AS file_path, n.start_line AS start_line, n.end_line AS end_line,
                       n.start_byte AS start_byte, n.end_byte AS end_byte
                """,
                {"symbol": symbol_name},
            )
        matches.extend(rows)
    return sorted(matches, key=lambda row: (row["kind"], row["file_path"], row["start_line"]))


def search_symbol_substring(
    conn: kuzu.Connection,
    query: str,
    kind: str = "",
    limit: int = 25,
) -> list[dict[str, Any]]:
    needle = query.strip().lower()
    if not needle:
        return []
    max_rows = max(1, min(int(limit), 100))
    tables = NODE_TABLES if kind.strip() == "" else (kind.strip(),)
    matches: list[dict[str, Any]] = []
    for table in tables:
        if table == "ImportModule":
            rows = query_all(
                conn,
                """
                MATCH (n:ImportModule)
                WHERE toLower(n.name) CONTAINS $needle
                   OR toLower(n.path) CONTAINS $needle
                   OR toLower(n.local_path) CONTAINS $needle
                RETURN 'ImportModule' AS kind, n.id AS id, n.name AS name, n.path AS qualified_name,
                       n.local_path AS file_path, 0 AS start_line, 0 AS end_line
                LIMIT $limit
                """,
                {"needle": needle, "limit": max_rows},
            )
        else:
            rows = query_all(
                conn,
                f"""
                MATCH (n:{table})
                WHERE toLower(n.name) CONTAINS $needle
                   OR toLower(n.qualified_name) CONTAINS $needle
                RETURN '{table}' AS kind, n.id AS id, n.name AS name, n.qualified_name AS qualified_name,
                       n.file_path AS file_path, n.start_line AS start_line, n.end_line AS end_line
                LIMIT $limit
                """,
                {"needle": needle, "limit": max_rows},
            )
        matches.extend(rows)
    return sorted(matches, key=lambda row: (row["kind"], row["file_path"], row["start_line"]))[:max_rows]


def outgoing_calls(conn: kuzu.Connection, function_id: str) -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    for table in ("Function", "Class"):
        rows.extend(
            query_all(
                conn,
                f"""
                MATCH (a:Function {{id: $id}})-[r:CALLS]->(b:{table})
                RETURN 'outgoing' AS direction, '{table}' AS target_kind,
                       b.id AS target_id, b.name AS target_name, b.qualified_name AS target_qualified_name,
                       b.file_path AS target_file, b.start_line AS target_line,
                       r.call_expr AS call_expr, r.line AS call_line
                """,
                {"id": function_id},
            )
        )
    return rows


def incoming_calls(conn: kuzu.Connection, symbol: dict[str, Any]) -> list[dict[str, Any]]:
    if symbol["kind"] not in {"Function", "Class"}:
        return []
    return query_all(
        conn,
        f"""
        MATCH (a:Function)-[r:CALLS]->(b:{symbol["kind"]} {{id: $id}})
        RETURN 'incoming' AS direction, 'Function' AS source_kind,
               a.id AS source_id, a.name AS source_name, a.qualified_name AS source_qualified_name,
               a.file_path AS source_file, a.start_line AS source_line,
               r.call_expr AS call_expr, r.line AS call_line
        """,
        {"id": symbol["id"]},
    )
