# Operator secrets layout

Streamclone and StreamPulse use **out-of-repo secret files** — never commit real tokens, webhooks, or connection strings. Gitleaks runs in CI on both repos; `.env` is gitignored; `.env.example` holds placeholders only.

## Two stores

| Store | Path | Used for |
|-------|------|----------|
| **Local operator** | `%USERPROFILE%\.streamclone\` (Windows) · `~/.streamclone/` (WSL/Linux/macOS) | Dev machine, WSL SSH to BearHost, backup upload from laptop |
| **BearHost production** | `/etc/streamclone/secrets/` | VPS observability, Azure backup upload, Alertmanager webhook |

Repo `.env` / `.env.local` are **generated or merged** from templates (`make setup`, profile env files). Machine-specific files under `.streamclone` are **not** merged into compose automatically unless a script or env var points at them.

Initialize the local layout (creates dirs + README, no secrets):

```powershell
# Windows
powershell -File scripts/operator-secrets-init.ps1
```

```bash
# WSL / Linux / macOS
bash scripts/operator-secrets-init.sh
```

## Local manifest (`~/.streamclone/`)

| File | Purpose | Install / rotate |
|------|---------|------------------|
| `alertmanager-webhook-url` | Discord (or compatible) webhook for Alertmanager | One line, no quotes; `scripts/bearhost-alertmanager-secret-install.sh` |
| `azure-archive-connection-string` | Azure Blob connection string for archive + pg backup upload | Terraform output or portal; see `docs/azure-archive-setup.md` |
| `archive.env.local.snippet` | Optional env snippet to merge into `.env.local` | Terraform / manual |
| `README` | Index of files (no secret values) | Created by init script |

Optional (machine-specific, not required for core stack):

| Path | Purpose |
|------|---------|
| `chrome-cdp-profile/` | Scraper / browser automation profile |
| `grafana-tunnel-watch.log` | SSH tunnel watchdog log |

**Permissions:** `chmod 600` on secret files in WSL; BearHost production secrets `644` when bind-mounted into Docker (directory stays `700` / owner `streamclone`).

## BearHost manifest (`/etc/streamclone/secrets/`)

| File | Purpose |
|------|---------|
| `alertmanager-webhook-url` | Alertmanager `webhook_url_file` mount |
| `azure-archive-connection-string` | Nightly pg dump upload + archive workers |

Install from laptop (URL not printed):

```bash
bash scripts/bearhost-alertmanager-secret-install.sh
```

## What never goes in git

- Real Discord webhook URLs (use placeholder in `.env.example`)
- Azure connection strings, Twitch OAuth tokens, `AUTH_COOKIE_SECRET`, API tokens
- `.env`, `.env.local`, `oauth-bundle.env`

**Verified:** Discord Alertmanager webhook was never committed to either repo (`git log -S "discord.com/api/webhooks"` empty). Rotate any webhook that was pasted into chat or a local file.

## Scan before push

```bash
# Streamclone
make security-scan

# streamclone-pulse
bash scripts/security-scan.sh
# or: pre-commit run gitleaks --all-files
```

## GitHub

- Enable **secret scanning** and **push protection** on both repos (private repo: Advanced Security if available).
- Streamclone allowlists `.env.example` in `.gitleaks.toml` for public Twitch client ID templates — **still do not put real webhooks there** (streamclone-pulse does **not** allowlist `.env.example`).

## Related docs

- [`docs/security.md`](security.md) — stack security checklist
- [`docs/azure-archive-setup.md`](azure-archive-setup.md) — Azure connection string
- [`docs/pulse-extension/top-500-ops-runbook.md`](../streamclone-pulse/docs/pulse-extension/top-500-ops-runbook.md) — Alertmanager closeout (sibling repo)
