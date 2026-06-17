#!/usr/bin/env bash
# Background kubectl port-forwards for Emote Pulse Grafana / InfluxDB.
# Prefer Docker Desktop LoadBalancer (localhost) when available — no tunnel needed.
set -euo pipefail

action="${1:-}"
target="${2:-}"
namespace="${HELM_NAMESPACE:-streamclone}"
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
pid_dir="${root}/.tmp/helm"

usage() {
  cat >&2 <<EOF
Usage: $0 start|stop grafana|influx|all
EOF
  exit 1
}

resolve_target() {
  case "$1" in
    grafana)
      local_port=3000
      svc=streamclone-pulse-grafana
      svc_port=3000
      pid_file="${pid_dir}/grafana-pf.pid"
      ;;
    influx)
      if is_wsl; then
        local_port="${PULSE_INFLUX_DOCKER_PORT:-18087}"
      else
        local_port="${PULSE_INFLUX_LOCAL_PORT:-18086}"
      fi
      svc=streamclone-pulse-influxdb
      svc_port="${PULSE_INFLUX_SERVICE_PORT:-18086}"
      pid_file="${pid_dir}/influx-pf.pid"
      ;;
    *)
      echo "Unknown target: $1" >&2
      usage
      ;;
  esac
}

is_alive() {
  kill -0 "$1" 2>/dev/null
}

is_wsl() {
  [ -n "${WSL_DISTRO_NAME:-}" ] || grep -qi microsoft /proc/version 2>/dev/null
}

win_port_listener_pid() {
  local port="$1"
  powershell.exe -NoProfile -Command \
    "(Get-NetTCPConnection -LocalPort ${port} -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1).OwningProcess" \
    2>/dev/null | tr -d '\r\n[:space:]' || true
}

win_kill_port_listener() {
  local port="$1"
  powershell.exe -NoProfile -Command \
    "Get-NetTCPConnection -LocalPort ${port} -State Listen -ErrorAction SilentlyContinue | ForEach-Object { Stop-Process -Id \$_.OwningProcess -Force -ErrorAction SilentlyContinue }" \
    >/dev/null 2>&1 || true
}

loadbalancer_reachable() {
  local port="$1"
  local path="${2:-/}"
  curl_local "http://127.0.0.1:${port}${path}"
}

curl_local() {
  local url="$1"
  if is_wsl && command -v curl.exe >/dev/null 2>&1; then
    curl.exe -sf -m 3 "$url" >/dev/null 2>&1 && return 0
  fi
  curl -sf -m 3 "$url" >/dev/null 2>&1
}

# Docker Desktop + WSL: Grafana may use Windows kubectl; Influx uses WSL kubectl
# so Compose containers can reach the WSL IP (host.docker.internal often fails for :18086).
start_port_forward() {
  local bind_port="$1"
  local remote_port="$2"
  local service="$3"
  local use_wsl_kubectl="${4:-false}"

  if is_wsl && [ "$use_wsl_kubectl" != "true" ]; then
    powershell.exe -NoProfile -Command \
      "\$p = Start-Process -WindowStyle Hidden -FilePath kubectl -ArgumentList @('-n','${namespace}','port-forward','--address','0.0.0.0','svc/${service}','${bind_port}:${remote_port}') -PassThru; \$p.Id" \
      2>/dev/null | tr -d '\r\n[:space:]'
    return
  fi

  kubectl -n "$namespace" port-forward --address 0.0.0.0 "svc/${service}" "${bind_port}:${remote_port}" \
    >/dev/null 2>&1 &
}

influx_reachable_from_container() {
  local port="${PULSE_INFLUX_LOCAL_PORT:-18086}"
  if is_wsl; then
    local ip
    ip="$(wsl_primary_ip)"
    if [ -n "$ip" ] && influx_reachable_from_container_url "http://${ip}:${port}"; then
      return 0
    fi
  fi
  docker run --rm curlimages/curl:8.5.0 -sf -m 3 \
    "http://host.docker.internal:${port}/health" >/dev/null 2>&1
}

wsl_primary_ip() {
  hostname -I 2>/dev/null | awk '{print $1}'
}

influx_reachable_from_container_url() {
  local url="$1"
  docker run --rm curlimages/curl:8.5.0 -sf -m 3 \
    "${url}/health" >/dev/null 2>&1
}

grafana_reachable() {
  curl_local "http://127.0.0.1:${local_port}/login"
}

port_forward_healthy() {
  if [ "$target" = "grafana" ]; then
    grafana_reachable && return 0
  fi
  curl_local "http://127.0.0.1:${local_port}/health" \
    || curl_local "http://127.0.0.1:${local_port}/login"
}

wait_for_healthy() {
  local i
  for i in 1 2 3 4 5 6 7 8 10 12 15; do
    if port_forward_healthy; then
      return 0
    fi
    sleep 1
  done
  return 1
}

record_listener_pid() {
  if is_wsl; then
    local win_pid
    win_pid="$(win_port_listener_pid "$local_port")"
    if [[ "$win_pid" =~ ^[0-9]+$ ]]; then
      echo "$win_pid" >"$pid_file"
      return 0
    fi
    return 1
  fi
  echo "$pid" >"$pid_file"
}

stop_wsl_forward() {
  win_kill_port_listener "$local_port"
  rm -f "$pid_file"
}

start_forward() {
  # $1 is the actual service name ("grafana" or "influx"), not the script-level $target
  local svc_target="$1"
  resolve_target "$1"
  mkdir -p "$pid_dir"

  if [ "$svc_target" = "grafana" ] && loadbalancer_reachable "$local_port" "/login"; then
    rm -f "$pid_file"
    echo "Grafana: localhost:${local_port} via LoadBalancer (persistent — no port-forward)"
    return 0
  fi

  if [ "$svc_target" = "influx" ] && loadbalancer_reachable "$local_port" "/health"; then
    container_reachable=false
    if influx_reachable_from_container; then
      container_reachable=true
    fi
    wsl_container_reachable=false
    wsl_ip=""
    if is_wsl; then
      wsl_ip="$(wsl_primary_ip)"
      if [ -n "$wsl_ip" ] && influx_reachable_from_container_url "http://${wsl_ip}:${local_port}"; then
        wsl_container_reachable=true
      fi
    fi
    if [ "$container_reachable" = "true" ]; then
      rm -f "$pid_file"
      echo "Influx: localhost:${local_port} via LoadBalancer (persistent — no port-forward)"
      return 0
    fi
    if [ "$wsl_container_reachable" = "true" ]; then
      rm -f "$pid_file"
      echo "Influx: localhost:${local_port} via WSL (${wsl_ip}) reachable from Docker"
      return 0
    fi
    # LoadBalancer present on host but NOT from Docker; fall through to start 0.0.0.0 forward
    echo "Influx on :${local_port} is host-only (LoadBalancer); starting --address 0.0.0.0 forward for Docker..." >&2
  fi

  if port_forward_healthy; then
    if [ "$svc_target" = "influx" ] && ! influx_reachable_from_container; then
      echo "Influx listener on :${local_port} is not reachable from Docker; restarting with 0.0.0.0..." >&2
    else
      record_listener_pid 2>/dev/null || true
      echo "Already forwarding ${svc_target}: localhost:${local_port} reachable"
      return 0
    fi
  fi

  if is_wsl; then
    win_kill_port_listener "$local_port"
    pkill -f "port-forward.*svc/${svc}" 2>/dev/null || true
    rm -f "$pid_file"
  elif [ -f "$pid_file" ]; then
    pid="$(tr -d '[:space:]' <"$pid_file")"
    if [[ "$pid" =~ ^[0-9]+$ ]] && is_alive "$pid"; then
      kill "$pid" 2>/dev/null || true
    fi
    rm -f "$pid_file"
  fi

  if is_wsl && [ "$svc_target" = "influx" ]; then
    start_port_forward "$local_port" "$svc_port" "$svc" true
    pid=$!
  elif is_wsl; then
    started_pid="$(start_port_forward "$local_port" "$svc_port" "$svc")"
  else
    start_port_forward "$local_port" "$svc_port" "$svc"
    pid=$!
  fi

  if ! wait_for_healthy; then
    rm -f "$pid_file"
    echo "Failed to start port-forward for ${svc_target} (nothing listening on :${local_port} after 15s)" >&2
    echo "Hint: kubectl -n ${namespace} get svc,pods | grep ${svc_target}" >&2
    exit 1
  fi

  record_listener_pid || true

  if is_wsl && [ "$svc_target" = "influx" ]; then
    echo "Forwarding ${svc_target}: 0.0.0.0:${local_port} → ${svc}:${svc_port} via WSL kubectl (Docker: WSL IP)"
  elif is_wsl; then
    echo "Forwarding ${svc_target}: 0.0.0.0:${local_port} → ${svc}:${svc_port} via Windows kubectl (tunnel — dies on pod restart)"
  else
    echo "Forwarding ${svc_target}: localhost:${local_port} → ${svc}:${svc_port} (pid ${pid:-unknown})"
  fi
}

stop_forward() {
  resolve_target "$1"

  if [ "$target" = "grafana" ] && loadbalancer_reachable "$local_port" "/login"; then
    rm -f "$pid_file"
    echo "Grafana still available via LoadBalancer on :${local_port} (nothing to stop)"
    return 0
  fi
  if [ "$target" = "influx" ] && loadbalancer_reachable "$local_port" "/health"; then
    rm -f "$pid_file"
    echo "Influx still available via LoadBalancer on :${local_port} (nothing to stop)"
    return 0
  fi

  if is_wsl; then
    if win_port_listener_pid "$local_port" | grep -qE '^[0-9]+$'; then
      stop_wsl_forward
      echo "Stopped ${target} port-forward on :${local_port}"
    else
      rm -f "$pid_file"
      echo "No port-forward running for ${target}"
    fi
    return 0
  fi

  if [ ! -f "$pid_file" ]; then
    echo "No port-forward running for ${target}"
    return 0
  fi
  pid="$(tr -d '[:space:]' <"$pid_file")"
  rm -f "$pid_file"
  if [[ "$pid" =~ ^[0-9]+$ ]] && is_alive "$pid"; then
    kill "$pid" 2>/dev/null || true
    echo "Stopped ${target} port-forward (pid ${pid})"
  else
    echo "Port-forward for ${target} was not running"
  fi
}

[ $# -eq 2 ] || usage
case "$action" in
  start)
    case "$target" in
      all)
        start_forward grafana
        start_forward influx
        ;;
      grafana|influx) start_forward "$target" ;;
      *) usage ;;
    esac
    ;;
  stop)
    case "$target" in
      all)
        stop_forward grafana
        stop_forward influx
        ;;
      grafana|influx) stop_forward "$target" ;;
      *) usage ;;
    esac
    ;;
  *) usage ;;
esac
