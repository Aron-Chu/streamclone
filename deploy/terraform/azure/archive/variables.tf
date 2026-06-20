variable "resource_group_name" {
  description = "Azure resource group for Streamclone archive resources."
  type        = string
  default     = "rg-streamclone-archive"
}

variable "location" {
  description = "Azure region. eastus is common for free-tier experiments."
  type        = string
  default     = "eastus"
}

variable "storage_account_prefix" {
  description = "Prefix for the storage account name (3-24 lowercase alphanumeric; suffix added for uniqueness)."
  type        = string
  default     = "ststreamclone"
}

variable "container_name" {
  description = "Private blob container for archive objects (matches ARCHIVE_AZURE_CONTAINER)."
  type        = string
  default     = "streamclone-archive"
}

variable "archive_prefix" {
  description = "Blob path prefix inside the container (matches ARCHIVE_AZURE_PREFIX)."
  type        = string
  default     = "streamclone"
}

variable "cool_tier_after_days" {
  description = "Move blobs to Cool tier after N days (0 disables lifecycle rule)."
  type        = number
  default     = 90
}

variable "enable_versioning" {
  description = "Enable blob versioning on the storage account."
  type        = bool
  default     = true
}

variable "tags" {
  description = "Tags applied to created resources."
  type        = map(string)
  default = {
    app         = "streamclone"
    component   = "archive"
    environment = "personal"
  }
}

variable "write_local_credentials" {
  description = "Write connection string + .env snippet to credentials_output_dir after apply."
  type        = bool
  default     = true
}

variable "credentials_output_dir" {
  description = "Directory for generated credentials (supports ~ expansion)."
  type        = string
  default     = "~/.streamclone"
}

variable "env_local_connection_string_path" {
  description = "Path embedded in generated .env.local snippet (use a path Docker/Windows can read)."
  type        = string
  default     = "C:/Users/Aron/.streamclone/azure-archive-connection-string"
}

variable "create_smoke_blob" {
  description = "Upload a smoke-test blob on apply."
  type        = bool
  default     = true
}

variable "grant_deployer_blob_access" {
  description = "Grant Storage Blob Data Contributor to the az login identity running Terraform."
  type        = bool
  default     = true
}

variable "create_budget_alert" {
  description = "Create a monthly cost budget on the archive resource group."
  type        = bool
  default     = true
}

variable "budget_amount_usd" {
  description = "Monthly budget cap (USD) for archive resource group."
  type        = number
  default     = 5
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

variable "create_tfstate_container" {
  description = "Create a private tfstate container for optional remote backend migration."
  type        = bool
  default     = true
}

variable "tfstate_container_name" {
  description = "Container name reserved for Terraform remote state."
  type        = string
  default     = "tfstate"
}
