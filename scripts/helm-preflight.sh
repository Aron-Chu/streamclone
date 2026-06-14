#!/usr/bin/env bash
# Verify kubectl/helm can reach a cluster before helm-up.
set -euo pipefail

win_user="$(
  powershell.exe -NoProfile -Command '[Environment]::UserName' 2>/dev/null \
    | tr -d '\r' \
    | tr '[:upper:]' '[:lower:]'
)"
win_kube="/mnt/c/Users/${win_user}/.kube/config"
wsl_kube="${HOME}/.kube/config"

if [ -z "${KUBECONFIG:-}" ] && [ ! -f "${wsl_kube}" ] && [ -f "${win_kube}" ]; then
  mkdir -p "${HOME}/.kube"
  ln -sf "${win_kube}" "${wsl_kube}"
  echo "Linked WSL kubeconfig → ${win_kube}"
fi

if [ -z "${KUBECONFIG:-}" ] && [ ! -f "${wsl_kube}" ] && [ ! -f "${win_kube}" ]; then
  cat >&2 <<EOF
No Kubernetes kubeconfig found.

Docker Desktop (recommended):
  1. Open Docker Desktop → Settings → Kubernetes
  2. Enable "Kubernetes" and wait until it shows Running (green)
  3. From WSL, run: make helm-kubeconfig
  4. Retry: make helm-up

Or set KUBECONFIG to your cluster config and ensure kubectl cluster-info works.
EOF
  exit 1
fi

context="$(kubectl config current-context 2>/dev/null || true)"

if [ -z "${context}" ]; then
  cat >&2 <<EOF
kubectl has no current context.

If using Docker Desktop, enable Kubernetes in Settings, then:
  make helm-kubeconfig
  make helm-up
EOF
  exit 1
fi

if ! kubectl cluster-info >/dev/null 2>&1; then
  err="$(kubectl cluster-info 2>&1 | tail -1 || true)"
  cat >&2 <<EOF
Kubernetes cluster unreachable (context: ${context}).

${err}

Docker Desktop: confirm Kubernetes is Running in Settings, then retry make helm-up.
EOF
  exit 1
fi

echo "Kubernetes ok (context: ${context})"
