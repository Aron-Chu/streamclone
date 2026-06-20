output "resource_group_name" {
  description = "Azure resource group name."
  value       = azurerm_resource_group.compute.name
}

output "location" {
  description = "Azure region."
  value       = azurerm_resource_group.compute.location
}

output "vm_name" {
  description = "Linux VM name."
  value       = azurerm_linux_virtual_machine.archive.name
}

output "vm_public_ip" {
  description = "Static public IPv4 for SSH/bootstrap only (not for scraper/Postgres)."
  value       = azurerm_public_ip.vm.ip_address
}

output "vm_private_ip" {
  description = "Private IP inside the VNet."
  value       = azurerm_network_interface.vm.private_ip_address
}

output "ssh_command" {
  description = "Example SSH command after apply."
  value       = "ssh ${var.admin_username}@${azurerm_public_ip.vm.ip_address}"
}

output "tailscale_hostname_hint" {
  description = "Register host-level Tailscale with this MagicDNS name."
  value       = "azure-streamclone"
}

output "setup_summary" {
  description = "Human-readable post-apply summary."
  value       = <<-EOT
    Streamclone Azure archive compute VM is ready.

    VM              : ${azurerm_linux_virtual_machine.archive.name}
    Size            : ${var.vm_size}
    Public IP (SSH) : ${azurerm_public_ip.vm.ip_address}
    Resource group  : ${azurerm_resource_group.compute.name}

    Next on the VM (SSH):
      curl -fsSL https://tailscale.com/install.sh | sh
      sudo tailscale up --hostname=azure-streamclone
      bash scripts/azure-vm-bootstrap.sh

    Mode A compose : deploy/docker-compose.azure-scraper.yml
    Mode B compose : deploy/docker-compose.azure-archive-plane.yml
    Runbook        : docs/azure-archive-plane.md
  EOT
}
