#!/usr/bin/env python3
"""Smoke checks for the Streamclone Kuzu code graph."""

from __future__ import annotations

import sys
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]
DB_PATH = REPO / ".codegraph" / "streamclone.kuzu"

CORE_NODE_TABLES = ("File", "Function", "Class", "Interface", "ImportModule")
CORE_REL_TABLES = ("DEFINES", "IMPORTS", "CALLS")
DOMAIN_NODE_TABLES = ("Route", "Test", "Service")

MIN_FILES = 900
MIN_SYMBOLS = 5000

MERGE_SYMBOL = "mergeMinuteRollups"
MERGE_FILE = "internal/analytics/api.go"
MERGE_LINE = 614

LEGACY_TOOLS = {
    "get_call_chain",
    "get_blast_radius",
    "get_ast_chunk",
    "search_symbols",
    "graph_status",
    "rebuild_graph",
}
NEW_TOOLS = {
    "find_callers",
    "find_callees",
    "find_routes",
    "find_tests_for_symbol",
    "impact_analysis",
    "explain_subsystem",
}


def fail(message: str) -> None:
    print(f"FAIL: {message}", file=sys.stderr)
    raise SystemExit(1)


def ok(message: str) -> None:
    print(f"ok: {message}")


def table_exists(conn, name: str) -> bool:
    result = conn.execute("CALL show_tables() RETURN *")
    while True:
        row = result.get_next()
        if row is None:
            break
        if any(str(cell) == name for cell in row):
            return True
    return False


def query_count(conn, label: str) -> int:
    result = conn.execute(f"MATCH (n:{label}) RETURN count(n) AS count")
    row = result.get_next()
    return int(row[0]) if row else 0


def main() -> int:
    if str(REPO) not in sys.path:
        sys.path.insert(0, str(REPO))

    import kuzu

    from tools.codegraph import codegraph_mcp
    from tools.codegraph.query import find_symbols, open_connection

    print(f"codegraph smoke (repo={REPO})")
    codegraph_mcp.REPO = REPO
    codegraph_mcp.DB_PATH = DB_PATH

    version = getattr(kuzu, "__version__", "unknown")
    ok(f"kuzu import (version {version})")

    if not DB_PATH.exists():
        fail(f"database not found at {DB_PATH}; run make codegraph")
    conn = open_connection(DB_PATH)
    ok(f"database opens at {DB_PATH}")

    for table in CORE_NODE_TABLES:
        if not table_exists(conn, table):
            fail(f"missing node table {table}")
    for table in CORE_REL_TABLES:
        if not table_exists(conn, table):
            fail(f"missing rel table {table}")
    ok(f"core tables present ({', '.join(CORE_NODE_TABLES)})")

    file_count = query_count(conn, "File")
    symbol_count = (
        query_count(conn, "Function") + query_count(conn, "Class") + query_count(conn, "Interface")
    )
    call_result = conn.execute("MATCH ()-[r:CALLS]->() RETURN count(r) AS count")
    call_count = int(call_result.get_next()[0])

    if file_count < MIN_FILES:
        fail(f"files={file_count} < minimum {MIN_FILES}")
    if symbol_count < MIN_SYMBOLS:
        fail(f"symbols={symbol_count} < minimum {MIN_SYMBOLS}")
    ok(
        f"counts files={file_count} symbols={symbol_count} calls={call_count}"
    )

    matches = find_symbols(conn, MERGE_SYMBOL, ("Function",))
    if not matches:
        fail(f"search target {MERGE_SYMBOL!r} not found")
    top = matches[0]
    if top["file_path"] != MERGE_FILE or int(top["start_line"]) != MERGE_LINE:
        fail(f"search expected {MERGE_FILE}:{MERGE_LINE}, got {top['file_path']}:{top['start_line']}")
    ok(f"search_symbols({MERGE_SYMBOL!r}) -> {top['file_path']}:{top['start_line']}")

    chunk = codegraph_mcp.get_ast_chunk(MERGE_SYMBOL)
    if chunk.get("error") or not chunk.get("matches"):
        fail(f"get_ast_chunk symbol mode failed: {chunk.get('error')}")
    symbol_chunk = chunk["matches"][0]
    if symbol_chunk["file_path"] != MERGE_FILE or MERGE_SYMBOL not in symbol_chunk.get("code", ""):
        fail("get_ast_chunk symbol mode unexpected result")
    ok(f"get_ast_chunk({MERGE_SYMBOL!r}) returns source")

    line_chunk = codegraph_mcp.get_ast_chunk(file_path=MERGE_FILE, start_line=MERGE_LINE, end_line=MERGE_LINE + 10)
    if line_chunk.get("error") or not line_chunk.get("matches"):
        fail(f"get_ast_chunk file/line mode failed: {line_chunk.get('error')}")
    if MERGE_SYMBOL not in line_chunk["matches"][0].get("code", ""):
        fail("get_ast_chunk file/line mode missing symbol text")
    ok(f"get_ast_chunk(file_path={MERGE_FILE!r}, start_line={MERGE_LINE}) returns region")

    for table in DOMAIN_NODE_TABLES:
        if not table_exists(conn, table):
            fail(f"missing domain node table {table}")
    route_count = query_count(conn, "Route")
    if route_count < 1:
        fail(f"routes={route_count} < 1")
    ext = conn.execute(
        "MATCH (r:Route) WHERE r.path CONTAINS '/v1/extension' RETURN count(r) AS count"
    )
    if int(ext.get_next()[0]) < 1:
        fail("no /v1/extension routes indexed")
    ok(f"domain tables present; routes={route_count}")

    tool_manager = getattr(codegraph_mcp.mcp, "_tool_manager", None)
    if tool_manager is None:
        fail("MCP tool manager not found")
    tool_names = sorted(tool_manager._tools.keys())  # noqa: SLF001
    if len(tool_names) < 6:
        fail(f"MCP tools={len(tool_names)} < 6")
    ok(f"MCP module loads with {len(tool_names)} tools: {', '.join(tool_names)}")

    for name in LEGACY_TOOLS | NEW_TOOLS:
        if name not in tool_names:
            fail(f"missing MCP tool {name}")

    search = codegraph_mcp.search_symbols(MERGE_SYMBOL, limit=5)
    if not search.get("matches"):
        fail("search_symbols MCP wrapper failed")

    print(f"PASS files={file_count} symbols={symbol_count} calls={call_count} mcp_tools={len(tool_names)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
