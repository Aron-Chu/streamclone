# Pulse extension spec (lives in streamclone-pulse)

The product spec for the Chrome extension is maintained in the **streamclone-pulse** repo. See [docs/workspace.md](../workspace.md) for the two-repo layout.

| Doc | GitHub | Local sibling checkout |
|-----|--------|------------------------|
| Requirements | [requirements.md](https://github.com/Aron-Chu/streamclone-pulse/blob/master/docs/pulse-extension/requirements.md) | `../../../streamclone-pulse/docs/pulse-extension/requirements.md` |
| Design | [design.md](https://github.com/Aron-Chu/streamclone-pulse/blob/master/docs/pulse-extension/design.md) | `../../../streamclone-pulse/docs/pulse-extension/design.md` |
| Tasks | [tasks.md](https://github.com/Aron-Chu/streamclone-pulse/blob/master/docs/pulse-extension/tasks.md) | `../../../streamclone-pulse/docs/pulse-extension/tasks.md` |
| Figma handoff + PNGs | [figma-handoff.md](https://github.com/Aron-Chu/streamclone-pulse/blob/master/docs/pulse-extension/figma-handoff.md) | `../../../streamclone-pulse/docs/pulse-extension/figma-handoff.md` |
| **Sidebar parity spec** | [parity-stream-pulse.md](https://github.com/Aron-Chu/streamclone-pulse/blob/master/docs/pulse-extension/parity-stream-pulse.md) | `../../../streamclone-pulse/docs/pulse-extension/parity-stream-pulse.md` |

In the multi-root workspace (`streamclone-pulse-extension.code-workspace`), open the **streamclone-pulse** folder → `docs/pulse-extension/`.

Backend implementation (BFF, bookmarks, recap, migrations) stays in this **streamclone** repo.

## Figma PNG mirror

Canonical PNG exports live in **streamclone-pulse** (`docs/pulse-extension/figma/`). This repo keeps a **local mirror** at [`figma/`](figma/) so streamclone-only Codex sessions can view designs without the sibling checkout.

- **Keep the mirror** — do not delete streamclone copies unless you rely solely on the multi-root workspace.
- **Sync one-way** from pulse → streamclone via `.\scripts\export-pulse-extension-figma.ps1` (or `.cjs`); edit handoff text in pulse, then re-export after Figma changes.
- Details: [`figma-handoff.md`](figma-handoff.md).
