#!/usr/bin/env python3
from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import shutil
import sys
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable

import kuzu
from tree_sitter import Node, Parser, Query, QueryCursor
from tree_sitter_language_pack import get_language


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
    ".venv",
    "bin",
    "dist",
    "node_modules",
}

QUERY_BY_LANGUAGE = {
    "go": {
        "definitions": r"""
          (function_declaration name: (identifier) @name) @function
          (method_declaration name: (field_identifier) @name) @method
          (type_declaration
            (type_spec name: (type_identifier) @name type: (struct_type) @body) @type) @class
          (type_declaration
            (type_spec name: (type_identifier) @name type: (interface_type) @body) @type) @interface
        """,
        "imports": r"""
          (import_spec
            name: (package_identifier)? @alias
            path: (interpreted_string_literal (interpreted_string_literal_content) @path)) @import
        """,
        "calls": r"""
          (call_expression function: (_) @callee) @call
        """,
        "inherits": r"""
          (type_declaration
            (type_spec name: (type_identifier) @from type:
              (struct_type
                (field_declaration_list
                  (field_declaration type: (_) @target))))) @inherits
          (type_declaration
            (type_spec name: (type_identifier) @from type:
              (interface_type
                (type_elem (type_identifier) @target)))) @inherits
        """,
    },
    "typescript": {
        "definitions": r"""
          (function_declaration name: (identifier) @name) @function
          (method_definition name: (_) @name) @method
          (variable_declarator
            name: (identifier) @name
            value: [(arrow_function) (function_expression)] @value) @variable_function
          (class_declaration name: (type_identifier) @name) @class
          (interface_declaration name: (type_identifier) @name) @interface
        """,
        "imports": r"""
          (import_statement
            source: (string (string_fragment) @path)) @import
        """,
        "calls": r"""
          (call_expression function: (_) @callee) @call
          (new_expression constructor: (_) @callee) @call
          (jsx_opening_element name: (_) @callee) @call
          (jsx_self_closing_element name: (_) @callee) @call
        """,
        "inherits": r"""
          (class_declaration
            name: (type_identifier) @from
            (class_heritage
              (extends_clause value: (_) @target))) @inherits
          (class_declaration
            name: (type_identifier) @from
            (class_heritage
              (implements_clause (_) @target))) @inherits
          (interface_declaration
            name: (type_identifier) @from
            (extends_type_clause (_) @target)) @inherits
        """,
    },
}

QUERY_BY_LANGUAGE["tsx"] = QUERY_BY_LANGUAGE["typescript"]
QUERY_BY_LANGUAGE["javascript"] = QUERY_BY_LANGUAGE["typescript"]


@dataclass(frozen=True)
class Definition:
    kind: str
    id: str
    name: str
    qualified_name: str
    file_path: str
    start_line: int
    end_line: int
    start_byte: int
    end_byte: int
    receiver: str = ""


@dataclass(frozen=True)
class FileRecord:
    path: str
    abspath: str
    language: str
    sha256: str
    byte_count: int


@dataclass(frozen=True)
class ImportRecord:
    file_path: str
    module_id: str
    module_name: str
    module_path: str
    local_path: str
    alias: str
    line: int


@dataclass(frozen=True)
class CallRecord:
    from_id: str
    call_expr: str
    short_name: str
    file_path: str
    line: int
    import_prefix: str


@dataclass(frozen=True)
class InheritRecord:
    from_kind: str
    from_id: str
    target_name: str
    file_path: str
    line: int


class CodeGraphBuilder:
    def __init__(self, repo: Path, db_path: Path) -> None:
        self.repo = repo.resolve()
        self.db_path = db_path.resolve()
        self.module_name = read_go_module(self.repo)
        self.files: list[FileRecord] = []
        self.definitions: list[Definition] = []
        self.imports: list[ImportRecord] = []
        self.calls: list[CallRecord] = []
        self.inherits: list[InheritRecord] = []
        self.file_import_aliases: dict[str, dict[str, ImportRecord]] = {}
        self.source_by_file: dict[str, bytes] = {}
        self.parser_by_language: dict[str, Parser] = {}

    def build(self) -> dict[str, int | str]:
        source_files = list(iter_source_files(self.repo))
        parsed_files = 0
        for path in source_files:
            parsed_files += self.parse_file(path)

        self.write_database()
        return {
            "repo": str(self.repo),
            "db": str(self.db_path),
            "files": len(self.files),
            "parsed_files": parsed_files,
            "definitions": len(self.definitions),
            "imports": len(self.imports),
            "calls": len(self.calls),
            "inheritance_edges": len(self.inherits),
        }

    def parser_for(self, language: str) -> Parser:
        parser = self.parser_by_language.get(language)
        if parser is not None:
            return parser
        parser = Parser()
        parser.language = get_language(language)
        self.parser_by_language[language] = parser
        return parser

    def parse_file(self, path: Path) -> int:
        language = EXTENSION_LANGUAGES.get(path.suffix.lower())
        if language is None:
            return 0

        rel_path = to_posix(path.relative_to(self.repo))
        try:
            source = path.read_bytes()
            source.decode("utf-8")
        except UnicodeDecodeError:
            return 0
        except OSError as exc:
            print(f"skip {rel_path}: {exc}", file=sys.stderr)
            return 0

        self.files.append(
            FileRecord(
                path=rel_path,
                abspath=str(path.resolve()),
                language=language,
                sha256=hashlib.sha256(source).hexdigest(),
                byte_count=len(source),
            )
        )
        self.source_by_file[rel_path] = source

        try:
            parser = self.parser_for(language)
            tree = parser.parse(source)
        except Exception as exc:
            print(f"parse failed {rel_path}: {exc}", file=sys.stderr)
            return 0

        root = tree.root_node
        if root.has_error:
            print(f"warning: tree-sitter reported parse errors in {rel_path}", file=sys.stderr)

        package_name = extract_go_package(root, source) if language == "go" else ""
        self.collect_imports(language, rel_path, root, source)
        self.collect_definitions(language, rel_path, root, source, package_name)
        self.collect_inherits(language, rel_path, root, source)
        return 1

    def collect_imports(self, language: str, rel_path: str, root: Node, source: bytes) -> None:
        query = query_text(language, "imports")
        if query is None:
            return

        aliases = self.file_import_aliases.setdefault(rel_path, {})
        for match in run_query(language, query, root):
            import_nodes = match.get("import", [])
            path_nodes = match.get("path", [])
            if not import_nodes or not path_nodes:
                continue
            import_node = import_nodes[0]
            module_path = node_text(path_nodes[0], source)
            if not module_path:
                continue

            if language == "go":
                alias_node = first(match.get("alias", []))
                alias = node_text(alias_node, source) if alias_node is not None else module_path.rsplit("/", 1)[-1]
                local_path = resolve_go_import(self.module_name, module_path)
                imported_aliases = [alias]
            else:
                alias = extract_ts_import_aliases(import_node, source)
                local_path = resolve_ts_import(self.repo, rel_path, module_path)
                imported_aliases = [part.strip() for part in alias.split(",") if part.strip()]

            module_id = f"local:{local_path}" if local_path else f"module:{module_path}"
            module_name = Path(local_path).name if local_path else module_path.rsplit("/", 1)[-1]
            record = ImportRecord(
                file_path=rel_path,
                module_id=module_id,
                module_name=module_name,
                module_path=module_path,
                local_path=local_path,
                alias=alias,
                line=import_node.start_point[0] + 1,
            )
            self.imports.append(record)
            for imported_alias in imported_aliases:
                aliases[imported_alias] = record

    def collect_definitions(
        self,
        language: str,
        rel_path: str,
        root: Node,
        source: bytes,
        package_name: str,
    ) -> None:
        query = query_text(language, "definitions")
        if query is None:
            return

        for match in run_query(language, query, root):
            if "function" in match:
                node = match["function"][0]
                name = node_text(match["name"][0], source)
                qualified_name = f"{package_name}.{name}" if package_name else name
                self.add_definition("Function", rel_path, node, name, qualified_name)
            elif "method" in match:
                node = match["method"][0]
                name = node_text(match["name"][0], source)
                receiver = extract_go_receiver(node, source) if language == "go" else enclosing_class_name(node, source)
                qualified_name = f"{receiver}.{name}" if receiver else name
                self.add_definition("Function", rel_path, node, name, qualified_name, receiver)
            elif "variable_function" in match:
                node = enclosing_declaration(match["variable_function"][0])
                name = node_text(match["name"][0], source)
                self.add_definition("Function", rel_path, node, name, name)
            elif "class" in match:
                node = match["class"][0]
                name = node_text(match["name"][0], source)
                self.add_definition("Class", rel_path, node, name, name)
            elif "interface" in match:
                node = match["interface"][0]
                name = node_text(match["name"][0], source)
                self.add_definition("Interface", rel_path, node, name, name)

        file_defs = [d for d in self.definitions if d.file_path == rel_path and d.kind == "Function"]
        self.collect_calls(language, rel_path, root, source, file_defs)

    def collect_calls(
        self,
        language: str,
        rel_path: str,
        root: Node,
        source: bytes,
        file_functions: list[Definition],
    ) -> None:
        query = query_text(language, "calls")
        if query is None:
            return

        for match in run_query(language, query, root):
            call_node = first(match.get("call", []))
            callee_node = first(match.get("callee", []))
            if call_node is None or callee_node is None:
                continue
            owner = enclosing_definition(call_node, file_functions)
            if owner is None:
                continue
            call_expr, short_name, import_prefix = normalize_callee(callee_node, source)
            if not short_name:
                continue
            self.calls.append(
                CallRecord(
                    from_id=owner.id,
                    call_expr=call_expr,
                    short_name=short_name,
                    file_path=rel_path,
                    line=call_node.start_point[0] + 1,
                    import_prefix=import_prefix,
                )
            )

    def collect_inherits(self, language: str, rel_path: str, root: Node, source: bytes) -> None:
        query = query_text(language, "inherits")
        if query is None:
            return

        def_by_name = {
            definition.name: definition
            for definition in self.definitions
            if definition.file_path == rel_path and definition.kind in {"Class", "Interface"}
        }
        for match in run_query(language, query, root):
            from_node = first(match.get("from", []))
            target_node = first(match.get("target", []))
            inherit_node = first(match.get("inherits", [])) or target_node
            if from_node is None or target_node is None or inherit_node is None:
                continue
            if language == "go":
                parent = target_node.parent
                if parent is not None and parent.type == "field_declaration" and parent.child_by_field_name("name") is not None:
                    continue
            from_name = node_text(from_node, source)
            source_def = def_by_name.get(from_name)
            if source_def is None:
                continue
            target_name = base_type_name(node_text(target_node, source))
            if target_name and target_name != from_name:
                self.inherits.append(
                    InheritRecord(
                        from_kind=source_def.kind,
                        from_id=source_def.id,
                        target_name=target_name,
                        file_path=rel_path,
                        line=inherit_node.start_point[0] + 1,
                    )
                )

    def add_definition(
        self,
        kind: str,
        rel_path: str,
        node: Node,
        name: str,
        qualified_name: str,
        receiver: str = "",
    ) -> None:
        if not name:
            return
        start_line = node.start_point[0] + 1
        definition = Definition(
            kind=kind,
            id=f"{kind.lower()}:{rel_path}:{qualified_name}:{start_line}",
            name=name,
            qualified_name=qualified_name,
            file_path=rel_path,
            start_line=start_line,
            end_line=node.end_point[0] + 1,
            start_byte=node.start_byte,
            end_byte=node.end_byte,
            receiver=receiver,
        )
        self.definitions.append(definition)

    def write_database(self) -> None:
        reset_database_path(self.db_path)
        self.db_path.parent.mkdir(parents=True, exist_ok=True)
        conn = kuzu.Connection(kuzu.Database(str(self.db_path)))
        create_schema(conn)

        indexed_at = int(time.time())
        for file_record in self.files:
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

        for definition in self.definitions:
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

        for import_record in self.imports:
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

        for inherit_record in self.inherits:
            target = self.resolve_type(inherit_record.target_name, inherit_record.file_path)
            if target is None:
                continue
            conn.execute(
                f"""
                MATCH (a:{inherit_record.from_kind} {{id: $from_id}}), (b:{target.kind} {{id: $target_id}})
                CREATE (a)-[:INHERITS_FROM {{reason: 'ast-inheritance', line: $line}}]->(b)
                """,
                {
                    "from_id": inherit_record.from_id,
                    "target_id": target.id,
                    "line": inherit_record.line,
                },
            )

        for call_record in self.calls:
            target = self.resolve_call(call_record)
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

    def resolve_type(self, target_name: str, file_path: str) -> Definition | None:
        candidates = [
            d
            for d in self.definitions
            if d.kind in {"Class", "Interface"} and d.name == target_name
        ]
        if not candidates:
            return None
        same_file = [d for d in candidates if d.file_path == file_path]
        return first(same_file) or first(candidates)

    def resolve_call(self, call: CallRecord) -> Definition | None:
        if call.import_prefix:
            imported = self.file_import_aliases.get(call.file_path, {}).get(call.import_prefix)
            if imported is not None and imported.local_path:
                imported_candidates = [
                    d
                    for d in self.definitions
                    if definition_is_under_import(d, imported.local_path)
                    and d.name == call.short_name
                    and d.kind in {"Function", "Class"}
                ]
                if imported_candidates:
                    return prefer_definition(imported_candidates, call.file_path)

        same_file_candidates = [
            d
            for d in self.definitions
            if d.file_path == call.file_path
            and d.name == call.short_name
            and d.kind in {"Function", "Class"}
        ]
        if same_file_candidates:
            return prefer_definition(same_file_candidates, call.file_path)

        exact_qualified = [
            d
            for d in self.definitions
            if d.qualified_name == call.call_expr and d.kind in {"Function", "Class"}
        ]
        if exact_qualified:
            return prefer_definition(exact_qualified, call.file_path)

        if call.import_prefix:
            return None

        global_candidates = [
            d
            for d in self.definitions
            if d.name == call.short_name and d.kind in {"Function", "Class"}
        ]
        if len(global_candidates) == 1:
            return global_candidates[0]
        return None


def create_schema(conn: kuzu.Connection) -> None:
    statements = [
        """
        CREATE NODE TABLE File(
            path STRING,
            abspath STRING,
            language STRING,
            sha256 STRING,
            byte_count INT64,
            indexed_at INT64,
            PRIMARY KEY(path)
        )
        """,
        """
        CREATE NODE TABLE Function(
            id STRING,
            name STRING,
            qualified_name STRING,
            file_path STRING,
            start_line INT64,
            end_line INT64,
            start_byte INT64,
            end_byte INT64,
            receiver STRING,
            PRIMARY KEY(id)
        )
        """,
        """
        CREATE NODE TABLE Class(
            id STRING,
            name STRING,
            qualified_name STRING,
            file_path STRING,
            start_line INT64,
            end_line INT64,
            start_byte INT64,
            end_byte INT64,
            receiver STRING,
            PRIMARY KEY(id)
        )
        """,
        """
        CREATE NODE TABLE Interface(
            id STRING,
            name STRING,
            qualified_name STRING,
            file_path STRING,
            start_line INT64,
            end_line INT64,
            start_byte INT64,
            end_byte INT64,
            receiver STRING,
            PRIMARY KEY(id)
        )
        """,
        """
        CREATE NODE TABLE ImportModule(
            id STRING,
            name STRING,
            path STRING,
            local_path STRING,
            PRIMARY KEY(id)
        )
        """,
        "CREATE REL TABLE DEFINES(FROM File TO Function, FROM File TO Class, FROM File TO Interface)",
        "CREATE REL TABLE IMPORTS(FROM File TO ImportModule, alias STRING, local_path STRING, line INT64)",
        "CREATE REL TABLE CALLS(FROM Function TO Function, FROM Function TO Class, call_expr STRING, line INT64, resolved_by STRING)",
        "CREATE REL TABLE INHERITS_FROM(FROM Class TO Class, FROM Class TO Interface, FROM Interface TO Interface, reason STRING, line INT64)",
    ]
    for statement in statements:
        conn.execute(statement)


def iter_source_files(repo: Path) -> Iterable[Path]:
    for root, dirs, files in os.walk(repo):
        root_path = Path(root)
        dirs[:] = [d for d in dirs if d not in SKIP_DIRS]
        for name in files:
            path = root_path / name
            if path.suffix.lower() in EXTENSION_LANGUAGES:
                yield path


def query_text(language: str, query_name: str) -> str | None:
    group = QUERY_BY_LANGUAGE.get(language)
    if group is None:
        return None
    return group.get(query_name)


def run_query(language: str, query_text_value: str, root: Node) -> list[dict[str, list[Node]]]:
    language_obj = get_language(language)
    query = Query(language_obj, query_text_value)
    return [captures for _, captures in QueryCursor(query).matches(root)]


def node_text(node: Node | None, source: bytes) -> str:
    if node is None:
        return ""
    return source[node.start_byte : node.end_byte].decode("utf-8", "replace")


def first(values: Iterable):
    for value in values:
        return value
    return None


def to_posix(path: Path) -> str:
    return path.as_posix()


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


def read_go_module(repo: Path) -> str:
    go_mod = repo / "go.mod"
    if not go_mod.exists():
        return ""
    for line in go_mod.read_text(encoding="utf-8", errors="replace").splitlines():
        match = re.match(r"\s*module\s+(\S+)", line)
        if match:
            return match.group(1)
    return ""


def resolve_go_import(module_name: str, module_path: str) -> str:
    if module_name and module_path == module_name:
        return "."
    if module_name and module_path.startswith(module_name + "/"):
        return module_path[len(module_name) + 1 :]
    return ""


def resolve_ts_import(repo: Path, rel_path: str, module_path: str) -> str:
    if not module_path.startswith("."):
        return ""
    start = (repo / rel_path).parent / module_path
    candidates = [
        start,
        start.with_suffix(".ts"),
        start.with_suffix(".tsx"),
        start.with_suffix(".js"),
        start.with_suffix(".jsx"),
        start / "index.ts",
        start / "index.tsx",
        start / "index.js",
        start / "index.jsx",
    ]
    for candidate in candidates:
        if candidate.exists() and candidate.is_file():
            return to_posix(candidate.resolve().relative_to(repo))
    return to_posix(start.resolve().relative_to(repo))


def extract_go_package(root: Node, source: bytes) -> str:
    for child in root.children:
        if child.type == "package_clause":
            for part in child.children:
                if part.type == "package_identifier":
                    return node_text(part, source)
    return ""


def extract_go_receiver(node: Node, source: bytes) -> str:
    receiver = node.child_by_field_name("receiver")
    if receiver is None:
        return ""
    text = node_text(receiver, source).strip()
    text = text.strip("()")
    parts = text.split()
    if not parts:
        return ""
    return base_type_name(parts[-1])


def enclosing_class_name(node: Node, source: bytes) -> str:
    parent = node.parent
    while parent is not None:
        if parent.type == "class_declaration":
            name = parent.child_by_field_name("name")
            return node_text(name, source)
        parent = parent.parent
    return ""


def enclosing_declaration(node: Node) -> Node:
    parent = node.parent
    while parent is not None:
        if parent.type in {"lexical_declaration", "variable_declaration"}:
            return parent
        if parent.type in {"export_statement"} and parent.parent is not None:
            return parent
        parent = parent.parent
    return node


def enclosing_definition(node: Node, definitions: list[Definition]) -> Definition | None:
    candidates = [
        definition
        for definition in definitions
        if definition.start_byte <= node.start_byte and node.end_byte <= definition.end_byte
    ]
    if not candidates:
        return None
    return min(candidates, key=lambda d: d.end_byte - d.start_byte)


def normalize_callee(node: Node, source: bytes) -> tuple[str, str, str]:
    raw = compact_expression(node_text(node, source))
    if not raw:
        return "", "", ""

    if node.type in {"selector_expression", "member_expression"}:
        parts = [p for p in re.split(r"[.?]+", raw) if p]
        if not parts:
            return raw, raw, ""
        return raw, parts[-1], parts[0] if len(parts) > 1 else ""

    if node.type in {"identifier", "type_identifier", "property_identifier"}:
        return raw, raw, ""

    if "." in raw:
        parts = [p for p in raw.split(".") if p]
        return raw, parts[-1], parts[0] if len(parts) > 1 else ""

    cleaned = base_type_name(raw)
    return raw, cleaned, ""


def compact_expression(value: str) -> str:
    return re.sub(r"\s+", "", value.strip())


def base_type_name(value: str) -> str:
    cleaned = value.strip()
    cleaned = cleaned.strip("&*[]")
    cleaned = cleaned.replace("React.", "")
    cleaned = cleaned.split("[", 1)[0]
    cleaned = cleaned.split("{", 1)[0]
    cleaned = cleaned.rsplit(".", 1)[-1]
    cleaned = cleaned.rsplit("/", 1)[-1]
    return re.sub(r"[^A-Za-z0-9_$]", "", cleaned)


def extract_ts_import_aliases(import_node: Node, source: bytes) -> str:
    clause = None
    for child in import_node.children:
        if child.type == "import_clause":
            clause = child
            break
    if clause is None:
        return ""
    names: list[str] = []
    for child in walk(clause):
        if child.type in {"identifier", "namespace_import"}:
            text = node_text(child, source)
            if text not in {"as"}:
                names.append(text)
    return ",".join(dict.fromkeys(names))


def walk(node: Node) -> Iterable[Node]:
    stack = [node]
    while stack:
        current = stack.pop()
        yield current
        stack.extend(reversed(current.children))


def definition_is_under_import(definition: Definition, local_path: str) -> bool:
    if not local_path:
        return False
    if Path(local_path).suffix:
        return definition.file_path == local_path
    return definition.file_path == local_path or definition.file_path.startswith(local_path.rstrip("/") + "/")


def prefer_definition(candidates: list[Definition], current_file: str) -> Definition:
    same_file = [candidate for candidate in candidates if candidate.file_path == current_file]
    if same_file:
        return same_file[0]
    return sorted(candidates, key=lambda d: (d.file_path, d.start_line))[0]


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Build a deterministic tree-sitter/Kuzu code graph.")
    parser.add_argument("--repo", type=Path, default=Path.cwd(), help="Repository root to index.")
    parser.add_argument("--db", type=Path, default=Path(".codegraph/streamclone.kuzu"), help="Kuzu database path.")
    parser.add_argument("--json", action="store_true", help="Emit machine-readable summary.")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    builder = CodeGraphBuilder(args.repo, args.db)
    summary = builder.build()
    if args.json:
        print(json.dumps(summary, indent=2, sort_keys=True))
    else:
        print(
            "Indexed {files} source files, {definitions} definitions, {imports} imports, "
            "{calls} resolved calls, {inheritance_edges} inheritance edges into {db}".format(**summary)
        )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
