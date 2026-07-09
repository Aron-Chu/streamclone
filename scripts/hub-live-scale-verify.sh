#!/usr/bin/env bash
# Public-safe hub verification after live-chat scale deploy.
# Usage: bash scripts/hub-live-scale-verify.sh [hub_url]
set -euo pipefail

HUB_URL="${1:-https://api.streampulse.stream/v1/public/hub}"

payload="$(curl -fsS "${HUB_URL}")"

node -e "
const d = JSON.parse(process.argv[1]);
const roster = d.corpusPipeline?.roster ?? {};
const synced = (d.liveChannels ?? []).filter(c => c.coverageState === 'synced').length;
const out = {
  tieringEnabled: d.ingest?.tieringEnabled,
  activeCollectors: d.ingest?.activeCollectors,
  trackingMax: d.coverage?.trackingMax,
  poolSize: d.poolSize,
  liveRows: (d.liveChannels ?? []).length,
  synced,
  rosterLive: roster.live,
  collectorTracking: roster.collectorTracking,
  liveCollectorDeficitRows: roster.liveCollectorDeficitRows,
  collecting: roster.collecting,
  warming: roster.warming,
};
console.log(JSON.stringify(out, null, 2));
let ok = true;
if ((out.liveCollectorDeficitRows ?? 0) > 0) {
  console.error('WARN: liveCollectorDeficitRows > 0');
  ok = false;
}
if ((out.poolSize ?? 0) < 200 && (out.trackingMax ?? 0) >= 250) {
  console.error('WARN: poolSize < 200 while trackingMax >= 250 (check PUBLIC_HUB_LIVE_CAP deploy)');
  ok = false;
}
process.exit(ok ? 0 : 1);
" "${payload}"
