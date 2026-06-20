# Streamclone Azure archive compute (Terraform)

Provisions a **Tailscale-only** Ubuntu VM for the Streamclone hybrid archive plane (Mode A scraper smoke → Mode B capture workers).

| Resource | Purpose |
|----------|---------|
| Resource group | `rg-streamclone-prod` (tags: `project=streamclone`, `env=prod`) |
| Linux VM | `Standard_B2s` — resize to `Standard_B2ms` if Mode B OOM |
| OS disk | 64–128 GB Standard SSD |
| Static public IP | **SSH/bootstrap only** — scraper/Postgres stay on tailnet |
| NSG | SSH (22) from `allowed_ssh_cidr` only; deny other inbound |
| Budget | $25/month at 50/75/90/100% (when `budget_alert_emails` set) |

**Not in scope:** AKS, Azure Database for PostgreSQL, Azure Cache for Redis, Application Gateway, Front Door.

Docs: [docs/azure-archive-plane.md](../../../docs/azure-archive-plane.md) · Blob storage: [deploy/terraform/azure/archive](../archive/README.md)

## Quick start

```bash
az login
cd deploy/terraform/azure/compute
cp terraform.tfvars.example terraform.tfvars
# edit admin_ssh_public_key + allowed_ssh_cidr
terraform init && terraform apply
```

After apply, SSH to the public IP and run [scripts/azure-vm-bootstrap.sh](../../../scripts/azure-vm-bootstrap.sh) (Docker, compose, host Tailscale).

## Customize

| Variable | Purpose |
|----------|---------|
| `vm_size` | Start `Standard_B2s`; use `Standard_B2ms` when memory >80% or Camoufox OOM |
| `allowed_ssh_cidr` | Operator IP `/32` for SSH |
| `budget_alert_emails` | Cost alerts at 50/75/90/100% of `budget_amount_usd` |
| `os_disk_size_gb` | Host disk for Docker images + Postgres named volume |

## Destroy

```bash
terraform destroy
```

Local `terraform.tfstate` is gitignored.
