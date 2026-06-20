resource "azurerm_consumption_budget_resource_group" "compute" {
  count = var.create_budget_alert ? 1 : 0

  name              = "budget-streamclone-compute"
  resource_group_id = azurerm_resource_group.compute.id
  amount            = var.budget_amount_usd
  time_grain        = "Monthly"

  time_period {
    start_date = var.budget_period_start
    end_date   = var.budget_period_end
  }

  dynamic "notification" {
    for_each = length(var.budget_alert_emails) > 0 ? [50, 75, 90, 100] : []
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
