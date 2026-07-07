# Multi-repo workspace

Streamclone ships across separate git repositories. This doc describes how they fit together on disk, who owns which docs, and the usual dev workflow.

**Repos:** [Aron-Chu/streamclone](https://github.com/Aron-Chu/streamclone) (backend, frontend, BFF, migrations, `pulse-core`) · [Aron-Chu/streamclone-pulse](https://github.com/Aron-Chu/streamclone-pulse) (Chrome MV3 extension) · ReplayForge (standalone Clip Studio / auto clipper sibling checkout).

---

## Local repo layout

| Repo | Role | Typical checkout path |
|------|------|------------------------|
| **streamclone** | Go services, Caddy stack, web app, analytics BFF (`/v1/extension/*`, `/v1/pulse/*`), Postgres migrations, shared `packages/pulse-core` | `twitch-7tv-clone` (legacy folder name on disk) |
| **streamclone-pulse** | Chrome extension (content scripts, service worker, overlay UI) | `streamclone-pulse` (sibling folder) |
| **replayforge** | Standalone ReplayForge API + Clip Studio UI for auto clipper, render/edit/export, captions, templates, and clip artifacts | `replayforge` (sibling folder, only needed for Clip Studio / auto clipper work) |

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

## Multi-root workspaces

For StreamPulse extension and portal work, open the two-repo workspace:

```text
streamclone-pulse-extension.code-workspace   # at streamclone repo root
```

Folders: **streamclone** (`.`) and **streamclone-pulse** (`../streamclone-pulse`). This is the default StreamPulse workspace because the extension and portal live in `streamclone-pulse`, while Streamclone provides the current backend contracts, shared package, migrations, and local Caddy stack.

For Clip Studio / auto clipper work, open the optional ecosystem workspace instead:

```text
streamclone-full.code-workspace              # streamclone + streamclone-pulse + replayforge
```

Folders: **streamclone** (`.`), **streamclone-pulse** (`../streamclone-pulse`), and **replayforge** (`../replayforge`). This workspace is for cross-repo integration only. It does not make ReplayForge part of the default StreamPulse product surface, and it does not move Clip Studio or rendering work into Streamclone.

Use the ecosystem workspace only when a task crosses those boundaries: Streamclone Analytics moment export, ReplayForge trigger/API contracts, `/studio` redirects, or auto clipper runbooks. For normal StreamPulse portal, extension, or public website work, stay in the two-repo workspace and do not open or edit ReplayForge. See [agents-streamclone-and-replayforge.md](agents-streamclone-and-replayforge.md).

**Private `streampulse-ops` is never part of public workspaces** — operator checkout only (secrets, production compose). See [Workspace / context boundaries](#workspace--context-boundaries) below.

---

## Workspace / context boundaries

Default to the **smallest workspace** that owns the task. Full matrix:

| Workspace | Folders | Use for | Keep out |
|---|---|---|---|
| **StreamPulse Extension** | `streamclone-pulse` | MV3 extension, content scripts, service worker | ReplayForge, private ops |
| **StreamPulse Web / Portal** | `streamclone-pulse` (`streampulse-web/` + `docs/website-portal/`) | Public site, portal, analytics hub | ReplayForge, streamclone unless API contract |
| **Streamclone Backend** | `streamclone` (this repo) | Go APIs, migrations, BFF/workers, source image builds | ReplayForge UI |
| **ReplayForge** | `replayforge` | Clip Studio, render pipeline, artifacts | StreamPulse portal/extension |
| **Auto Clipper Integration** | `streamclone` + `replayforge` | Moment export, ReplayForge trigger contract | StreamPulse web unless UI contract |
| **Full Ecosystem** | `streamclone` + `streamclone-pulse` + `replayforge` | Cross-repo audits, integration planning | Daily feature work; **never** private ops |
| **Private Ops** | `streampulse-ops` only (separate checkout) | Production compose, promotion cutover | All public workspaces |

Focused workspace files at repo root (optional):

| File | Folders |
|------|---------|
| [`streamclone-pulse-extension.code-workspace`](../streamclone-pulse-extension.code-workspace) | streamclone + streamclone-pulse (default product) |
| [`streampulse-extension.code-workspace`](../streampulse-extension.code-workspace) | streamclone-pulse only |
| [`streampulse-portal.code-workspace`](../streampulse-portal.code-workspace) | streamclone-pulse (portal context) |
| [`streamclone-backend.code-workspace`](../streamclone-backend.code-workspace) | streamclone only |
| [`replayforge.code-workspace`](../replayforge.code-workspace) | replayforge only |
| [`auto-clipper-integration.code-workspace`](../auto-clipper-integration.code-workspace) | streamclone + replayforge |
| [`streamclone-full.code-workspace`](../streamclone-full.code-workspace) | all three public repos |

Agents: pick smallest workspace before broad reads. Hosted production image migration: [`production-promotion-contract.md`](production-promotion-contract.md), sibling [image exit audit](../streamclone-pulse/docs/pulse-extension/evidence/streamclone-image-exit-audit-2026-07.md).

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
| **Production promotion (hosted)** | [`docs/production-promotion-contract.md`](production-promotion-contract.md) | Sibling image exit audit in `streamclone-pulse` |
| **Laptopworker dev hub** | [`docs/laptopworker-dev.md`](laptopworker-dev.md) | Tailscale host `laptopworker`; remote `make laptopworker-status` |

Redirect stubs in `docs/pulse-extension/` point to the pulse repo. On GitHub, use repo URLs; on disk, open the sibling checkout or multi-root workspace.

---

## Laptopworker (optional tailnet dev host)

Home Linux box on Tailscale (`laptopworker`) runs the **core dev stack only** — UI at `http://laptopworker:8090`, no local scraper/corpus. Network-heavy scrape stays on **legacy-rollback-host**.

- Topology (what runs where): [`.kiro/steering/laptopworker-hosting.md`](../.kiro/steering/laptopworker-hosting.md)
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

Extension and portal work do not add MCP tools in the pulse repo; probe the backend via `:8090` only when local backend truth is needed.

---

## Dev workflow

1. **Stack** (streamclone): `make up` → Caddy at `http://localhost:8090` (Go BFF / extension smoke only).
2. **StreamPulse web** (sibling `streamclone-pulse/streampulse-web`): `npm run dev` → `http://127.0.0.1:5173/analytics` against **hosted** API by default. Opt-in local stack: `npm run dev:local` — see [streamclone-pulse local-dev-runbook](../streamclone-pulse/docs/website-portal/local-dev-runbook.md). (`make pulse-web-dev` wraps the same.)
3. **BFF health**: `curl http://localhost:8090/v1/extension/health`
4. **Extension** (streamclone-pulse): `npm run build` → Load unpacked from `dist/` in Chrome.
5. **Tests**: `make check-quick` in streamclone; `npm test` / `npm run typecheck` in pulse.
6. **Spec edits**: requirements/design/tasks in **streamclone-pulse**; API/schema changes in **streamclone**.

Windows localhost issues → `.kiro/steering/windows-dev.md`.
