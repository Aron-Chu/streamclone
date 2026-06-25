#!/usr/bin/env python3
from __future__ import annotations

import argparse
import hashlib
import json
import subprocess
import sys
import time
from collections import deque
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

_REPO = Path(__file__).resolve().parents[2]
if str(_REPO) not in sys.path:
    sys.path.insert(0, str(_REPO))

import kuzu
from mcp.server.fastmcp import FastMCP

from tools.codegraph.query import (
    NODE_TABLES,
    find_symbols,
    incoming_calls,
    open_connection,
    outgoing_calls,
    query_all,
    search_symbol_substring,
)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Serve the local Kuzu code graph over MCP.")
    parser.add_argument("--repo", type=Path, default=Path.cwd(), help="Repository root.")
    parser.add_argument("--db", type=Path, default=Path(".codegraph/streamclone.kuzu"), help="Kuzu database path.")
    return parser.parse_args()


args = parse_args()
REPO = args.repo.resolve()
DB_PATH = args.db.resolve()

mcp = FastMCP(
    "streamclone-codegraph",
    instructions="Deterministic tree-sitter/Kuzu graph tools for the local streamclone repository.",
    log_level="ERROR",
)


def connection() -> kuzu.Connection:
    return open_connection(DB_PATH)


def db_query_all(query: str, params: dict[str, Any] | None = None) -> list[dict[str, Any]]:
    return query_all(connection(), query, params)


def db_find_symbols(symbol_name: str, tables: tuple[str, ...] = NODE_TABLES) -> list[dict[str, Any]]:
    return find_symbols(connection(), symbol_name, tables)


def read_line_region(file_path: str, start_line: int, end_line: int) -> dict[str, Any]:
    path = REPO / file_path
    if not path.exists():
        return {"error": f"file not found: {file_path}"}
    lines = path.read_text(encoding="utf-8", errors="replace").splitlines()
    if start_line < 1 or start_line > len(lines):
        return {"error": f"start_line {start_line} out of range for {file_path}"}
    if end_line <= 0:
        end_line = min(len(lines), start_line + 50)
    end_line = min(end_line, len(lines))
    code = "\n".join(lines[start_line - 1 : end_line])
    indexed_file = db_query_all(
        "MATCH (f:File {path: $path}) RETURN f.sha256 AS sha256",
        {"path": file_path},
    )
    current_sha = hashlib.sha256(path.read_bytes()).hexdigest()
    indexed_sha = indexed_file[0]["sha256"] if indexed_file else ""
    return {
        "file_path": file_path,
        "absolute_path": str(path.resolve()),
        "start_line": start_line,
        "end_line": end_line,
        "stale_index": bool(indexed_sha and indexed_sha != current_sha),
        "code": code,
    }


@mcp.tool()
def get_call_chain(function_name: str, depth: int = 3) -> dict[str, Any]:
    """Trace direct and indirect callers and callees for a function up to 3 hops deep."""
    max_depth = max(1, min(int(depth), 3))
    conn = connection()
    seeds = find_symbols(conn, function_name, ("Function",))
    if not seeds:
        return {"query": function_name, "error": "No function matched this name, qualified name, or id.", "seeds": []}

    edges: list[dict[str, Any]] = []
    callers: dict[str, dict[str, Any]] = {}
    callees: dict[str, dict[str, Any]] = {}

    for seed in seeds:
        queue = deque([(seed["id"], 0, "callee"), (seed["id"], 0, "caller")])
        seen = {(seed["id"], "callee"), (seed["id"], "caller")}
        while queue:
            current_id, current_depth, mode = queue.popleft()
            if current_depth >= max_depth:
                continue

            if mode == "callee":
                for edge in outgoing_calls(conn, current_id):
                    edge["depth"] = current_depth + 1
                    edge["from_id"] = current_id
                    edges.append(edge)
                    target_id = edge["target_id"]
                    callees[target_id] = {
                        "kind": edge["target_kind"],
                        "id": target_id,
                        "name": edge["target_name"],
                        "qualified_name": edge["target_qualified_name"],
                        "file_path": edge["target_file"],
                        "line": edge["target_line"],
                        "depth": min(edge["depth"], callees.get(target_id, {}).get("depth", edge["depth"])),
                    }
                    if edge["target_kind"] == "Function" and (target_id, "callee") not in seen:
                        seen.add((target_id, "callee"))
                        queue.append((target_id, current_depth + 1, "callee"))
            else:
                current_symbol = {"kind": "Function", "id": current_id}
                for edge in incoming_calls(conn, current_symbol):
                    edge["depth"] = current_depth + 1
                    edge["to_id"] = current_id
                    edges.append(edge)
                    source_id = edge["source_id"]
                    callers[source_id] = {
                        "kind": edge["source_kind"],
                        "id": source_id,
                        "name": edge["source_name"],
                        "qualified_name": edge["source_qualified_name"],
                        "file_path": edge["source_file"],
                        "line": edge["source_line"],
                        "depth": min(edge["depth"], callers.get(source_id, {}).get("depth", edge["depth"])),
                    }
                    if (source_id, "caller") not in seen:
                        seen.add((source_id, "caller"))
                        queue.append((source_id, current_depth + 1, "caller"))

    return {
        "query": function_name,
        "max_depth": max_depth,
        "seeds": seeds,
        "callers": sorted(callers.values(), key=lambda row: (row["depth"], row["file_path"], row["line"])),
        "callees": sorted(callees.values(), key=lambda row: (row["depth"], row["file_path"], row["line"])),
        "edges": edges,
    }


def inheritance_edges(seed: dict[str, Any]) -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    for table in ("Class", "Interface"):
        rows.extend(
            db_query_all(
                f"""
                MATCH (a:{seed["kind"]} {{id: $id}})-[r:INHERITS_FROM]->(b:{table})
                RETURN 'inherits from seed' AS direction, '{table}' AS other_kind,
                       b.id AS other_id, b.name AS other_name, b.qualified_name AS other_qualified_name,
                       b.file_path AS other_file, b.start_line AS other_line,
                       r.line AS line
                """,
                {"id": seed["id"]},
            )
        )
        rows.extend(
            db_query_all(
                f"""
                MATCH (a:{table})-[r:INHERITS_FROM]->(b:{seed["kind"]} {{id: $id}})
                RETURN 'inheritor of seed' AS direction, '{table}' AS other_kind,
                       a.id AS other_id, a.name AS other_name, a.qualified_name AS other_qualified_name,
                       a.file_path AS other_file, a.start_line AS other_line,
                       r.line AS line
                """,
                {"id": seed["id"]},
            )
        )
    return rows


def import_targets_symbol_file(import_edge: dict[str, Any], symbol_file: str) -> bool:
    local_path = import_edge.get("local_path") or ""
    if not local_path:
        return False
    if Path(local_path).suffix:
        return local_path == symbol_file
    return symbol_file == local_path or symbol_file.startswith(local_path.rstrip("/") + "/")


@mcp.tool()
def get_blast_radius(symbol_name: str) -> dict[str, Any]:
    """Find files, functions, imports, and direct graph edges that rely on a symbol."""
    seeds = db_find_symbols(symbol_name)
    if not seeds:
        return {"query": symbol_name, "error": "No symbol matched this name, qualified name, module path, or id.", "seeds": []}

    conn = connection()
    files: dict[str, dict[str, Any]] = {}
    functions: dict[str, dict[str, Any]] = {}
    classes: dict[str, dict[str, Any]] = {}
    imports: dict[str, dict[str, Any]] = {}
    edges: list[dict[str, Any]] = []

    all_import_edges = query_all(
        conn,
        """
        MATCH (f:File)-[r:IMPORTS]->(m:ImportModule)
        RETURN f.path AS file_path, m.id AS module_id, m.name AS module_name,
               m.path AS module_path, m.local_path AS local_path,
               r.alias AS alias, r.line AS line
        """,
    )

    for seed in seeds:
        if seed["kind"] in {"Function", "Class", "Interface"}:
            files[seed["file_path"]] = {"path": seed["file_path"], "reason": "defines seed"}

            for row in query_all(
                conn,
                f"""
                MATCH (f:File)-[:DEFINES]->(n:{seed["kind"]} {{id: $id}})
                RETURN f.path AS file_path
                """,
                {"id": seed["id"]},
            ):
                files[row["file_path"]] = {"path": row["file_path"], "reason": "defines seed"}

            if seed["kind"] in {"Function", "Class"}:
                for edge in incoming_calls(conn, seed):
                    functions[edge["source_id"]] = {
                        "id": edge["source_id"],
                        "name": edge["source_name"],
                        "qualified_name": edge["source_qualified_name"],
                        "file_path": edge["source_file"],
                        "line": edge["source_line"],
                        "reason": "calls seed",
                    }
                    files[edge["source_file"]] = {"path": edge["source_file"], "reason": "calls seed"}
                    edges.append(edge)

            if seed["kind"] == "Function":
                for edge in outgoing_calls(conn, seed["id"]):
                    collection = classes if edge["target_kind"] == "Class" else functions
                    collection[edge["target_id"]] = {
                        "id": edge["target_id"],
                        "name": edge["target_name"],
                        "qualified_name": edge["target_qualified_name"],
                        "file_path": edge["target_file"],
                        "line": edge["target_line"],
                        "reason": "called by seed",
                    }
                    files[edge["target_file"]] = {"path": edge["target_file"], "reason": "called by seed"}
                    edges.append(edge)

            if seed["kind"] in {"Class", "Interface"}:
                for edge in inheritance_edges(seed):
                    target_map = classes if edge["other_kind"] == "Class" else functions
                    if edge["other_kind"] == "Interface":
                        imports_key = f"interface:{edge['other_id']}"
                        imports[imports_key] = {
                            "id": edge["other_id"],
                            "name": edge["other_name"],
                            "file_path": edge["other_file"],
                            "reason": edge["direction"],
                        }
                    else:
                        target_map[edge["other_id"]] = {
                            "id": edge["other_id"],
                            "name": edge["other_name"],
                            "qualified_name": edge["other_qualified_name"],
                            "file_path": edge["other_file"],
                            "line": edge["other_line"],
                            "reason": edge["direction"],
                        }
                    files[edge["other_file"]] = {"path": edge["other_file"], "reason": edge["direction"]}
                    edges.append(edge)

            for edge in all_import_edges:
                if import_targets_symbol_file(edge, seed["file_path"]):
                    imports[edge["module_id"]] = {**edge, "reason": "imports seed module"}
                    files[edge["file_path"]] = {"path": edge["file_path"], "reason": "imports seed module"}

        elif seed["kind"] == "ImportModule":
            for edge in all_import_edges:
                if edge["module_id"] == seed["id"]:
                    imports[edge["module_id"]] = {**edge, "reason": "imports module"}
                    files[edge["file_path"]] = {"path": edge["file_path"], "reason": "imports module"}

    return {
        "query": symbol_name,
        "seeds": seeds,
        "files": sorted(files.values(), key=lambda row: row["path"]),
        "functions": sorted(functions.values(), key=lambda row: (row["file_path"], row["line"])),
        "classes": sorted(classes.values(), key=lambda row: (row["file_path"], row["line"])),
        "imports": sorted(imports.values(), key=lambda row: (row.get("file_path", ""), row.get("line", 0))),
        "edges": edges,
    }


@mcp.tool()
def get_ast_chunk(
    function_name: str = "",
    file_path: str = "",
    start_line: int = 0,
    end_line: int = 0,
) -> dict[str, Any]:
    """Return exact source text for a symbol or a file line range."""
    if file_path and start_line > 0:
        region = read_line_region(file_path, int(start_line), int(end_line))
        if region.get("error"):
            return {"query": file_path, "error": region["error"], "matches": []}
        return {
            "query": file_path,
            "mode": "file_lines",
            "matches": [region],
        }

    if not function_name:
        return {"query": "", "error": "Provide function_name or file_path + start_line.", "matches": []}

    matches = find_symbols(connection(), function_name, ("Function",))
    if not matches:
        return {"query": function_name, "error": "No function matched this name, qualified name, or id.", "matches": []}

    chunks: list[dict[str, Any]] = []
    for match in matches:
        path = REPO / match["file_path"]
        content = path.read_bytes()
        indexed_file = db_query_all(
            "MATCH (f:File {path: $path}) RETURN f.sha256 AS sha256, f.abspath AS abspath",
            {"path": match["file_path"]},
        )
        current_sha = hashlib.sha256(content).hexdigest()
        indexed_sha = indexed_file[0]["sha256"] if indexed_file else ""
        start_byte = int(match["start_byte"])
        end_byte = int(match["end_byte"])
        code = content[start_byte:end_byte].decode("utf-8", "replace")
        chunks.append(
            {
                "id": match["id"],
                "name": match["name"],
                "qualified_name": match["qualified_name"],
                "file_path": match["file_path"],
                "absolute_path": str(path.resolve()),
                "start_line": match["start_line"],
                "end_line": match["end_line"],
                "stale_index": bool(indexed_sha and indexed_sha != current_sha),
                "code": code,
            }
        )
    return {"query": function_name, "mode": "symbol", "matches": chunks}


@mcp.tool()
def search_symbols(query: str, kind: str = "", limit: int = 25) -> dict[str, Any]:
    """Search function/class/interface names by substring (case-insensitive)."""
    if not query.strip():
        return {"query": query, "error": "query must not be empty", "matches": []}
    if kind.strip() and kind.strip() not in NODE_TABLES:
        return {
            "query": query,
            "error": f"invalid kind {kind!r}",
            "allowed_kinds": list(NODE_TABLES),
            "matches": [],
        }
    max_rows = max(1, min(int(limit), 100))
    matches = search_symbol_substring(connection(), query, kind, max_rows)
    return {"query": query, "kind_filter": kind or None, "limit": max_rows, "matches": matches}


@mcp.tool()
def graph_status() -> dict[str, Any]:
    """Return code graph database age, indexed file counts, and symbol totals."""
    if not DB_PATH.exists():
        return {
            "db_path": str(DB_PATH),
            "exists": False,
            "error": "Run make codegraph to build the graph.",
        }
    db_stat = DB_PATH.stat()
    indexed_at = datetime.fromtimestamp(db_stat.st_mtime, tz=timezone.utc).isoformat()
    counts: dict[str, int] = {}
    for label in ("File", "Function", "Class", "Interface", "ImportModule", "Route", "Test", "Service"):
        rows = db_query_all(f"MATCH (n:{label}) RETURN count(n) AS count")
        counts[label.lower()] = int(rows[0]["count"]) if rows else 0
    extensions: dict[str, int] = {}
    for row in db_query_all("MATCH (f:File) RETURN f.path AS path"):
        suffix = Path(row["path"]).suffix.lower() or "(none)"
        extensions[suffix] = extensions.get(suffix, 0) + 1
    return {
        "db_path": str(DB_PATH),
        "exists": True,
        "indexed_at_utc": indexed_at,
        "counts": counts,
        "extensions": dict(sorted(extensions.items())),
    }


@mcp.tool()
def rebuild_graph() -> dict[str, Any]:
    """Rebuild the Kuzu code graph from the repository (runs codegraph_ingest.py)."""
    ingest = REPO / "tools" / "codegraph" / "codegraph_ingest.py"
    if not ingest.exists():
        return {"error": f"ingest script not found at {ingest}"}
    started = time.time()
    proc = subprocess.run(
        [sys.executable, str(ingest), "--repo", str(REPO), "--db", str(DB_PATH), "--json"],
        cwd=REPO,
        capture_output=True,
        text=True,
        check=False,
    )
    elapsed = round(time.time() - started, 2)
    summary: dict[str, Any] = {}
    if proc.stdout.strip():
        try:
            summary = json.loads(proc.stdout)
        except json.JSONDecodeError:
            summary = {"stdout": proc.stdout[-4000:]}
    return {
        "exit_code": proc.returncode,
        "elapsed_seconds": elapsed,
        "summary": summary,
        "stderr": proc.stderr[-2000:] if proc.stderr else "",
        "status": graph_status() if proc.returncode == 0 else None,
    }


@mcp.tool()
def find_callers(symbol_or_query: str, limit: int = 50) -> dict[str, Any]:
    """Find functions that call the given symbol."""
    max_rows = max(1, min(int(limit), 200))
    conn = connection()
    seeds = find_symbols(conn, symbol_or_query, ("Function", "Class"))
    if not seeds:
        seeds = search_symbol_substring(conn, symbol_or_query, "Function", 5)
    callers: list[dict[str, Any]] = []
    seen: set[str] = set()
    for seed in seeds:
        for edge in incoming_calls(conn, seed):
            if edge["source_id"] in seen:
                continue
            seen.add(edge["source_id"])
            callers.append(edge)
            if len(callers) >= max_rows:
                break
    return {"query": symbol_or_query, "seeds": seeds, "callers": callers[:max_rows]}


@mcp.tool()
def find_callees(symbol_or_query: str, limit: int = 50) -> dict[str, Any]:
    """Find functions/classes called by the given symbol."""
    max_rows = max(1, min(int(limit), 200))
    conn = connection()
    seeds = find_symbols(conn, symbol_or_query, ("Function",))
    if not seeds:
        seeds = search_symbol_substring(conn, symbol_or_query, "Function", 5)
    callees: list[dict[str, Any]] = []
    seen: set[str] = set()
    for seed in seeds:
        for edge in outgoing_calls(conn, seed["id"]):
            key = edge["target_id"]
            if key in seen:
                continue
            seen.add(key)
            callees.append(edge)
            if len(callees) >= max_rows:
                break
    return {"query": symbol_or_query, "seeds": seeds, "callees": callees[:max_rows]}


@mcp.tool()
def find_routes(query: str = "", method: str = "", path: str = "") -> dict[str, Any]:
    """Find HTTP routes by optional query, method, or path substring."""
    conn = connection()
    clauses: list[str] = []
    params: dict[str, Any] = {}
    if query.strip():
        clauses.append(
            "(toLower(r.path) CONTAINS $query OR toLower(r.handler) CONTAINS $query OR toLower(r.file_path) CONTAINS $query)"
        )
        params["query"] = query.strip().lower()
    if method.strip():
        clauses.append("toLower(r.method) = $method")
        params["method"] = method.strip().lower()
    if path.strip():
        clauses.append("toLower(r.path) CONTAINS $path")
        params["path"] = path.strip().lower()
    where = f"WHERE {' AND '.join(clauses)}" if clauses else ""
    rows = query_all(
        conn,
        f"""
        MATCH (r:Route)
        {where}
        RETURN r.id AS id, r.method AS method, r.path AS path, r.handler AS handler,
               r.file_path AS file_path, r.line AS line, r.source AS source
        ORDER BY r.path, r.method, r.line
        LIMIT 100
        """,
        params,
    )
    for row in rows:
        handlers = query_all(
            conn,
            """
            MATCH (route:Route {id: $id})-[:HANDLES]->(fn:Function)
            RETURN fn.name AS name, fn.qualified_name AS qualified_name,
                   fn.file_path AS file_path, fn.start_line AS start_line
            """,
            {"id": row["id"]},
        )
        row["resolved_handlers"] = handlers
    return {"query": query, "method": method or None, "path": path or None, "routes": rows}


@mcp.tool()
def find_tests_for_symbol(symbol_or_file: str) -> dict[str, Any]:
    """Find tests linked to a symbol or source file."""
    conn = connection()
    target_file = symbol_or_file
    target_symbol = ""
    seeds = find_symbols(conn, symbol_or_file, ("Function",))
    if seeds:
        target_file = seeds[0]["file_path"]
        target_symbol = seeds[0]["name"]

    tests = query_all(
        conn,
        """
        MATCH (t:Test)
        WHERE t.target_file = $file
           OR ($symbol <> '' AND t.target_symbol = $symbol)
           OR t.file_path CONTAINS $file
        RETURN t.id AS id, t.name AS name, t.file_path AS file_path,
               t.target_file AS target_file, t.target_symbol AS target_symbol, t.line AS line
        ORDER BY t.file_path, t.line
        LIMIT 100
        """,
        {"file": target_file, "symbol": target_symbol},
    )
    return {
        "query": symbol_or_file,
        "target_file": target_file,
        "target_symbol": target_symbol or None,
        "tests": tests,
    }


@mcp.tool()
def impact_analysis(symbol_or_file_or_config: str) -> dict[str, Any]:
    """Expanded blast radius including routes, tests, and subsystem membership."""
    base = get_blast_radius(symbol_or_file_or_config)
    if base.get("error"):
        file_query = symbol_or_file_or_config
        routes = db_query_all(
            """
            MATCH (r:Route)
            WHERE r.file_path CONTAINS $needle OR r.path CONTAINS $needle
            RETURN r.method AS method, r.path AS path, r.handler AS handler,
                   r.file_path AS file_path, r.line AS line
            LIMIT 50
            """,
            {"needle": file_query},
        )
        tests = db_query_all(
            """
            MATCH (t:Test)
            WHERE t.file_path CONTAINS $needle OR t.target_file CONTAINS $needle
            RETURN t.name AS name, t.file_path AS file_path, t.target_file AS target_file, t.line AS line
            LIMIT 50
            """,
            {"needle": file_query},
        )
        return {
            "query": symbol_or_file_or_config,
            "error": base["error"],
            "routes": routes,
            "tests": tests,
        }

    seed_files = {row["path"] for row in base.get("files", [])}
    routes: list[dict[str, Any]] = []
    tests: list[dict[str, Any]] = []
    services: list[dict[str, Any]] = []
    for file_path in sorted(seed_files):
        routes.extend(
            db_query_all(
                """
                MATCH (r:Route {file_path: $file})
                RETURN r.method AS method, r.path AS path, r.handler AS handler, r.line AS line, r.file_path AS file_path
                """,
                {"file": file_path},
            )
        )
        tests.extend(
            db_query_all(
                """
                MATCH (t:Test)
                WHERE t.target_file = $file OR t.file_path = $file
                RETURN t.name AS name, t.file_path AS file_path, t.target_file AS target_file, t.line AS line
                """,
                {"file": file_path},
            )
        )
        services.extend(
            db_query_all(
                """
                MATCH (f:File {path: $file})-[:BELONGS_TO]->(s:Service)
                RETURN DISTINCT s.id AS id, s.name AS name, s.keywords AS keywords
                """,
                {"file": file_path},
            )
        )

    return {
        **base,
        "routes": routes,
        "tests": tests,
        "services": services,
    }


@mcp.tool()
def explain_subsystem(query: str) -> dict[str, Any]:
    """Explain a subsystem by keyword using seeded Service nodes and related graph entities."""
    needle = query.strip().lower()
    if not needle:
        return {"query": query, "error": "query must not be empty"}

    services = db_query_all(
        """
        MATCH (s:Service)
        WHERE toLower(s.name) CONTAINS $needle
           OR toLower(s.id) CONTAINS $needle
           OR toLower(s.keywords) CONTAINS $needle
        RETURN s.id AS id, s.name AS name, s.keywords AS keywords
        ORDER BY s.name
        LIMIT 10
        """,
        {"needle": needle},
    )
    if not services:
        services = db_query_all(
            """
            MATCH (s:Service)
            RETURN s.id AS id, s.name AS name, s.keywords AS keywords
            ORDER BY s.name
            LIMIT 20
            """
        )

    details: list[dict[str, Any]] = []
    for service in services[:5]:
        files = db_query_all(
            """
            MATCH (f:File)-[:BELONGS_TO]->(s:Service {id: $id})
            RETURN f.path AS path
            ORDER BY f.path
            LIMIT 40
            """,
            {"id": service["id"]},
        )
        routes = db_query_all(
            """
            MATCH (r:Route)-[:BELONGS_TO]->(s:Service {id: $id})
            RETURN r.method AS method, r.path AS path, r.handler AS handler, r.file_path AS file_path
            ORDER BY r.path
            LIMIT 40
            """,
            {"id": service["id"]},
        )
        symbols = db_query_all(
            """
            MATCH (fn:Function)-[:BELONGS_TO]->(s:Service {id: $id})
            RETURN fn.name AS name, fn.file_path AS file_path, fn.start_line AS start_line
            ORDER BY fn.file_path, fn.start_line
            LIMIT 40
            """,
            {"id": service["id"]},
        )
        details.append({**service, "files": files, "routes": routes, "symbols": symbols})

    return {"query": query, "services": details}


if __name__ == "__main__":
    mcp.run()
