# Azure archive setup (Streamclone)

Status: operator guide for Phase A infrastructure
Related: [finalplan.md](finalplan.md), [scraping-archive/requirements.md](scraping-archive/requirements.md), [deploy/terraform/azure/archive/README.md](../deploy/terraform/azure/archive/README.md)

Streamclone cold archive uses **Azure Blob Storage**. Postgres remains the hot cache; blobs are the durable tier.

---

## One-shot deploy

```bash
az login
bash scripts/azure-archive-fresh-start.sh
```

**Single `terraform apply` creates:**

- Resource group + storage account + private containers (archive + tfstate)
- Cool-tier lifecycle rule (90 days)
- Monthly budget ($5 default)
- RBAC for your Azure login (`Storage Blob Data Contributor`)
- Smoke-test blob under `streamclone/smoke-tests/`
- Local credentials in `~/.streamclone/`:
  - `azure-archive-connection-string`
  - `archive.env.local.snippet` → merge into repo `.env.local`
  - `archive-setup.txt`

No manual `terraform output`, `az storage cp`, or connection-string copy steps.

---

## After apply

1. Merge snippet into `.env.local`:

   ```bash
   cat ~/.streamclone/archive.env.local.snippet >> .env.local
   ```

2. Set `ARCHIVE_ENABLED=true` in `.env.local` (or merge `deploy/env/profile-archive.env`) after verifying blob list access.

3. Run a test sync with `ARCHIVE_EXPORT_ON_SYNC=true` and confirm `archive_exports` + blob under `streamclone/rollups/`.

---

## Customize (optional)

```bash
cd deploy/terraform/azure/archive
cp terraform.tfvars.example terraform.tfvars
```

Useful settings:

| Variable | Purpose |
|----------|---------|
| `budget_alert_emails` | Email when spend hits 90%/100% of budget |
| `env_local_connection_string_path` | Path Docker on Windows reads for connection string |
| `credentials_output_dir` | Where Terraform writes local files (default `~/.streamclone`) |
| `location` | Azure region (default `eastus`) |

Then `terraform apply` again.

---

## Manual path

```bash
cd deploy/terraform/azure/archive
terraform init && terraform apply
terraform output setup_summary
```

---

## Teardown

```bash
cd deploy/terraform/azure/archive
terraform destroy
```

---

## Troubleshooting

| Error | Fix |
|-------|-----|
| `/usr/bin/env: 'bash\r'` | `sed -i 's/\r$//' scripts/azure-archive-fresh-start.sh` |
| `Please run az login` | `az login` |
| Budget create fails | Set `create_budget_alert=false` in tfvars (some subscriptions restrict budgets) |
| Role assignment slow | Wait 1–2 min; re-run `terraform apply` if smoke blob fails |

---

## Restore rollups

Selective Analytics restore **without** TwitchTracker re-scrape (acceptance criterion #1 in [scraping-archive/requirements.md](scraping-archive/requirements.md)):

1. Confirm `ARCHIVE_ENABLED=true` and a confirmed `archive_exports` row for the stream (rollup artifact).
2. Truncate or reset local rollup tables for that stream (or full Postgres reset).
3. Restore from blob:

   ```bash
   go run ./cmd/archive restore --stream-id <twitch-stream-id>
   ```

4. Open the channel Analytics page — viewer chart should load from restored rollups without a new TT scrape.

Full database restore uses the gzip pg_dump under `streamclone/postgres/nightly/` (see `scripts/backup-streamclone.ps1` output). Rollup-only restore is faster for single-stream smoke tests.

---

## Next operator tasks

- Run restore smoke for a known exported stream (e.g. after sync with `ARCHIVE_EXPORT_ON_SYNC=true`).
- Enable `deploy/env/profile-archive.env` on analytics when ready for retention guard + Tier-0 exports.
- Phase 2 Bronze bulk index (`channels/top500.json.gz`, Helix VOD index, TT summary) — see [scraping-archive/requirements.md](scraping-archive/requirements.md) Phase 2 checklist.
