# Multi-repo workspace

Streamclone ships as two separate git repositories. This doc describes how they fit together on disk, who owns which docs, and the usual dev workflow.

**Repos:** [Aron-Chu/streamclone](https://github.com/Aron-Chu/streamclone) (backend, frontend, BFF, migrations, `pulse-core`) · [Aron-Chu/streamclone-pulse](https://github.com/Aron-Chu/streamclone-pulse) (Chrome MV3 extension).

---

## Two-repo layout

| Repo | Role | Typical checkout path |
|------|------|------------------------|
| **streamclone** | Go services, Caddy stack, web app, analytics BFF (`/v1/extension/*`, `/v1/pulse/*`), Postgres migrations, shared `packages/pulse-core` | `twitch-7tv-clone` (legacy folder name on disk) |
| **streamclone-pulse** | Chrome extension (content scripts, service worker, overlay UI) | `streamclone-pulse` (sibling folder) |

Product name is **Streamclone** everywhere in docs and commits — not `twitch-7tv-clone`.

Release install at `%USERPROFILE%\streamclone` (Setup.exe / ZIP) is **not** this source tree.

---

## Folder naming

| Name | Meaning |
|------|---------|
| `twitch-7tv-clone` | Legacy local folder name for the streamclone git checkout |
| `streamclone` | Product, GitHub repo, and workspace folder label |
| `streamclone-pulse` | Extension repo checkout (sibling) |
| `%USERPROFILE%\streamclone` | Release install — bug fixes must land in git, not only here |

---

## Multi-root workspace

Open both repos in one Cursor/VS Code window:

```text
streamclone-pulse-extension.code-workspace   # at streamclone repo root
```

Folders: **streamclone** (`.`) and **streamclone-pulse** (`../streamclone-pulse`). Clone both siblings under the same parent directory so relative paths resolve.

---

## Doc ownership matrix

| Topic | Canonical location | Streamclone stub / mirror |
|-------|-------------------|---------------------------|
| Extension requirements, design, tasks | `streamclone-pulse/docs/pulse-extension/` | `docs/pulse-extension/*.md` (redirect stubs) |
| Figma handoff + PNG exports | `streamclone-pulse/docs/pulse-extension/figma/` | `docs/pulse-extension/figma/` (optional local mirror for Codex) |
| Extension agent guide | `streamclone-pulse/AGENTS.md` | Cross-linked from root `AGENTS.md` |
| BFF routes, bookmarks, recap | `internal/analytics/extension_api.go` | — |
| Shared scoring / heat types | `packages/pulse-core/` | Imported by frontend and extension |
| Migrations (bookmarks, etc.) | `migrations/` in streamclone | — |
| Stack / compose / Caddy | `deploy/`, `docs/SERVICE_MAP.md` | — |
| **Laptopworker dev hub** | [`docs/laptopworker-dev.md`](laptopworker-dev.md) | Tailscale host `laptopworker`; remote `make laptopworker-status` |

Redirect stubs in `docs/pulse-extension/` point to the pulse repo. On GitHub, use repo URLs; on disk, open the sibling checkout or multi-root workspace.

---

## Laptopworker (optional tailnet dev host)

Home Linux box on Tailscale (`laptopworker`) runs the **core dev stack only** — UI at `http://laptopworker:8090`, no local scraper/corpus. Network-heavy scrape stays on **BearHost VPS**.

- Runbook: [`docs/laptopworker-dev.md`](laptopworker-dev.md) (stack ops, extension/portal backend modes, Remote SSH)
- From Windows repo root: `make laptopworker-status` or `scripts\laptopworker-remote.cmd status`
- After pushing laptopworker scripts to `master`: `make laptopworker-update`
- Extension laptop target: Options → Backend URL `http://laptopworker:8090` (see runbook)
- Portal laptop target: `VITE_BACKEND_URL=http://laptopworker:8090` in sibling `streamclone-pulse/streampulse-web`
- One-time on laptop: `sudo loginctl enable-linger aron` + `bash scripts/laptopworker-stack.sh ufw-tailnet` (boot without login; tailnet-only `:8090`)

---

## Separate commits

- Commit **streamclone** and **streamclone-pulse** independently — no monorepo.
- Follow [Conventional Commits](https://www.conventionalcommits.org/) (see `CONTRIBUTING.md`).
- Author: `Aron-Chu <aroncloudchu@gmail.com>` — no agent `Co-authored-by` trailers.
- Do not commit secrets (`.env`, tokens, machine-specific `.cursor/mcp.json`).

---

## MCP and codegraph scope

MCP servers (`streamclone-codegraph`, `streamclone-stack`, `streamclone-data`) index the **streamclone** checkout only.

- Setup: `make mcp-setup` in streamclone; copy `.cursor/mcp.recommended.json.example` → `.cursor/mcp.json`.
- Rebuild after symbol moves: `make codegraph`.
- Preflight: `bash scripts/mcp-preflight.sh` (WSL on Windows).
- Codex mirror: `make codex-setup` → `.codex/config.toml`, `.agents/skills/streamclone/` (synced from `.cursor/skills/streamclone/`). See [docs/CODEX.md](CODEX.md).

Extension work does not add MCP tools in the pulse repo; probe the backend via `:8090`.

---

## Dev workflow

1. **Stack** (streamclone): `make up` → Caddy at `http://localhost:8090`.
2. **StreamPulse web** (sibling `streamclone-pulse/streampulse-web`): `make pulse-local-up` then `make pulse-web-dev` → `http://localhost:5173` (beta key from `make pulse-local-enable`).
3. **BFF health**: `curl http://localhost:8090/v1/extension/health`
4. **Extension** (streamclone-pulse): `npm run build` → Load unpacked from `dist/` in Chrome.
5. **Tests**: `make check-quick` in streamclone; `npm test` / `npm run typecheck` in pulse.
6. **Spec edits**: requirements/design/tasks in **streamclone-pulse**; API/schema changes in **streamclone**.

Windows localhost issues → `.kiro/steering/windows-dev.md`.
