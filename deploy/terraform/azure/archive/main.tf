resource "random_string" "storage_suffix" {
  length  = 6
  special = false
  upper   = false
}

locals {
  storage_account_name = substr(
    replace("${var.storage_account_prefix}${random_string.storage_suffix.result}", "-", ""),
    0,
    24
  )
}

resource "azurerm_resource_group" "archive" {
  name     = var.resource_group_name
  location = var.location
  tags     = var.tags
}

resource "azurerm_storage_account" "archive" {
  name                            = local.storage_account_name
  resource_group_name             = azurerm_resource_group.archive.name
  location                        = azurerm_resource_group.archive.location
  account_tier                    = "Standard"
  account_replication_type        = "LRS"
  min_tls_version                 = "TLS1_2"
  allow_nested_items_to_be_public = false

  blob_properties {
    versioning_enabled = var.enable_versioning
  }

  tags = var.tags
}

resource "azurerm_storage_container" "archive" {
  name                  = var.container_name
  storage_account_id    = azurerm_storage_account.archive.id
  container_access_type = "private"
}

resource "azurerm_storage_management_policy" "archive" {
  count = var.cool_tier_after_days > 0 ? 1 : 0

  storage_account_id = azurerm_storage_account.archive.id

  rule {
    name    = "move-to-cool-after-${var.cool_tier_after_days}-days"
    enabled = true

    filters {
      blob_types   = ["blockBlob"]
      prefix_match = ["${var.archive_prefix}/"]
    }

    actions {
      base_blob {
        tier_to_cool_after_days_since_modification_greater_than = var.cool_tier_after_days
      }
    }
  }
}
