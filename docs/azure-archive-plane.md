# Azure hybrid archive plane

Tailscale-only Azure VM for always-on Streamclone capture: **Mode A** (remote scraper smoke) → **Mode B** (self-contained archive plane with Azure-local Postgres, workers, and nightly `pg_dump` to Blob).

Related: [azure-archive-setup.md](azure-archive-setup.md) (Blob Terraform) · [scraping-archive/requirements.md](scraping-archive/requirements.md) · [ENVIRONMENT.md](ENVIRONMENT.md)

---

## Decision tree

| Mode | Azure runs | Local runs | When |
|------|------------|------------|------|
| **A — Remote scraper** | Camoufox scraper + host Tailscale | Postgres, analytics workers, UI | First smoke (lowest risk) |
| **B — Archive plane** | Scraper + Postgres + Redis + metadata + `analytics-workers` | UI/dev; **workers OFF** via `profile-local-hybrid.env` | 24h top-200 bronze proof |

**Avoid long-term:** Azure workers → local Postgres over Tailscale (PC sleep, Docker restarts, tailnet drops).

---

## Architecture (Mode B)

```mermaid
flowchart TB
  subgraph Local [Local Dev Streamclone]
    UI[Frontend / Caddy :8090]
    PGL[(Local Postgres dev)]
  end

  subgraph AzureVM [Azure Archive Plane]
    SCRAPER[scraper :8000 tailnet]
    WORKERS[analytics-workers]
    META[metadata]
    PGA[(Postgres volume)]
    REDIS_A[(Redis)]
  end

  subgraph Blob [Azure Blob]
    ARCHIVE[streamclone-archive]
  end

  UI -->|SCRAPER_API_URL| SCRAPER
  WORKERS --> PGA
  WORKERS --> REDIS_A
  WORKERS --> META
  WORKERS --> SCRAPER
  WORKERS --> ARCHIVE
  PGA -->|nightly pg_dump| ARCHIVE
```

Durable truth: **Azure Blob**. Azure Postgres is the hot working set on the VM (named volume `azure-pg-data`).

---

## Tailscale hostnames

| Machine | Pattern | Notes |
|---------|---------|-------|
| **Azure VM** | Host-level Tailscale (`azure-streamclone`) | Scraper binds `127.0.0.1:8000`; UFW allows `:8000` on `tailscale0` only |
| **Local PC** | Host Tailscale (`local-streamclone`) or tailnet IP in `SCRAPER_API_URL` | Windows Docker may not resolve MagicDNS — use tailnet IP or document sidecar |

Local analytics calls: `http://azure-streamclone:8000/v2/scrape`

---

## Env matrix

| Profile | File | Purpose |
|---------|------|---------|
| Mode A VM | `deploy/env/profile-azure-scraper.env` | Scraper caps, proxy vars on VM only |
| Mode B VM | `profile-archive.env` + `profile-azure-workers.env` | Workers ON; `DATABASE_URL` → **Azure** `postgres` service |
| Local hybrid | `deploy/env/profile-local-hybrid.env` | Workers OFF; remote `SCRAPER_API_URL`; emote roster preload ON |

Mode B **must not** set `DATABASE_URL` to `local-streamclone` — workers use compose-internal `postgres://app:app@postgres:5432/streamclone`.

---

## Bootstrap order

### Phase 0 — Prerequisites

1. Azure Blob — `bash scripts/azure-archive-fresh-start.sh` → `~/.streamclone/azure-archive-connection-string`
2. Tailscale — same tailnet, MagicDNS hostnames above
3. Flame creds — VM `.env.local` only; see `deploy/env/proxy-flame.env.example`
4. Compute VM — `deploy/terraform/azure/compute/` then `scripts/azure-vm-bootstrap.sh`

### Stage 1 — Mode A

**Azure VM:**

```bash
docker compose --env-file .env \
  --env-file deploy/env/profile-azure-scraper.env \
  -f deploy/docker-compose.azure-scraper.yml \
  -f deploy/docker-compose.release.yml up -d
```

**Local:** merge `profile-local-hybrid.env`, keep local scraper profile **off**, `make up`.

Validate: `make hybrid-preflight`, `pwsh scripts/scraper-preflight.ps1 -ScraperURL http://azure-streamclone:8000`

### Stage 2 — Mode B

**Azure VM** (add full plane):

```bash
docker compose --env-file .env \
  --env-file deploy/env/profile-archive.env \
  --env-file deploy/env/profile-azure-workers.env \
  -f deploy/docker-compose.azure-archive-plane.yml \
  -f deploy/docker-compose.release.yml up -d
```

Mount secrets: copy connection string to `~/.streamclone/` on VM; set `STREAMCLONE_SECRETS_DIR=~/.streamclone` if not using default bind.

**Local:** workers remain disabled in `profile-local-hybrid.env`.

**Critical test:** stop local Streamclone 30+ minutes — Azure Tier-0/Bronze must continue.

### Proxy smoke

```powershell
pwsh scripts/azure-hybrid-smoke.ps1
```

- TT detail `useProxy=false` → scraper logs show proxy **disabled**
- Social `useProxy=true` → scraper logs show proxy **enabled**

---

## VM sizing

| SKU | Spec | Use |
|-----|------|-----|
| `Standard_B2s` | 2 vCPU / 4 GB | Start here (Mode A) |
| `Standard_B2ms` | 2 vCPU / 8 GB | Mode B when memory >80%, OOM, Camoufox crash loops |

Resize in `deploy/terraform/azure/compute/terraform.tfvars` → `terraform apply`.

Worker caps on B2s (in `profile-azure-workers.env`): `SCRAPER_MAX_CONCURRENT=1`, `ANALYTICS_VOD_GQL_CONCURRENCY=1`, `BRONZE_TT_SUMMARY_CONCURRENCY=2`.

---

## Cost checklist

| Item | Budget hint |
|------|-------------|
| VM + disk | Terraform budget $25/mo on `rg-streamclone-prod` (50/75/90/100% alerts) |
| Blob archive | Separate $5/mo budget on archive RG |
| Flame proxy | Usage-based — monitor social scrape volume |

---

## Do not expose publicly

- `/v2/scrape`, Postgres `:5432`, Redis `:6379` on the public Internet
- NSG allows **SSH only** from `allowed_ssh_cidr`
- Scraper reachable on tailnet via host Tailscale + `127.0.0.1:8000` publish

---

## Validation commands

```sh
make azure-scraper-config-check
make azure-archive-plane-config-check
make hybrid-preflight          # needs tailnet + remote scraper up
terraform -chdir=deploy/terraform/azure/compute validate
go test ./internal/emote/preload/...
```

Bronze acceptance (Mode B, remote VM):

```powershell
# Quick smoke after Mode B up (Tailscale default host)
pwsh scripts/azure-archive-acceptance.ps1 -Stage smoke
pwsh scripts/azure-archive-acceptance.ps1 -Stage smoke -SshKey ~/.ssh/azure-streamclone -Duration 5m -PollInterval 1m

# 6h smoke — merge profile-bronze-smoke.env on VM before long run
pwsh scripts/azure-archive-acceptance.ps1 -Stage smoke -Duration 6h

# 24h acceptance — merge profile-bronze-acceptance.env on VM
pwsh scripts/azure-archive-acceptance.ps1 -Stage acceptance -Duration 24h
```

**Parameters:** `-Stage smoke|acceptance`, `-SshHost` (default `azure-streamclone`), `-SshUser` (default `streamclone`), `-SshKey`, `-Duration`, `-PollInterval`, `-RemoteRepoDir` (default `~/streamclone-src`), `-DryRun`.

Operator public IP (for Terraform `allowed_ssh_cidr`): run `curl -s https://api.ipify.org` from your workstation before `terraform apply`.

SSH polls `backfill bronze status`, `backfill status`, and `backfill coverage report` via Docker on the VM Mode B network (`streamclone-azure-archive-plane_default`). Snapshots copy locally to `docs/benchmarks/coverage-azure-archive-plane-*.json`.

Local-only bronze runs (same machine as workers): `scripts/bronze-acceptance-run.ps1` — **not** for Mode B Azure proof.

---

## Phase 3 (deferred)

Public site via Caddy, Pulse Wire on Azure, Parquet warehouse — after 24h Bronze proof on Mode B.
