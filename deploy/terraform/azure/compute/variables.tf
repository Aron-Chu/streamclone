variable "resource_group_name" {
  description = "Azure resource group for Streamclone archive compute."
  type        = string
  default     = "rg-streamclone-prod"
}

variable "location" {
  description = "Azure region for the archive VM."
  type        = string
  default     = "eastus"
}

variable "vm_name" {
  description = "Linux VM name (Tailscale hostname should match azure-streamclone)."
  type        = string
  default     = "azure-streamclone"
}

variable "vm_size" {
  description = "Azure VM SKU. Start B2s; resize to Standard_B2ms if OOM under Mode B load."
  type        = string
  default     = "Standard_B2s"
}

variable "admin_username" {
  description = "SSH admin username on the VM."
  type        = string
  default     = "streamclone"
}

variable "admin_ssh_public_key" {
  description = "SSH public key for admin_username (required for apply)."
  type        = string
}

variable "allowed_ssh_cidr" {
  description = "CIDR allowed to reach SSH (22) on the public IP. Use operator /32."
  type        = string
}

variable "os_disk_size_gb" {
  description = "OS disk size in GB (64–128 recommended for Camoufox + Postgres volume on host)."
  type        = number
  default     = 128
}

variable "tags" {
  description = "Tags applied to created resources."
  type        = map(string)
  default = {
    project = "streamclone"
    env     = "prod"
    role    = "archive-plane"
  }
}

variable "create_budget_alert" {
  description = "Create a monthly cost budget on the compute resource group."
  type        = bool
  default     = true
}

variable "budget_amount_usd" {
  description = "Monthly budget cap (USD) for compute resource group."
  type        = number
  default     = 25
}

variable "budget_alert_emails" {
  description = "Email addresses for budget alerts (empty = budget without email notifications)."
  type        = list(string)
  default     = []
}

variable "budget_period_start" {
  description = "Budget tracking start (ISO8601)."
  type        = string
  default     = "2026-06-01T00:00:00Z"
}

variable "budget_period_end" {
  description = "Budget tracking end (ISO8601)."
  type        = string
  default     = "2028-06-01T00:00:00Z"
}

variable "custom_data" {
  description = "Optional cloud-init payload (base64). Leave empty to bootstrap via scripts/azure-vm-bootstrap.sh after SSH."
  type        = string
  default     = ""
}
