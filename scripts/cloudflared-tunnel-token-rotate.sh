#!/usr/bin/env bash
# Rotate a Cloudflare tunnel connector token and reinstall cloudflared on the target host.
# The connector token is never printed. Requires CLOUDFLARE_API_TOKEN and SSH access.
set -euo pipefail

: "${CLOUDFLARE_API_TOKEN:?CLOUDFLARE_API_TOKEN required}"

ACCOUNT_ID="${CF_ACCOUNT_ID:-}"
TUNNEL_NAME="${CF_TUNNEL_NAME:-streampulse-bearhost}"
VPS_HOST="${VPS_HOST:-root@23.173.152.156}"
VPS_KEY="${VPS_SSH_KEY:-$HOME/.ssh/id_ed25519}"
HEALTH_URL="${PULSE_SMOKE_BASE_URL:-https://api.streampulse.stream}/v1/extension/health"

if [[ ! -f "${VPS_KEY}" ]]; then
  echo "ERROR: VPS_SSH_KEY does not exist: ${VPS_KEY}" >&2
  exit 1
fi

if [[ -z "${ACCOUNT_ID}" ]]; then
  ACCOUNT_ID="$(curl -fsS -H "Authorization: Bearer ${CLOUDFLARE_API_TOKEN}" \
    "https://api.cloudflare.com/client/v4/accounts" \
    | python3 -c "import sys,json; r=json.load(sys.stdin).get('result') or []; assert r, 'no Cloudflare accounts visible'; print(r[0]['id'])")"
fi

echo "==> Cloudflare account ${ACCOUNT_ID}"
TUNNEL_JSON="$(curl -fsS -G -H "Authorization: Bearer ${CLOUDFLARE_API_TOKEN}" \
  --data-urlencode "name=${TUNNEL_NAME}" \
  "https://api.cloudflare.com/client/v4/accounts/${ACCOUNT_ID}/cfd_tunnel")"
TUNNEL_ID="$(python3 -c "import json,sys; d=json.loads(sys.argv[1]); res=d.get('result') or []; assert res, 'tunnel not found'; print(res[0]['id'])" "${TUNNEL_JSON}")"
echo "==> tunnel id=${TUNNEL_ID} name=${TUNNEL_NAME}"

echo "==> issue new connector token (not printed)"
TOKEN_RESP="$(curl -fsS -X POST -H "Authorization: Bearer ${CLOUDFLARE_API_TOKEN}" \
  -H "Content-Type: application/json" \
  "https://api.cloudflare.com/client/v4/accounts/${ACCOUNT_ID}/cfd_tunnel/${TUNNEL_ID}/token")"
NEW_TOKEN="$(python3 -c "import json,sys; d=json.loads(sys.argv[1]); assert d.get('success'), 'token issue failed'; print(d['result'])" "${TOKEN_RESP}")"

TMP_TOKEN="$(mktemp)"
cleanup() {
  rm -f "${TMP_TOKEN}"
}
trap cleanup EXIT
chmod 600 "${TMP_TOKEN}"
printf '%s' "${NEW_TOKEN}" > "${TMP_TOKEN}"

echo "==> reinstall cloudflared on ${VPS_HOST}"
scp -i "${VPS_KEY}" -o StrictHostKeyChecking=accept-new "${TMP_TOKEN}" "${VPS_HOST}:/tmp/cloudflared-token.new"
ssh -i "${VPS_KEY}" -o StrictHostKeyChecking=accept-new "${VPS_HOST}" bash -s <<'REMOTE'
set -euo pipefail
TOKEN="$(tr -d '\r\n' < /tmp/cloudflared-token.new)"
rm -f /tmp/cloudflared-token.new
if systemctl is-active cloudflared >/dev/null 2>&1; then
  systemctl stop cloudflared
fi
cloudflared service uninstall >/dev/null 2>&1 || true
cloudflared service install "${TOKEN}"
systemctl enable cloudflared
systemctl restart cloudflared
systemctl is-active cloudflared
pgrep -af cloudflared | sed 's/tunnel run [^ ]*/tunnel run [REDACTED]/'
REMOTE

echo "==> hosted health probe"
curl -fsS -o /dev/null -w "hosted-health=%{http_code} time=%{time_total}s\n" "${HEALTH_URL}"
echo "==> rotation complete"
