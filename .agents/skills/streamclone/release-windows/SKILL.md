---
description: Windows release install — %USERPROFILE%\\streamclone vs git checkout, Setup.exe/ZIP, install-desktop, release bundle. Docker fixes ≠ release fixes.
---

# Release / Windows Install

Read `AGENTS.md`, [`docs/install-desktop.md`](../../../../docs/install-desktop.md), and [`docs/repo-maintenance.md`](../../../../docs/repo-maintenance.md).

## Two locations (do not confuse)

| Path | What it is |
|------|------------|
| **Git checkout** (e.g. `twitch-7tv-clone` on disk) | Source repo — commit and push bug fixes here (`Aron-Chu/streamclone`) |
| **`%USERPROFILE%\streamclone`** | Release install from Setup.exe / ZIP — **not** this git repo |

Copying scripts only into the install folder does **not** ship fixes to other users. Commit fixes in the git repo and release via tag → GHCR + Setup.exe.

## Release pipeline

- Tag `v*` pushes trigger [`.github/workflows/release-images.yml`](../../../../.github/workflows/release-images.yml) — `release-gate` (`make check`) before image publish
- `VERSION` file must match git tag before tagging
- Bundle: `scripts/package-release.sh`, `scripts/validate-release-env.sh`
- OAuth: `oauth-bundle.env` — never commit; see install-desktop docs

## Install / desktop / bootstrap

- Launcher scripts under `scripts/` — setup-control, Caddy tunnel, bootstrap
- After **install / desktop / bootstrap / uninstall** bug fixes, append one row to the *Install bug fix log* table in `docs/repo-maintenance.md`

## Warning: Docker fixes ≠ release fixes

- `make up` / compose changes validate in dev Docker stack
- Windows desktop users run release images + bundled scripts — verify install-desktop path and release bundle env synthesis
- Caddy tunnel / localhost issues on Windows → `.kiro/steering/windows-dev.md`

## Checks

```sh
make compose-config-check
bash scripts/validate-release-env.sh   # when touching release env synthesis
```
