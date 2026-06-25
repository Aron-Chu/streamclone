"""Domain extractors for routes, tests, services, and frontend API calls."""

from __future__ import annotations

import json
import re
from dataclasses import dataclass
from pathlib import Path


@dataclass(frozen=True)
class RouteRecord:
    id: str
    method: str
    path: str
    handler: str
    file_path: str
    line: int
    source: str = "go-chi"


@dataclass(frozen=True)
class TestRecord:
    id: str
    name: str
    file_path: str
    target_file: str
    target_symbol: str
    line: int


@dataclass(frozen=True)
class ServiceRecord:
    id: str
    name: str
    description: str
    keywords: str
    path_patterns: tuple[str, ...]
    routes: tuple[str, ...]


@dataclass(frozen=True)
class ClientCallRecord:
    file_path: str
    route_path: str
    call_expr: str
    line: int


GO_METHOD_RE = re.compile(
    r'r\.(Get|Post|Put|Delete|Patch|Head|Options)\s*\(\s*"([^"]+)"\s*,\s*([^,\)]+)',
)
GO_ROUTE_MOUNT_RE = re.compile(r'r\.(?:Route|Mount)\s*\(\s*"([^"]+)"')
FETCH_PATH_RE = re.compile(
    r"""fetch\s*\(\s*(?:`([^`]+)`|'([^']+)'|"([^"]+)"|\$\{[^}]+\}([^`'"?]+))""",
    re.IGNORECASE,
)
V1_PATH_RE = re.compile(r"/v1/[A-Za-z0-9_./{}${}-]+")


def join_route_path(prefixes: list[str], subpath: str) -> str:
    parts: list[str] = []
    for prefix in prefixes:
        parts.append(prefix.strip("/"))
    parts.append(subpath.strip("/"))
    joined = "/".join(part for part in parts if part)
    return "/" + joined if joined else "/"


def extract_go_routes(content: str, rel_path: str) -> list[RouteRecord]:
    lines = content.splitlines()
    prefix_stack: list[str] = []
    routes: list[RouteRecord] = []

    for line_no, line in enumerate(lines, 1):
        stripped = line.strip()
        mount_match = GO_ROUTE_MOUNT_RE.search(line)
        if mount_match:
            prefix_stack.append(mount_match.group(1))

        method_match = GO_METHOD_RE.search(line)
        if method_match:
            method = method_match.group(1).upper()
            subpath = method_match.group(2)
            handler = method_match.group(3).strip().rstrip(")")
            full_path = join_route_path(prefix_stack, subpath)
            route_id = f"route:{rel_path}:{method}:{full_path}:{line_no}"
            routes.append(
                RouteRecord(
                    id=route_id,
                    method=method,
                    path=full_path,
                    handler=handler,
                    file_path=rel_path,
                    line=line_no,
                )
            )

        if stripped in {"})", "}"} and prefix_stack:
            prefix_stack.pop()

    return routes


def extract_frontend_client_calls(content: str, rel_path: str) -> list[ClientCallRecord]:
    calls: list[ClientCallRecord] = []
    for line_no, line in enumerate(content.splitlines(), 1):
        for match in FETCH_PATH_RE.finditer(line):
            raw = next((g for g in match.groups() if g), "")
            if not raw:
                continue
            for path_match in V1_PATH_RE.finditer(raw):
                route_path = path_match.group(0).split("?", 1)[0]
                calls.append(
                    ClientCallRecord(
                        file_path=rel_path,
                        route_path=route_path,
                        call_expr=match.group(0),
                        line=line_no,
                    )
                )
    return calls


def infer_test_target(rel_path: str) -> str:
    path = Path(rel_path)
    name = path.name
    if name.endswith("_test.go"):
        return str(path.with_name(name[: -len("_test.go")] + ".go")).replace("\\", "/")
    if name.endswith(".test.ts"):
        return str(path.with_name(name[: -len(".test.ts")] + ".ts")).replace("\\", "/")
    if name.endswith(".test.tsx"):
        return str(path.with_name(name[: -len(".test.tsx")] + ".tsx")).replace("\\", "/")
    if name.endswith(".spec.ts"):
        return str(path.with_name(name[: -len(".spec.ts")] + ".ts")).replace("\\", "/")
    if name.endswith(".spec.tsx"):
        return str(path.with_name(name[: -len(".spec.tsx")] + ".tsx")).replace("\\", "/")
    return ""


def extract_tests(content: str, rel_path: str, language: str) -> list[TestRecord]:
    target_file = infer_test_target(rel_path)
    if not target_file:
        return []

    tests: list[TestRecord] = []
    if language == "go":
        pattern = re.compile(r"^func\s+(Test[A-Za-z0-9_]+)\s*\(", re.MULTILINE)
        for match in pattern.finditer(content):
            name = match.group(1)
            line = content[: match.start()].count("\n") + 1
            symbol = name.removeprefix("Test")
            test_id = f"test:{rel_path}:{name}:{line}"
            tests.append(
                TestRecord(
                    id=test_id,
                    name=name,
                    file_path=rel_path,
                    target_file=target_file,
                    target_symbol=symbol,
                    line=line,
                )
            )
    elif language in {"tsx", "typescript"}:
        pattern = re.compile(r"(?:it|test)\s*\(\s*['\"]([^'\"]+)['\"]", re.MULTILINE)
        for match in pattern.finditer(content):
            name = match.group(1)
            line = content[: match.start()].count("\n") + 1
            test_id = f"test:{rel_path}:{line}:{name[:40]}"
            tests.append(
                TestRecord(
                    id=test_id,
                    name=name,
                    file_path=rel_path,
                    target_file=target_file,
                    target_symbol="",
                    line=line,
                )
            )
    return tests


def load_services(config_path: Path) -> list[ServiceRecord]:
    if not config_path.exists():
        return []
    payload = json.loads(config_path.read_text(encoding="utf-8"))
    services: list[ServiceRecord] = []
    for item in payload.get("subsystems", []):
        services.append(
            ServiceRecord(
                id=str(item["id"]),
                name=str(item["name"]),
                description=str(item.get("description", item["name"])),
                keywords=",".join(item.get("keywords", [])),
                path_patterns=tuple(item.get("path_patterns", [])),
                routes=tuple(item.get("routes", [])),
            )
        )
    return services


def file_belongs_to_service(file_path: str, service: ServiceRecord) -> bool:
    for pattern in service.path_patterns:
        normalized = pattern.rstrip("/")
        if file_path == pattern or file_path.startswith(normalized + "/"):
            return True
        if pattern.endswith(".go") or pattern.endswith(".ts") or pattern.endswith(".tsx"):
            if file_path == pattern:
                return True
    return False


def route_belongs_to_service(route_path: str, service: ServiceRecord) -> bool:
    for prefix in service.routes:
        if route_path == prefix or route_path.startswith(prefix.rstrip("/") + "/"):
            return True
    return False
