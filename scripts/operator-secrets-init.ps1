# Create %USERPROFILE%\.streamclone operator secret directory with README (no secret values).
$ErrorActionPreference = 'Stop'
$Dir = if ($env:STREAMCLONE_SECRETS_DIR) { $env:STREAMCLONE_SECRETS_DIR } else { Join-Path $env:USERPROFILE '.streamclone' }
New-Item -ItemType Directory -Force -Path $Dir | Out-Null
@'
Streamclone operator secrets (local only — never commit)

Files in this directory:
  alertmanager-webhook-url          Discord webhook for Alertmanager (one line)
  azure-archive-connection-string   Azure Blob connection string
  archive.env.local.snippet         Optional snippet to merge into repo .env.local

Docs: streamclone docs/operator-secrets.md
Install BearHost webhook (WSL): bash scripts/bearhost-alertmanager-secret-install.sh
'@ | Set-Content -Encoding utf8 (Join-Path $Dir 'README')
Write-Host "operator-secrets-init: created $Dir"
Get-ChildItem $Dir
