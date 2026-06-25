#!/usr/bin/env bash
# Bootstrap laptopworker: Docker, Streamclone core dev stack (no local scraper/workers).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=laptopworker-env.sh
source "$ROOT/scripts/laptopworker-env.sh"

INSTALL_DIR="${STREAMCLONE_INSTALL_DIR:-$HOME/streamclone}"
SKIP_POWER="${SKIP_POWER:-}"
SKIP_CLONE="${SKIP_CLONE:-}"
SKIP_UP="${SKIP_UP:-}"

usage() {
  cat <<'EOF'
Usage: scripts/laptopworker-bootstrap.sh [options]

  --install-dir PATH   Checkout path (default: ~/streamclone)
  --skip-power         Skip lid-ignore / sleep mask (already configured)
  --skip-clone         Repo already at install dir
  --no-up              Synthesize .env only; do not start compose
  -h, --help
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --install-dir) INSTALL_DIR="$2"; shift 2 ;;
    --skip-power) SKIP_POWER=1; shift ;;
    --skip-clone) SKIP_CLONE=1; shift ;;
    --no-up) SKIP_UP=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage >&2; exit 1 ;;
  esac
done

echo "==> Streamclone laptopworker bootstrap"
echo "    Install dir: $INSTALL_DIR"
echo "    Network-heavy scrape/corpus: BearHost VPS (not this host)"

if ! laptopworker_required_files "$ROOT"; then
  echo "Run bootstrap from a checkout that contains laptopworker files (not bare origin/master until merged)." >&2
  exit 1
fi

if [ -z "$SKIP_POWER" ]; then
  bash "$ROOT/scripts/laptopworker-power-config.sh"
fi

echo "==> Installing base packages..."
sudo apt-get update -qq
sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -qq \
  ca-certificates curl git make jq

if ! command -v tailscale >/dev/null 2>&1; then
  echo "Tailscale CLI missing — install from https://tailscale.com/download/linux" >&2
  exit 1
fi
if ! systemctl is-active --quiet tailscaled 2>/dev/null; then
  echo "tailscaled is not active — run: sudo tailscale up --hostname=laptopworker" >&2
  exit 1
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "==> Installing Docker..."
  curl -fsSL https://get.docker.com | sudo sh
fi
sudo usermod -aG docker "${USER}" || true

if ! docker compose version >/dev/null 2>&1; then
  echo "docker compose plugin missing" >&2
  exit 1
fi

mkdir -p "${HOME}/.streamclone/secrets"
chmod 700 "${HOME}/.streamclone" "${HOME}/.streamclone/secrets"

if [ -z "$SKIP_CLONE" ] && [ ! -d "$INSTALL_DIR/.git" ]; then
  echo "==> Cloning Streamclone to $INSTALL_DIR..."
  git clone --depth 1 https://github.com/Aron-Chu/streamclone.git "$INSTALL_DIR"
fi

install_root="$(cd "$INSTALL_DIR" && pwd)"
if [ "$ROOT" != "$install_root" ]; then
  echo "==> Syncing laptopworker files into $INSTALL_DIR..."
  laptopworker_sync_files "$ROOT" "$INSTALL_DIR"
fi

if ! laptopworker_required_files "$INSTALL_DIR"; then
  cat >&2 <<EOF
laptopworker files missing in $INSTALL_DIR after clone/sync.
Push/merge laptopworker scripts to origin/master, or re-run bootstrap from a dev checkout that contains them.
EOF
  exit 1
fi

cd "$INSTALL_DIR"
echo "==> Synthesizing .env (core profile + laptopworker overrides)..."
laptopworker_synth_env "$INSTALL_DIR"

if [ "$SKIP_UP" != "1" ]; then
  # shellcheck source=lib/env.sh
  source "$INSTALL_DIR/scripts/lib/env.sh"
  if ! docker info >/dev/null 2>&1; then
    if sg docker -c "docker info" >/dev/null 2>&1; then
      echo "Docker OK via sg docker — continuing"
    else
      echo "Docker not ready — re-login or: newgrp docker" >&2
      echo "Then: bash scripts/laptopworker-stack.sh start" >&2
      exit 1
    fi
  fi
  echo "==> Starting core dev stack..."
  bash scripts/laptopworker-stack.sh start
  bash scripts/laptopworker-stack.sh smoke || true
fi

cat <<EOF

Bootstrap complete.

  UI (tailnet):  http://laptopworker:8090
  Stack control: bash scripts/laptopworker-stack.sh {start|stop|status|logs|smoke}
  Boot service:  bash scripts/laptopworker-stack.sh install-service
                 then once: sudo loginctl enable-linger $USER
  Secrets dir:   ~/.streamclone/secrets/
  Copy from PC:  oauth-bundle.env → deploy/env/oauth-bundle.env (optional)

  Scrape + corpus workers stay on BearHost VPS — do not enable scraper profile here.

Optional tailnet firewall (once):
  bash scripts/laptopworker-stack.sh ufw-tailnet

EOF

if ! groups | grep -q docker; then
  echo "Re-login (or: newgrp docker) so docker group membership applies."
fi
