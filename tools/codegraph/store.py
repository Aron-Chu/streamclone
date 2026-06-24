"""Persist parsed graph records into Kuzu."""

from __future__ import annotations

import shutil
import time
from dataclasses import dataclass
from pathlib import Path

import kuzu

from tools.codegraph.extractors.domain import (
    ClientCallRecord,
    RouteRecord,
    ServiceRecord,
    TestRecord,
    file_belongs_to_service,
    route_belongs_to_service,
)
from tools.codegraph.extractors.treesitter import Definition, ImportRecord
from tools.codegraph.schema import create_schema


@dataclass(frozen=True)
class FileRecord:
    path: str
    abspath: str
    language: str
    sha256: str
    byte_count: int


def reset_database_path(db_path: Path) -> None:
    if db_path.exists():
        if db_path.is_dir():
            shutil.rmtree(db_path)
        else:
            db_path.unlink()
    wal_path = db_path.with_name(f"{db_path.name}.wal")
    if wal_path.exists():
        if wal_path.is_dir():
            shutil.rmtree(wal_path)
        else:
            wal_path.unlink()


def write_graph(
    db_path: Path,
    files: list[FileRecord],
    definitions: list[Definition],
    imports: list[ImportRecord],
    calls: list[tuple],
    inherits: list[tuple],
    routes: list[RouteRecord],
    tests: list[TestRecord],
    services: list[ServiceRecord],
    client_calls: list[ClientCallRecord],
    resolve_call,
    resolve_type,
    resolve_handler,
    resolve_test_target,
) -> None:
    reset_database_path(db_path)
    db_path.parent.mkdir(parents=True, exist_ok=True)
    conn = kuzu.Connection(kuzu.Database(str(db_path)))
    create_schema(conn)

    indexed_at = int(time.time())
    for file_record in files:
        conn.execute(
            """
            MERGE (f:File {path: $path})
            SET f.abspath = $abspath,
                f.language = $language,
                f.sha256 = $sha256,
                f.byte_count = $byte_count,
                f.indexed_at = $indexed_at
            """,
            {
                "path": file_record.path,
                "abspath": file_record.abspath,
                "language": file_record.language,
                "sha256": file_record.sha256,
                "byte_count": file_record.byte_count,
                "indexed_at": indexed_at,
            },
        )

    for definition in definitions:
        table = definition.kind
        definition_params = {
            "id": definition.id,
            "name": definition.name,
            "qualified_name": definition.qualified_name,
            "file_path": definition.file_path,
            "start_line": definition.start_line,
            "end_line": definition.end_line,
            "start_byte": definition.start_byte,
            "end_byte": definition.end_byte,
            "receiver": definition.receiver,
        }
        conn.execute(
            f"""
            MERGE (n:{table} {{id: $id}})
            SET n.name = $name,
                n.qualified_name = $qualified_name,
                n.file_path = $file_path,
                n.start_line = $start_line,
                n.end_line = $end_line,
                n.start_byte = $start_byte,
                n.end_byte = $end_byte,
                n.receiver = $receiver
            """,
            definition_params,
        )
        conn.execute(
            f"""
            MATCH (f:File {{path: $file_path}}), (n:{table} {{id: $id}})
            CREATE (f)-[:DEFINES]->(n)
            """,
            {"file_path": definition.file_path, "id": definition.id},
        )

    for import_record in imports:
        conn.execute(
            """
            MERGE (m:ImportModule {id: $module_id})
            SET m.name = $module_name,
                m.path = $module_path,
                m.local_path = $local_path
            """,
            {
                "module_id": import_record.module_id,
                "module_name": import_record.module_name,
                "module_path": import_record.module_path,
                "local_path": import_record.local_path,
            },
        )
        conn.execute(
            """
            MATCH (f:File {path: $file_path}), (m:ImportModule {id: $module_id})
            CREATE (f)-[:IMPORTS {alias: $alias, local_path: $local_path, line: $line}]->(m)
            """,
            {
                "file_path": import_record.file_path,
                "module_id": import_record.module_id,
                "alias": import_record.alias,
                "local_path": import_record.local_path,
                "line": import_record.line,
            },
        )

    for inherit_record in inherits:
        from_kind, from_id, target, line = inherit_record
        if target is None:
            continue
        conn.execute(
            f"""
            MATCH (a:{from_kind} {{id: $from_id}}), (b:{target.kind} {{id: $target_id}})
            CREATE (a)-[:INHERITS_FROM {{reason: 'ast-inheritance', line: $line}}]->(b)
            """,
            {"from_id": from_id, "target_id": target.id, "line": line},
        )

    for call_record, target in calls:
        if target is None:
            continue
        conn.execute(
            f"""
            MATCH (a:Function {{id: $from_id}}), (b:{target.kind} {{id: $target_id}})
            CREATE (a)-[:CALLS {{
                call_expr: $call_expr,
                line: $line,
                resolved_by: $resolved_by
            }}]->(b)
            """,
            {
                "from_id": call_record.from_id,
                "target_id": target.id,
                "call_expr": call_record.call_expr,
                "line": call_record.line,
                "resolved_by": "tree-sitter-name-and-import",
            },
        )

    for route in routes:
        conn.execute(
            """
            MERGE (r:Route {id: $id})
            SET r.method = $method,
                r.path = $path,
                r.handler = $handler,
                r.file_path = $file_path,
                r.line = $line,
                r.source = $source
            """,
            {
                "id": route.id,
                "method": route.method,
                "path": route.path,
                "handler": route.handler,
                "file_path": route.file_path,
                "line": route.line,
                "source": route.source,
            },
        )
        handler_fn = resolve_handler(route)
        if handler_fn is not None:
            conn.execute(
                """
                MATCH (r:Route {id: $route_id}), (f:Function {id: $fn_id})
                CREATE (r)-[:HANDLES {handler_expr: $handler}]->(f)
                """,
                {"route_id": route.id, "fn_id": handler_fn.id, "handler": route.handler},
            )

    for test in tests:
        conn.execute(
            """
            MERGE (t:Test {id: $id})
            SET t.name = $name,
                t.file_path = $file_path,
                t.target_file = $target_file,
                t.target_symbol = $target_symbol,
                t.line = $line
            """,
            {
                "id": test.id,
                "name": test.name,
                "file_path": test.file_path,
                "target_file": test.target_file,
                "target_symbol": test.target_symbol,
                "line": test.line,
            },
        )
        conn.execute(
            """
            MATCH (t:Test {id: $id}), (f:File {path: $target_file})
            CREATE (t)-[:TESTS {reason: 'test-file-target'}]->(f)
            """,
            {"id": test.id, "target_file": test.target_file},
        )
        target_fn = resolve_test_target(test)
        if target_fn is not None:
            conn.execute(
                """
                MATCH (t:Test {id: $id}), (fn:Function {id: $fn_id})
                CREATE (t)-[:TESTS {reason: 'symbol-name-heuristic'}]->(fn)
                """,
                {"id": test.id, "fn_id": target_fn.id},
            )

    for service in services:
        conn.execute(
            """
            MERGE (s:Service {id: $id})
            SET s.name = $name,
                s.description = $description,
                s.keywords = $keywords
            """,
            {
                "id": service.id,
                "name": service.name,
                "description": service.description,
                "keywords": service.keywords,
            },
        )

    for file_record in files:
        for service in services:
            if file_belongs_to_service(file_record.path, service):
                conn.execute(
                    """
                    MATCH (f:File {path: $path}), (s:Service {id: $service_id})
                    CREATE (f)-[:BELONGS_TO]->(s)
                    """,
                    {"path": file_record.path, "service_id": service.id},
                )

    for definition in definitions:
        if definition.kind != "Function":
            continue
        for service in services:
            if file_belongs_to_service(definition.file_path, service):
                conn.execute(
                    """
                    MATCH (fn:Function {id: $id}), (s:Service {id: $service_id})
                    CREATE (fn)-[:BELONGS_TO]->(s)
                    """,
                    {"id": definition.id, "service_id": service.id},
                )

    for route in routes:
        for service in services:
            if route_belongs_to_service(route.path, service):
                conn.execute(
                    """
                    MATCH (r:Route {id: $id}), (s:Service {id: $service_id})
                    CREATE (r)-[:BELONGS_TO]->(s)
                    """,
                    {"id": route.id, "service_id": service.id},
                )

    route_by_path = {route.path: route for route in routes}
    for client_call in client_calls:
        route = route_by_path.get(client_call.route_path)
        if route is None:
            for candidate in routes:
                if client_call.route_path.startswith(candidate.path.rstrip("/")):
                    route = candidate
                    break
        if route is None:
            continue
        conn.execute(
            """
            MATCH (f:File {path: $file_path}), (r:Route {id: $route_id})
            CREATE (f)-[:CLIENT_CALLS {call_expr: $call_expr, line: $line}]->(r)
            """,
            {
                "file_path": client_call.file_path,
                "route_id": route.id,
                "call_expr": client_call.call_expr,
                "line": client_call.line,
            },
        )

    try:
        conn.execute("CHECKPOINT")
    except Exception:
        pass
    del conn
