"""Build the Streamclone code graph from repository sources."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import sys
import time
from dataclasses import dataclass
from pathlib import Path

from tree_sitter import Node, Parser
from tree_sitter_language_pack import PackConfig, configure, get_language

from tools.codegraph.extractors.domain import (
    ClientCallRecord,
    RouteRecord,
    TestRecord,
    extract_frontend_client_calls,
    extract_go_routes,
    extract_tests,
    load_services,
)
from tools.codegraph.extractors.treesitter import (
    CallRecord,
    Definition,
    ImportRecord,
    InheritRecord,
    base_type_name,
    definition_is_under_import,
    enclosing_class_name,
    enclosing_declaration,
    enclosing_definition,
    extract_go_package,
    extract_go_receiver,
    extract_ts_import_aliases,
    first,
    node_text,
    normalize_callee,
    prefer_definition,
    query_text,
    read_go_module,
    resolve_go_import,
    resolve_ts_import,
    run_query,
)
from tools.codegraph.store import FileRecord, write_graph
from tools.codegraph.walker import EXTENSION_LANGUAGES, iter_source_files, to_posix


def configure_tree_sitter_cache(repo: Path) -> None:
    cache = Path(os.environ.get("TREE_SITTER_LANGUAGE_PACK_CACHE_DIR", repo / ".codegraph" / "tree-sitter-cache"))
    cache.mkdir(parents=True, exist_ok=True)
    configure(PackConfig(cache_dir=str(cache)))


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
        self.routes: list[RouteRecord] = []
        self.tests: list[TestRecord] = []
        self.client_calls: list[ClientCallRecord] = []
        self.services = load_services(self.repo / "tools" / "codegraph" / "subsystems.json")
        self.file_import_aliases: dict[str, dict[str, ImportRecord]] = {}
        self.source_by_file: dict[str, bytes] = {}
        self.source_text_by_file: dict[str, str] = {}
        self.parser_by_language: dict[str, Parser] = {}

    def build(self) -> dict[str, int | str]:
        source_files = list(iter_source_files(self.repo))
        parsed_files = 0
        for path in source_files:
            parsed_files += self.parse_file(path)

        self.extract_domain_records()
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
            "routes": len(self.routes),
            "tests": len(self.tests),
            "services": len(self.services),
            "client_calls": len(self.client_calls),
        }

    def extract_domain_records(self) -> None:
        for rel_path, text in self.source_text_by_file.items():
            language = next((f.language for f in self.files if f.path == rel_path), "")
            if language == "go":
                self.routes.extend(extract_go_routes(text, rel_path))
            if language in {"tsx", "typescript"}:
                self.client_calls.extend(extract_frontend_client_calls(text, rel_path))
            if rel_path.endswith("_test.go") or ".test." in rel_path or ".spec." in rel_path:
                lang_key = "go" if rel_path.endswith("_test.go") else "tsx"
                self.tests.extend(extract_tests(text, rel_path, lang_key))

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
        self.source_text_by_file[rel_path] = source.decode("utf-8", "replace")

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
                if language == "python":
                    receiver = enclosing_class_name(node, source)
                    qualified_name = f"{receiver}.{name}" if receiver else name
                else:
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
        resolved_calls = [(call, self.resolve_call(call)) for call in self.calls]
        resolved_inherits = [
            (record.from_kind, record.from_id, self.resolve_type(record.target_name, record.file_path), record.line)
            for record in self.inherits
        ]
        write_graph(
            self.db_path,
            self.files,
            self.definitions,
            self.imports,
            resolved_calls,
            resolved_inherits,
            self.routes,
            self.tests,
            self.services,
            self.client_calls,
            resolve_call=self.resolve_call,
            resolve_type=self.resolve_type,
            resolve_handler=self.resolve_handler,
            resolve_test_target=self.resolve_test_target,
        )

    def resolve_type(self, target_name: str, file_path: str) -> Definition | None:
        candidates = [
            d for d in self.definitions if d.kind in {"Class", "Interface"} and d.name == target_name
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
            if d.file_path == call.file_path and d.name == call.short_name and d.kind in {"Function", "Class"}
        ]
        if same_file_candidates:
            return prefer_definition(same_file_candidates, call.file_path)

        exact_qualified = [
            d for d in self.definitions if d.qualified_name == call.call_expr and d.kind in {"Function", "Class"}
        ]
        if exact_qualified:
            return prefer_definition(exact_qualified, call.file_path)

        if call.import_prefix:
            return None

        global_candidates = [
            d for d in self.definitions if d.name == call.short_name and d.kind in {"Function", "Class"}
        ]
        if len(global_candidates) == 1:
            return global_candidates[0]
        return None

    def resolve_handler(self, route: RouteRecord) -> Definition | None:
        handler = route.handler.strip()
        if not handler:
            return None
        method_name = handler.split(".")[-1]
        candidates = [
            d
            for d in self.definitions
            if d.file_path == route.file_path and d.name == method_name and d.kind == "Function"
        ]
        if candidates:
            return candidates[0]
        candidates = [d for d in self.definitions if d.name == method_name and d.kind == "Function"]
        if len(candidates) == 1:
            return candidates[0]
        return None

    def resolve_test_target(self, test: TestRecord) -> Definition | None:
        if not test.target_symbol:
            return None
        candidates = [
            d
            for d in self.definitions
            if d.file_path == test.target_file and d.name == test.target_symbol and d.kind == "Function"
        ]
        if candidates:
            return candidates[0]
        return None


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Build a deterministic tree-sitter/Kuzu code graph.")
    parser.add_argument("--repo", type=Path, default=Path.cwd(), help="Repository root to index.")
    parser.add_argument("--db", type=Path, default=Path(".codegraph/streamclone.kuzu"), help="Kuzu database path.")
    parser.add_argument("--json", action="store_true", help="Emit machine-readable summary.")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    configure_tree_sitter_cache(args.repo.resolve())
    builder = CodeGraphBuilder(args.repo, args.db)
    summary = builder.build()
    if args.json:
        print(json.dumps(summary, indent=2, sort_keys=True))
    else:
        print(
            "Indexed {files} source files, {definitions} definitions, {imports} imports, "
            "{calls} resolved calls, {inheritance_edges} inheritance edges, "
            "{routes} routes, {tests} tests, {services} services into {db}".format(**summary)
        )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
