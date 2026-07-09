# CI / workflow inventory (boundary split)

Audit of public Streamclone automation. Each row: **keep public**, **move to backend**, **delete**, or **trim** (Step 4 PR batch).

---

## GitHub Actions (`.github/workflows/`)

| Workflow | Action |
|----------|--------|
| `ci.yml` | **Trim** — drop analytics/storygraph matrix rows |
| `release-images.yml` | **Split** — core images + Setup.exe stay; analytics/migrate/workers → backend workflow |
| `analytics-gold-segments-integration.yml` | **Move** to streampulse-backend |
| `smoke-scraper.yml` | **Delete** |
| `codeql.yml` | **Keep** |
| `azure-archive-terraform.yml` | **Move** to backend or streampulse-ops |

---

## Dependabot (`.github/dependabot.yml`)

| Entry | Action |
|-------|--------|
| `gomod` `/` | **Keep** (core modules after split) |
| `npm` `/frontend` | **Keep**; remove `packages/pulse-*` paths from public |
| `github-actions` | **Keep** |
| New backend repo | Add dependabot for Go + packages |

---

## Makefile phony targets (sample — full grep required before Step 4)

**Delete or move:** `test-analytics`, `test-analytics-gold-segments`, `test-pulse-emote`, `smoke-pulse-emote*`, `rebuild-analytics-emote`, `restart-analytics`, `packages-pulse-core-test`, `up-scraper`, `up-full`, `codegraph-pulse`, `bearhost-*`, `grafana-*`, `azure-scraper-*`, `hybrid-preflight`, `scraper-*`

**Keep:** `up`, `stop`, `smoke`, `test`, `test-video`, `test-emote`, `test-metadata`, `frontend-test`, `compose-config-check`, `check-quick`, `check`, `package-release`, `codegraph`, `mcp-setup`, `codex-setup`

---

## Other automation

| Path | Action |
|------|--------|
| `.pre-commit-config.yaml` + `scripts/pre-commit-public-ops-guard.sh` | **Keep** + extend product boundary guard at Step 7 |
| `scripts/pre-commit-go-test.sh` | **Trim** analytics packages |
| `analytics-overlap-check` | **Move** to streamclone-pulse only |
| Workspace files | **Trim** pulse-backend roots from public workspaces |
| `scripts/package-release.sh`, `smoke-release-bundle.*` | **Keep** — verify core-only bundle contents |
| `scripts/build-windows-installer.ps1` | **Keep** |
| `hub-health-monitor.yml` | **Missing** — ops-only or add core-only public probe |

---

## Sign-off

Step 4 PR must close all **trim/delete** rows for the public repo. Backend repo owns new CI for `cmd/analytics` and `packages/pulse-*`.
