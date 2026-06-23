---
description: Agent readiness — AGENTS, SERVICE_MAP, TESTING, MCP, ENVIRONMENT docs, Makefile targets, make check-quick, make mcp-setup. No runtime/product edits.
---

# Agent Readiness

Docs and tooling only — **no product/runtime behavior changes** unless the task explicitly requires them.

## Read first (in order)

1. [`AGENTS.md`](../../../../AGENTS.md) — golden rules, task router, safe commands
2. [`docs/SERVICE_MAP.md`](../../../../docs/SERVICE_MAP.md) — service boundaries and ports
3. [`docs/TESTING.md`](../../../../docs/TESTING.md) — domain test map
4. [`docs/MCP.md`](../../../../docs/MCP.md) — MCP tiers, avoid list, setup
5. [`docs/ENVIRONMENT.md`](../../../../docs/ENVIRONMENT.md) — compose profiles and env overlays

## Makefile gates

```sh
make check-quick     # PR gate: test, vet, frontend-test, compose-config-check
make check           # full gate before release (security-scan, build, audit, clipper, …)
make mcp-setup       # codegraph-install + codegraph + mcp-preflight
make compose-config-check
make validate-env PROFILE=core
```

## MCP bootstrap

```sh
make mcp-setup
# Cursor: copy .cursor/mcp.recommended.json.example → .cursor/mcp.json (gitignored)
# Codex: make codex-setup — see docs/CODEX.md
bash scripts/mcp-preflight.sh
```

Essential MCP loop: codegraph → stack → data → Playwright. See `docs/MCP.md` priority matrix.

## Optional pre-commit (light)

```sh
make install-hooks
pre-commit run check-quick-light --all-files   # opt-in manual hook
```

## When editing agent docs

- Update steering/docs after scraper, analytics, Pulse Wire, install, OAuth, or large frontend changes
- Run `make codegraph` when symbols move
- Never commit `.cursor/mcp.json`, `.env`, or tokens
