"""Tree-sitter parsing helpers and query definitions."""

from __future__ import annotations

import re
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable

from tree_sitter import Node, Parser, Query, QueryCursor
from tree_sitter_language_pack import get_language

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
QUERY_BY_LANGUAGE["python"] = {
    "definitions": r"""
      (function_definition name: (identifier) @name) @function
      (class_definition name: (identifier) @name) @class
    """,
}


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
            return candidate.resolve().relative_to(repo).as_posix()
    return start.resolve().relative_to(repo).as_posix()


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
        if parent.type in {"class_declaration", "class_definition"}:
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
