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

import kuzu
from mcp.server.fastmcp import FastMCP


NODE_TABLES = ("Function", "Class", "Interface", "ImportModule")


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
    if not DB_PATH.exists():
        raise FileNotFoundError(f"Code graph database not found at {DB_PATH}. Run tools/codegraph/codegraph_ingest.py first.")
    return kuzu.Connection(kuzu.Database(str(DB_PATH), read_only=True))


def query_all(query: str, params: dict[str, Any] | None = None) -> list[dict[str, Any]]:
    conn = connection()
    result = conn.execute(query, params or {})
    columns = result.get_column_names()
    return [dict(zip(columns, row, strict=False)) for row in result.get_all()]


def find_symbols(symbol_name: str, tables: tuple[str, ...] = NODE_TABLES) -> list[dict[str, Any]]:
    matches: list[dict[str, Any]] = []
    for table in tables:
        if table == "ImportModule":
            rows = query_all(
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


def outgoing_calls(function_id: str) -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    for table in ("Function", "Class"):
        rows.extend(
            query_all(
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


def incoming_calls(symbol: dict[str, Any]) -> list[dict[str, Any]]:
    if symbol["kind"] not in {"Function", "Class"}:
        return []
    return query_all(
        f"""
        MATCH (a:Function)-[r:CALLS]->(b:{symbol["kind"]} {{id: $id}})
        RETURN 'incoming' AS direction, 'Function' AS source_kind,
               a.id AS source_id, a.name AS source_name, a.qualified_name AS source_qualified_name,
               a.file_path AS source_file, a.start_line AS source_line,
               r.call_expr AS call_expr, r.line AS call_line
        """,
        {"id": symbol["id"]},
    )


@mcp.tool()
def get_call_chain(function_name: str, depth: int = 3) -> dict[str, Any]:
    """Trace direct and indirect callers and callees for a function up to 3 hops deep."""
    max_depth = max(1, min(int(depth), 3))
    seeds = find_symbols(function_name, ("Function",))
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
                for edge in outgoing_calls(current_id):
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
                for edge in incoming_calls(current_symbol):
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


@mcp.tool()
def get_blast_radius(symbol_name: str) -> dict[str, Any]:
    """Find files, functions, imports, and direct graph edges that rely on a symbol."""
    seeds = find_symbols(symbol_name)
    if not seeds:
        return {"query": symbol_name, "error": "No symbol matched this name, qualified name, module path, or id.", "seeds": []}

    files: dict[str, dict[str, Any]] = {}
    functions: dict[str, dict[str, Any]] = {}
    classes: dict[str, dict[str, Any]] = {}
    imports: dict[str, dict[str, Any]] = {}
    edges: list[dict[str, Any]] = []

    all_import_edges = query_all(
        """
        MATCH (f:File)-[r:IMPORTS]->(m:ImportModule)
        RETURN f.path AS file_path, m.id AS module_id, m.name AS module_name,
               m.path AS module_path, m.local_path AS local_path,
               r.alias AS alias, r.line AS line
        """
    )

    for seed in seeds:
        if seed["kind"] in {"Function", "Class", "Interface"}:
            files[seed["file_path"]] = {"path": seed["file_path"], "reason": "defines seed"}

            for row in query_all(
                f"""
                MATCH (f:File)-[:DEFINES]->(n:{seed["kind"]} {{id: $id}})
                RETURN f.path AS file_path
                """,
                {"id": seed["id"]},
            ):
                files[row["file_path"]] = {"path": row["file_path"], "reason": "defines seed"}

            if seed["kind"] in {"Function", "Class"}:
                for edge in incoming_calls(seed):
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
                for edge in outgoing_calls(seed["id"]):
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
def get_ast_chunk(function_name: str) -> dict[str, Any]:
    """Return exact source text bounded by the matched function's tree-sitter node."""
    matches = find_symbols(function_name, ("Function",))
    if not matches:
        return {"query": function_name, "error": "No function matched this name, qualified name, or id.", "matches": []}

    chunks: list[dict[str, Any]] = []
    for match in matches:
        path = REPO / match["file_path"]
        content = path.read_bytes()
        indexed_file = query_all(
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
    return {"query": function_name, "matches": chunks}


def inheritance_edges(seed: dict[str, Any]) -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    for table in ("Class", "Interface"):
        rows.extend(
            query_all(
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
            query_all(
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
def search_symbols(query: str, kind: str = "", limit: int = 25) -> dict[str, Any]:
    """Search function/class/interface names by substring (case-insensitive)."""
    needle = query.strip().lower()
    if not needle:
        return {"query": query, "error": "query must not be empty", "matches": []}
    max_rows = max(1, min(int(limit), 100))
    tables = NODE_TABLES if kind.strip() == "" else (kind.strip(),)
    invalid = [table for table in tables if table not in NODE_TABLES]
    if invalid:
        return {
            "query": query,
            "error": f"invalid kind {invalid!r}",
            "allowed_kinds": list(NODE_TABLES),
            "matches": [],
        }

    matches: list[dict[str, Any]] = []
    for table in tables:
        if table == "ImportModule":
            rows = query_all(
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
    matches = sorted(matches, key=lambda row: (row["kind"], row["file_path"], row["start_line"]))[:max_rows]
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
    for label in ("File", "Function", "Class", "Interface", "ImportModule"):
        rows = query_all(f"MATCH (n:{label}) RETURN count(n) AS count")
        counts[label.lower()] = int(rows[0]["count"]) if rows else 0
    extensions: dict[str, int] = {}
    for row in query_all("MATCH (f:File) RETURN f.path AS path"):
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


if __name__ == "__main__":
    mcp.run()
