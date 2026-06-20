data "azurerm_client_config" "current" {}

locals {
  credentials_dir = startswith(var.credentials_output_dir, "~") ? pathexpand(var.credentials_output_dir) : var.credentials_output_dir
  smoke_blob_name = "${var.archive_prefix}/smoke-tests/terraform-apply.txt"
}
