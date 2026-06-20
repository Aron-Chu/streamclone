resource "azurerm_storage_container" "tfstate" {
  count = var.create_tfstate_container ? 1 : 0

  name                  = var.tfstate_container_name
  storage_account_id    = azurerm_storage_account.archive.id
  container_access_type = "private"
}

resource "azurerm_role_assignment" "deployer_blob_contributor" {
  count = var.grant_deployer_blob_access ? 1 : 0

  scope                = azurerm_storage_account.archive.id
  role_definition_name = "Storage Blob Data Contributor"
  principal_id         = data.azurerm_client_config.current.object_id
}

resource "azurerm_storage_blob" "smoke" {
  count = var.create_smoke_blob ? 1 : 0

  name                   = local.smoke_blob_name
  storage_account_name   = azurerm_storage_account.archive.name
  storage_container_name = azurerm_storage_container.archive.name
  type                   = "Block"
  content_type           = "text/plain"

  source_content = "Streamclone Azure archive smoke test\napplied_at=${timestamp()}\nstorage_account=${azurerm_storage_account.archive.name}\n"

  lifecycle {
    ignore_changes = [source_content]
  }
}

resource "azurerm_consumption_budget_resource_group" "archive" {
  count = var.create_budget_alert ? 1 : 0

  name              = "budget-streamclone-archive"
  resource_group_id = azurerm_resource_group.archive.id
  amount            = var.budget_amount_usd
  time_grain        = "Monthly"

  time_period {
    start_date = var.budget_period_start
    end_date   = var.budget_period_end
  }

  dynamic "notification" {
    for_each = length(var.budget_alert_emails) > 0 ? [90, 100] : []
    content {
      enabled        = true
      threshold      = notification.value
      operator       = "GreaterThan"
      contact_emails = var.budget_alert_emails
    }
  }

  lifecycle {
    ignore_changes = [time_period]
  }
}
