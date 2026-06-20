# Azure archive — CI/CD, Kubernetes, and secrets

Status: architecture guide (2026-06)
Related: [azure-archive-setup.md](azure-archive-setup.md), [finalplan.md](finalplan.md), [scraping-archive/requirements.md](scraping-archive/requirements.md)

---

## TL;DR

| Question | Answer |
|----------|--------|
| **CI/CD for blob archive infra?** | **GitHub Actions + Terraform** — `plan` on PR, `apply` on manual approval. |
| **ArgoCD?** | **Not for archive.** Only if you GitOps-deploy **Kubernetes apps** (e.g. Pulse charts). |
| **Kubernetes?** | **Optional** for Grafana/Pulse. **Not** for Streamclone core, scraper, or blob storage. |
| **Vault?** | **Overkill** for solo dev. Use **`.env.local` → Azure Key Vault → GitHub OIDC** in that order. |

---

## What you are deploying

```text
┌─────────────────────────────────────────────────────────────┐
│  Layer 1 — Azure infra (Terraform)                          │
│  Resource group, storage account, private container         │
│  CI: GitHub Actions terraform plan/apply                    │
└─────────────────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────────────────┐
│  Layer 2 — Streamclone runtime (local / VM)                 │
│  Docker Compose: analytics, postgres, scraper, frontend     │
│  Archive export worker reads Postgres → writes Blob Storage │
└─────────────────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────────────────┐
│  Layer 3 — Optional observability (K8s / Helm)              │
│  charts/pulse: Influx, Grafana, Prometheus                  │
└─────────────────────────────────────────────────────────────┘
```

Blob archive does not run inside Kubernetes.

---

## Recommended CI/CD (Terraform → Azure)

### Goals

- No long-lived storage keys in GitHub if avoidable
- PRs show `terraform plan` diff
- `apply` only with human approval
- Terraform state in Azure Storage (after account exists), not committed

### Identity: GitHub OIDC → Azure

**Preferred for GitHub Actions → Azure:**

1. Create **App registration** + **federated credential** for `token.actions.githubusercontent.com`
2. Grant **Contributor** on resource group `rg-streamclone-archive` (or scoped custom role)
3. Workflow uses `azure/login@v2` with `client-id`, `tenant-id`, `subscription-id` — no `AZURE_CREDENTIALS` JSON secret

**Local dev:** `az login` (your user).

**Streamclone app uploads (home PC):** connection string file in `%USERPROFILE%\.streamclone\` until export worker runs on a VM with managed identity.

### Workflow shape

```text
PR opened / updated
  └─ job: terraform fmt -check, init, plan

workflow_dispatch (with environment approval)
  └─ job: terraform apply -auto-approve
       └─ GitHub Environment: production-azure
```

### State backend (after first apply)

```hcl
terraform {
  backend "azurerm" {
    resource_group_name  = "rg-streamclone-archive"
    storage_account_name = "YOUR_TF_STATE_ACCOUNT"
    container_name       = "tfstate"
    key                  = "archive.terraform.tfstate"
  }
}
```

Use a **separate** small storage account for Terraform state, or the same account with a dedicated container.

### What **not** to put in CI yet

- Full Streamclone compose deploy to Azure
- TwitchTracker scraper in CI
- Automatic `terraform apply` on every green PR without approval

---

## Secrets ladder (solo dev → team)

| Stage | Archive credentials | Twitch / proxy |
|-------|---------------------|----------------|
| **1** | `.env.local` + file under `~/.streamclone/` | `.env.local` |
| **2** | Azure Key Vault secret reference on a VM | Same vault |
| **3** | Managed identity on Azure VM / Container Apps | Key Vault |

Skip HashiCorp Vault unless you already operate it.

---

## Repo variables (CI plan)

| Variable | Example |
|----------|---------|
| `STREAMCLONE_AZURE_RG` | `rg-streamclone-archive` |
| `STREAMCLONE_AZURE_LOCATION` | `eastus` |

Workflow falls back to placeholders when unset (validate-only plan).

---

## Files

- Terraform: [deploy/terraform/azure/archive/](../deploy/terraform/azure/archive/)
- Install: [scripts/install-azure-archive-tools.sh](../scripts/install-azure-archive-tools.sh)
- Fresh start: [scripts/azure-archive-fresh-start.sh](../scripts/azure-archive-fresh-start.sh)
- CI: [.github/workflows/azure-archive-terraform.yml](../.github/workflows/azure-archive-terraform.yml)
