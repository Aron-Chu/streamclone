# Multi-repo workspace

Streamclone ships across separate git repositories. This doc describes how they fit together on disk and the usual dev workflow.

**Repos:** [Aron-Chu/streamclone](https://github.com/Aron-Chu/streamclone) (watch replica) · [Aron-Chu/streamclone-pulse](https://github.com/Aron-Chu/streamclone-pulse) (extension + portal) · private **streampulse-backend** (BFF, ingest, packages) · private **streampulse-ops** (deploy) · ReplayForge (optional Clip Studio sibling).

See [streampulse-product-boundary.md](streampulse-product-boundary.md) for ownership.

---

## Local repo layout

| Repo | Role | Typical checkout path |
|------|------|------------------------|
| **streamclone** | Core Go services, Caddy stack, watch frontend, desktop install CI | `twitch-7tv-clone` (legacy folder name) |
| **streamclone-pulse** | Chrome extension, streampulse-web portal, product docs | `streamclone-pulse` (sibling) |
| **streampulse-backend** | Analytics BFF, ingest, `pulse-core` packages *(private)* | operator checkout |
| **streampulse-ops** | Production deploy, secrets, evidence *(private)* | operator checkout |
| **replayforge** | Clip Studio / auto clipper *(optional sibling)* | `replayforge` |

Product name is **Streamclone** everywhere in docs and commits — not `twitch-7tv-clone`.

Release install at `%USERPROFILE%\streamclone` (Setup.exe / ZIP) is **not** this source tree.

---

## Multi-root workspaces

| Workspace file | Folders | Use for |
|----------------|---------|---------|
| `streamclone-backend.code-workspace` | streamclone only | Watch, playback, emotes, install |
| `streamclone-pulse-extension.code-workspace` | streamclone + streamclone-pulse | Extension + local core stack |
| `streampulse-portal.code-workspace` | streamclone-pulse | Portal / streampulse-web |
| `auto-clipper-integration.code-workspace` | streamclone + replayforge | Clipper integration only |
| `streamclone-full.code-workspace` | all three public repos | Cross-repo audits |

**Private `streampulse-ops` and `streampulse-backend` are never part of public multi-root workspaces.**

Agents: pick the **smallest** workspace that owns the task.

---

## Doc ownership matrix

| Topic | Canonical location |
|-------|-------------------|
| Extension requirements, design, tasks | `streamclone-pulse/docs/pulse-extension/` |
| Portal layout and guardrails | `streamclone-pulse/docs/website-portal/` |
| Watch / playback / emotes / install | **streamclone** (this repo) |
| BFF, ingest, hub API, packages | **streampulse-backend** (private) |
| Deploy, SSH, promotion evidence | **streampulse-ops** (private) |
| Product boundary (public stub) | [streampulse-product-boundary.md](streampulse-product-boundary.md) |

---

## MCP and codegraph scope

MCP servers (`streamclone-codegraph`, `streamclone-stack`, `streamclone-data`) index the **streamclone** checkout.

- Setup: `make mcp-setup` in streamclone; copy `.cursor/mcp.recommended.json.example` → `.cursor/mcp.json`.
- Rebuild after symbol moves: `make codegraph`.
- Codex: `make codex-setup` → `.codex/config.toml`, `.agents/skills/streamclone/`. See [docs/CODEX.md](CODEX.md).

Extension and portal work use **streamclone-pulse** docs; backend MCP lives in **streampulse-backend**.

---

## Dev workflow (core)

1. **Stack** (streamclone): `make up` → Caddy at `http://localhost:8090`.
2. **Smoke**: `make smoke`, `playback_probe` MCP.
3. **Frontend**: `cd frontend && npm test`.
4. **Extension** (streamclone-pulse): `npm run build` → reload in Chrome.
5. **Portal** (streamclone-pulse): hosted API by default — see sibling local-dev-runbook.

Windows localhost issues → `.kiro/steering/windows-dev.md`.

---

## Separate commits

- Commit each repo independently — no monorepo.
- Conventional Commits (see `CONTRIBUTING.md`).
- Author: `Aron-Chu <aroncloudchu@gmail.com>` — no agent co-author trailers.
- Do not commit secrets.
