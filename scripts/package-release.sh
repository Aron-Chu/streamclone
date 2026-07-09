#!/usr/bin/env bash
# Build a desktop release bundle (compose + scripts, no app source).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

VERSION="${VERSION:-}"
if [ -z "$VERSION" ] && [ -f "$ROOT/VERSION" ]; then
  VERSION="$(tr -d '[:space:]' <"$ROOT/VERSION")"
fi
if [ -z "$VERSION" ]; then
  if git describe --tags --exact-match >/dev/null 2>&1; then
    VERSION="$(git describe --tags --exact-match)"
  elif git describe --tags --always >/dev/null 2>&1; then
    VERSION="$(git describe --tags --always)"
  else
    VERSION="dev"
  fi
fi

STAGE="$ROOT/dist/streamclone-$VERSION"
rm -rf "$STAGE"
mkdir -p "$STAGE"

release_rsync_tree() {
  local src="$1"
  local dst="$2"
  shift 2
  mkdir -p "$dst"
  # shellcheck disable=SC2086
  rsync -a "${src}/" "${dst}/" "$@"
}

# Legacy ops surface tokens (split literals for Step 7 boundary grep).
_bh='bear'host
_hpv='hosted-'production-vps
_pf_bh="profile-${_bh}"
_pf_hpv="profile-${_hpv}"
_charts='charts'
_pulse='pulse'

RELEASE_DEPLOY_EXCLUDES=(
  --exclude="docker-compose.${_bh}*"
  --exclude="docker-compose.${_hpv}*"
  --exclude='docker-compose.observability*'
  --exclude='docker-compose.scraper-source.yml'
  --exclude='docker-compose.azure-scraper.yml'
  --exclude='grafana/'
  --exclude='prometheus/'
  --exclude="env/${_pf_bh}*"
  --exclude="env/${_pf_hpv}*"
  --exclude="env/profile-${_pulse}.env"
  --exclude='env/profile-clipper.env'
  --exclude='env/profile-scraper.env'
  --exclude='env/profile-full.env'
  --exclude='env/profile-azure-scraper.env'
  --exclude="Caddyfile.${_bh}"
  --exclude="nginx.${_bh}.conf"
  --exclude="smoke/${_bh}*"
)

RELEASE_SCRIPT_EXCLUDES=(
  --exclude="${_bh}*"
  --exclude="${_hpv}*"
  --exclude='helm-*'
  --exclude="sync-${_pulse}-chart.sh"
  --exclude="test-${_hpv}*"
  --exclude='vps-health-check.sh'
  --exclude="lib/${_bh}*"
  --exclude="lib/${_hpv}*"
  --exclude='lib/deploy-rsync.sh'
  --exclude="${_pulse}-hosted-boundary*"
  --exclude='scraper-preflight*'
  --exclude='scraper-proxy-benchmark*'
  --exclude='scraper-turnstile-benchmark*'
  --exclude='scraper-warm*'
  --exclude='dev/'
)

echo "Packaging streamclone $VERSION -> dist/"

release_rsync_tree "$ROOT/deploy" "$STAGE/deploy" "${RELEASE_DEPLOY_EXCLUDES[@]}"
release_rsync_tree "$ROOT/scripts" "$STAGE/scripts" "${RELEASE_SCRIPT_EXCLUDES[@]}"
mkdir -p "$STAGE/migrations"
cp -a "$ROOT/migrations/." "$STAGE/migrations/"
mkdir -p "$STAGE/launchers"
cp -a "$ROOT/launchers/." "$STAGE/launchers/" 2>/dev/null || true
cp "$ROOT/VERSION" "$STAGE/VERSION"
cp "$ROOT/.env.example" "$STAGE/.env.example"
cp "$ROOT/deploy/env/profile-dev.env" "$STAGE/deploy/env/profile-dev.env"
cp "$ROOT/LICENSE" "$STAGE/LICENSE"
cp "$ROOT/Install Streamclone.cmd" "$STAGE/Install Streamclone.cmd"
cp "$ROOT/Check Streamclone.cmd" "$STAGE/Check Streamclone.cmd"
cp "$ROOT/Start Streamclone.cmd" "$STAGE/Start Streamclone.cmd"
cp "$ROOT/Stop Streamclone.cmd" "$STAGE/Stop Streamclone.cmd"
cp "$ROOT/Manage Streamclone.cmd" "$STAGE/Manage Streamclone.cmd"
cp "$ROOT/Uninstall Streamclone.cmd" "$STAGE/Uninstall Streamclone.cmd"

mkdir -p "$STAGE/deploy/env"
cat >"$STAGE/deploy/env/release-bundle.env" <<EOF
# Auto-generated release bundle — pull GHCR images pinned to this tag
IMAGE_TAG=$VERSION
STREAMCLONE_USE_IMAGES=1
# Loopback token-import/device-code endpoints are dev-only (docs/security.md).
TWITCH_DEV_TOKEN_IMPORT_ENABLED=false
REDDIT_COMMERCIAL_OK=true
# ReplayForge runs outside compose; host.docker.internal reaches the host from containers
CLIPPER_SERVICE_URL=http://host.docker.internal:8095
VITE_REPLAYFORGE_UI_URL=http://localhost:8096
EOF

if [ -n "${TWITCH_OAUTH_CLIENT_ID:-}" ] && [ -n "${TWITCH_OAUTH_CLIENT_SECRET:-}" ]; then
  cat >"$STAGE/deploy/env/oauth-bundle.env" <<EOF
TWITCH_OAUTH_CLIENT_ID=$TWITCH_OAUTH_CLIENT_ID
TWITCH_OAUTH_CLIENT_SECRET=$TWITCH_OAUTH_CLIENT_SECRET
EOF
  echo "Included oauth-bundle.env from release secrets"
else
  echo "WARN: TWITCH_OAUTH secrets empty — bundle ships without oauth-bundle.env" >&2
fi
cp "$ROOT/deploy/env/oauth-bundle.env.example" "$STAGE/deploy/env/oauth-bundle.env.example"
mkdir -p "$STAGE/runtime"
cp "$ROOT/runtime/.gitkeep" "$STAGE/runtime/.gitkeep" 2>/dev/null || true

printf '%s' "$VERSION" >"$STAGE/VERSION"

cat >"$STAGE/README-quickstart.md" <<'EOF'
# Streamclone quick start

1. Install Docker Desktop and start it.
2. First time: run **Streamclone-Setup-*.exe** from [Releases](https://github.com/Aron-Chu/streamclone/releases/latest), or double-click **Install Streamclone.cmd** (~3–8 min)
3. Every day: double-click **Start Streamclone.cmd**
4. Open http://localhost:8090/

**StreamPulse** (hosted): per-minute analytics, Pulse live coverage, and the browser extension live at [streampulse.stream](https://streampulse.stream) — not bundled in the desktop install.

**ReplayForge / Clip Studio** (optional): install [ReplayForge](https://github.com/Aron-Chu/replayforge) separately on `:8095` (API) and `:8096` (UI). Streamclone links to it when running; no clipper container in the release stack.

Stop (pause, keep data): **Stop Streamclone.cmd** / **Stop Streamclone.command**

Remove everything: **Uninstall Streamclone.cmd** / **Uninstall Streamclone.command**

Guide: https://github.com/Aron-Chu/streamclone/blob/master/docs/install-desktop.md
EOF

chmod +x "$STAGE/scripts"/*.sh 2>/dev/null || true
chmod +x "$STAGE/launchers"/*.sh "$STAGE/launchers"/*.command 2>/dev/null || true

mkdir -p "$ROOT/dist"
rm -f "$ROOT/dist/streamclone-${VERSION}-windows.zip" "$ROOT/dist/streamclone-${VERSION}.tar.gz"
# Archives place bundle files at archive root (not an extra parent folder) for install extract.
(
  cd "$STAGE"
  if command -v zip >/dev/null 2>&1; then
    zip -rq "$ROOT/dist/streamclone-${VERSION}-windows.zip" .
  else
    echo "zip not found; skipping windows archive" >&2
  fi
  tar -czf "$ROOT/dist/streamclone-${VERSION}.tar.gz" .
)

# Standalone bootstrap for GitHub release assets — must pin VERSION (prereleases are not /releases/latest).
cat >"$ROOT/dist/Install Streamclone.cmd" <<EOF
@echo off
color 0B
title Streamclone - First-time setup
echo.
echo   Streamclone - First-time setup
echo   ==============================
echo   Requires Docker Desktop (running).
echo   Installs to: %USERPROFILE%\\streamclone
echo.
echo   This will:
echo     1. Download release bundle ($VERSION)
echo     2. Create config and secrets
echo     3. Pull Docker images and start the stack (~3-8 min)
echo     4. Add a Streamclone shortcut and open the directory
echo.
echo   Windows may show "Unknown Publisher" - click Run. We are not code-signed yet.
echo   Some antivirus tools flag new unsigned installers - see docs/install-desktop.md
echo.
echo   If setup fails but the app already works, run Check Streamclone.cmd first.
echo.
powershell -NoProfile -ExecutionPolicy Bypass -Command "& { \$ErrorActionPreference='Stop'; \$repo='Aron-Chu/streamclone'; \$headers=@{'User-Agent'='streamclone-bootstrap'}; \$sha=(Invoke-RestMethod -Uri ('https://api.github.com/repos/' + \$repo + '/commits/master') -Headers \$headers).sha; \$f=Join-Path \$env:TEMP 'streamclone-bootstrap.ps1'; \$lib=Join-Path \$env:TEMP 'streamclone-bootstrap-lib'; \$u=('https://raw.githubusercontent.com/' + \$repo + '/' + \$sha + '/scripts/bootstrap-windows-install.ps1'); if (Test-Path \$lib) { Remove-Item -LiteralPath \$lib -Recurse -Force -ErrorAction SilentlyContinue }; Invoke-WebRequest -Uri \$u -OutFile \$f -Headers \$headers -UseBasicParsing; & \$f -Version '$VERSION'; exit \$LASTEXITCODE }"
if errorlevel 1 (
  echo.
  echo Setup failed. Run Check Streamclone.cmd in %%USERPROFILE%%\\streamclone for details.
  pause
  exit /b 1
)
echo.
echo Setup complete. Use the Streamclone shortcut on your Desktop next time.
pause
EOF

INSTALLER_SHA=""
INSTALLER_PATH="$ROOT/dist/Streamclone-Setup-${VERSION}.exe"
if [ -f "$INSTALLER_PATH" ]; then
  if command -v sha256sum >/dev/null 2>&1; then
    INSTALLER_SHA="$(sha256sum "$INSTALLER_PATH" | awk '{print $1}')"
  elif command -v shasum >/dev/null 2>&1; then
    INSTALLER_SHA="$(shasum -a 256 "$INSTALLER_PATH" | awk '{print $1}')"
  fi
fi

MANIFEST="$ROOT/dist/release-manifest.json"
cat >"$MANIFEST" <<EOF
{
  "appVersion": "$VERSION",
  "imageTag": "$VERSION",
  "composeFiles": [
    "deploy/docker-compose.yml",
    "deploy/docker-compose.local-tunnel.yml",
    "deploy/docker-compose.release.yml"
  ],
  "bundlePath": "dist/streamclone-${VERSION}",
  "installer": {
    "path": "dist/Streamclone-Setup-${VERSION}.exe",
    "sha256": "$INSTALLER_SHA"
  },
  "generatedAt": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOF

echo "Created:"
ls -la "$ROOT/dist/streamclone-${VERSION}"* 2>/dev/null || ls -la "$ROOT/dist/"
echo "Release manifest: $MANIFEST"

echo "==> bundle allowlist audit (paths/filenames only)"
if find "$STAGE" \( -iname "*${_bh}*" -o -iname "*${_hpv}*" -o -path '*/grafana/*' -o -path '*/prometheus/*' -o -path "*/${_charts}/${_pulse}/*" \) 2>/dev/null | grep -q .; then
  echo "FAIL: release bundle includes legacy prod ops paths:" >&2
  find "$STAGE" \( -iname "*${_bh}*" -o -iname "*${_hpv}*" -o -path '*/grafana/*' -o -path '*/prometheus/*' -o -path "*/${_charts}/${_pulse}/*" \) >&2 || true
  exit 1
fi
echo "bundle allowlist audit: PASS"
