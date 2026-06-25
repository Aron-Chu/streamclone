#!/usr/bin/env python3
"""Block git commit when staged files likely contain secrets."""

from __future__ import annotations

import json
import re
import subprocess
import sys

PATTERNS = [
    re.compile(r"PULSE_BETA_KEYS\s*=\s*[^\s#,]+", re.I),
    re.compile(r"(?i)(oauth|client_secret|private_key)\s*=\s*['\"]?[A-Za-z0-9_\-/+=]{16,}"),
]
SKIP_PREFIXES = (".env.example", "deploy/env/", "docs/", ".cursor/hooks/")


def staged_files() -> list[str]:
    proc = subprocess.run(["git", "diff", "--cached", "--name-only"], capture_output=True, text=True, check=False)
    return [p.strip().replace("\\", "/") for p in proc.stdout.splitlines() if p.strip()]


def staged_diff() -> str:
    proc = subprocess.run(["git", "diff", "--cached"], capture_output=True, text=True, check=False)
    return proc.stdout


def main() -> int:
    try:
        payload = json.load(sys.stdin)
    except json.JSONDecodeError:
        return 0

    command = str(payload.get("command") or "")
    if "git commit" not in command:
        return 0

    diff = staged_diff()
    hits: list[str] = []
    for pattern in PATTERNS:
        for match in pattern.finditer(diff):
            snippet = match.group(0)[:80]
            if any(x in snippet.lower() for x in ("example", "replace-with", "set-on-vps", "your-", "changeme")):
                continue
            hits.append(snippet)

    for path in staged_files():
        if path.startswith(SKIP_PREFIXES):
            continue
        if path.endswith((".env", ".env.local", "oauth-bundle.env", "pulse-beta.env")):
            hits.append(f"staged secret file: {path}")

    if not hits:
        return 0

    msg = "Possible secret in staged changes: " + "; ".join(hits[:5])
    print(
        json.dumps(
            {
                "permission": "deny",
                "user_message": msg,
                "agent_message": "Remove secrets from staged files before committing.",
            }
        )
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
