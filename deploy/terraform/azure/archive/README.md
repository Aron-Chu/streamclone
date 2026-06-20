# Streamclone Azure archive (Terraform)

**One command** provisions Phase A cold storage and writes local credentials:

| Resource | Purpose |
|----------|---------|
| Resource group | `rg-streamclone-archive` |
| Storage account | Private archive blobs, versioning, Cool tier after 90d |
| Archive container | `streamclone-archive` |
| tfstate container | Optional remote state migration |
| Smoke blob | Verifies upload path on apply |
| Budget | $5/month cap on resource group |
| RBAC | `Storage Blob Data Contributor` for your `az login` identity |
| Local files | `~/.streamclone/azure-archive-connection-string`, `archive.env.local.snippet` |

Docs: [docs/azure-archive-setup.md](../../../docs/azure-archive-setup.md)

## Quick start

```bash
az login
bash scripts/azure-archive-fresh-start.sh
```

Or directly:

```bash
cd deploy/terraform/azure/archive
terraform init && terraform apply
```

After apply, merge `~/.streamclone/archive.env.local.snippet` into repo `.env.local`.

## Customize

```bash
cp terraform.tfvars.example terraform.tfvars
# edit budget_alert_emails, env_local_connection_string_path, location, etc.
terraform apply
```

## Remote state (optional)

After first apply, copy `backend.tf.example` → `backend.tf`, set `storage_account_name` from `terraform output storage_account_name`, then `terraform init -migrate-state`.

## Destroy

```bash
terraform destroy
```

Local `terraform.tfstate` and generated credentials are gitignored.
