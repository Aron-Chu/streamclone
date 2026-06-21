#Requires -Version 5.1
# Enable Grafana + Prometheus on BearHost VPS (via WSL rsync + SSH).
param(
    [switch]$SkipRebuild
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'wsl-path.ps1')
$paths = Get-RepoWslPath -StartPath $PSScriptRoot
$wslRepo = $paths.WslRepo

$rebuild = if ($SkipRebuild) { 'true' } else { 'false' }
$cmd = @"
cd '$wslRepo' && bash scripts/bearhost-rsync-to-vps.sh && \
source scripts/lib/bearhost-ssh.sh && bearhost_ssh_config && \
bearhost_ssh 'cd ${BEARHOST_REMOTE_APP:-/opt/streamclone/app} && bash scripts/bearhost-observability.sh up' && \
if [ '$rebuild' != 'true' ]; then \
  bearhost_ssh 'cd /opt/streamclone/app && docker compose --env-file .env --env-file deploy/env/profile-full.env --env-file deploy/env/profile-archive.env --env-file deploy/env/profile-bearhost-prod.env -f deploy/docker-compose.yml -f deploy/docker-compose.prod.yml -f deploy/docker-compose.bearhost-prod.yml -f deploy/docker-compose.bearhost-build.yml --profile scraper up -d --build analytics-workers'; \
fi
"@

Write-Host "==> Enable BearHost observability (Grafana + Prometheus)"
wsl bash -lc $cmd
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Write-Host ""
Write-Host "Next: powershell -File scripts/bearhost-grafana-tunnel.ps1"
Write-Host "Open:  http://localhost:3001/d/streamclone-archive/streamclone-archive"
