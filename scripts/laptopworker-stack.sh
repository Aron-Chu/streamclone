#!/usr/bin/env bash
# Remote start/stop for laptopworker dev stack (docker compose — not machine power).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
# shellcheck source=laptopworker-env.sh
source "$ROOT/scripts/laptopworker-env.sh"

if ! laptopworker_required_files "$ROOT"; then
  echo "laptopworker overlay missing — see docs/laptopworker-dev.md" >&2
  exit 1
fi

_laptopworker_up() {
  laptopworker_compose_up "$ROOT"
}

usage() {
  cat <<'EOF'
Usage: scripts/laptopworker-stack.sh <command>

  start    Start core dev stack (detached)
  stop     Stop containers (machine stays awake)
  restart  stop then start
  status   docker compose ps
  logs     Follow compose logs (Ctrl+C)
  smoke    Health via :8090 extension + frontend
  update   git pull + resynth .env + compose up (after push to master)
  install-service  systemd user unit + boot linger (run once on laptop)
  ufw-tailnet      Apply tailnet-only UFW rules (sudo once)
  enable-linger    sudo loginctl enable-linger (sudo once; boot without login)
  boot-check       Linger, systemd unit, script perms, smoke (no sudo)
  setup            One-shot remote QoL (sudo once at Windows desk)
  setup-verify     Confirm sudoers, ufw, linger, boot order (no sudo)
EOF
}

cmd="${1:-}"
case "$cmd" in
  start)
    _laptopworker_up
    laptopworker_compose "$ROOT" ps
    ;;
  stop)
    laptopworker_compose "$ROOT" stop
    laptopworker_stop_storygraph
    ;;
  restart)
    laptopworker_compose "$ROOT" stop
    _laptopworker_up
    laptopworker_compose "$ROOT" ps
    ;;
  status)
    laptopworker_compose "$ROOT" ps
    ;;
  logs)
    laptopworker_compose "$ROOT" logs -f --tail=100
    ;;
  smoke)
    curl -fsS "http://127.0.0.1:8090/v1/extension/health" | head -c 240
    echo
    code="$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8090/)"
    echo "OK frontend / → HTTP $code"
    ;;
  update)
    bash "$ROOT/scripts/laptopworker-update.sh"
    ;;
  install-service)
    bash "$ROOT/scripts/laptopworker-install-service.sh"
    ;;
  ufw-tailnet)
    bash "$ROOT/scripts/laptopworker-ufw-tailnet.sh"
    ;;
  enable-linger)
    if ! command -v loginctl >/dev/null 2>&1; then
      echo "loginctl not found" >&2
      exit 1
    fi
    sudo -n loginctl enable-linger "$(id -un)" 2>/dev/null || sudo loginctl enable-linger "$(id -un)"
    loginctl show-user "$(id -un)" -p Linger
    ;;
  setup)
    bash "$ROOT/scripts/laptopworker-setup-remote.sh"
    ;;
  setup-verify)
    echo "==> root-owned helpers (/usr/local/sbin)"
    for h in firewall power boot; do
      if [ -x "/usr/local/sbin/streamclone-laptopworker-${h}" ]; then
        echo "  streamclone-laptopworker-${h} ok"
      else
        echo "  streamclone-laptopworker-${h} MISSING — run setup"
      fi
    done
    echo "==> sudoers"
    if sudo -n grep -q '/usr/local/sbin/streamclone-laptopworker-firewall' /etc/sudoers.d/streamclone-laptopworker 2>/dev/null; then
      echo "ok (root-owned helpers only)"
    elif [ -f /etc/sudoers.d/streamclone-laptopworker ]; then
      echo "STALE — re-run setup"
    else
      echo "missing — run setup"
    fi
    echo "==> boot-time firewall unit"
    systemctl is-enabled streamclone-laptopworker-firewall.service 2>/dev/null || echo "not enabled"
    echo "==> firewall rules"
    if [ -x /usr/local/sbin/streamclone-laptopworker-firewall ]; then
      sudo -n /usr/local/sbin/streamclone-laptopworker-firewall --verify 2>/dev/null || \
        echo "rules missing — run: scripts/laptopworker-remote.cmd ufw-tailnet"
    else
      echo "helper not installed"
    fi
    echo "==> stack boot-check"
    laptopworker_ensure_scripts_executable "$ROOT"
    loginctl show-user "$(id -un)" -p Linger 2>/dev/null || true
    systemctl --user is-active streamclone-dev.service 2>/dev/null || echo "streamclone-dev inactive"
    curl -fsS "http://127.0.0.1:8090/v1/extension/health" | head -c 120
    echo
    ;;
  boot-check)
    laptopworker_ensure_scripts_executable "$ROOT"
    echo "==> linger"
    loginctl show-user "$(id -un)" -p Linger 2>/dev/null || echo "loginctl unavailable"
    echo "==> streamclone-dev.service"
    systemctl --user is-enabled streamclone-dev.service 2>/dev/null && echo "enabled" || echo "not enabled"
    systemctl --user is-active streamclone-dev.service 2>/dev/null && echo "active" || echo "inactive (run: systemctl --user start streamclone-dev.service)"
    echo "==> script permissions"
    ls -l "$ROOT/scripts/laptopworker-stack.sh"
    echo "==> smoke"
    curl -fsS "http://127.0.0.1:8090/v1/extension/health" | head -c 240
    echo
    ;;
  -h|--help|"")
    usage
    [ -n "$cmd" ] || exit 1
    ;;
  *)
    echo "Unknown command: $cmd" >&2
    usage >&2
    exit 1
    ;;
esac
