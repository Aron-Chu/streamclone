#!/bin/sh
set -eu

cat >/usr/share/nginx/html/config.js <<EOF
window.__STREAMCLONE_CONFIG__ = {
  metadataUrl: "${VITE_METADATA_URL:-auto}",
  videoUrl: "${VITE_VIDEO_URL:-auto}",
  emoteUrl: "${VITE_EMOTE_URL:-auto}",
  analyticsUrl: "${VITE_ANALYTICS_URL:-auto}",
  chatWs: "${VITE_CHAT_WS:-auto}",
  chatHttp: "${VITE_CHAT_HTTP:-auto}",
  clipperUrl: "${VITE_CLIPPER_URL:-auto}",
  replayforgeUiUrl: "${VITE_REPLAYFORGE_UI_URL:-http://localhost:8096}",
  maxRetainedMessages: "${VITE_MAX_RETAINED_MESSAGES:-250}",
  streamcloneProfile: "${STREAMCLONE_PROFILE:-core}",
  setupControlToken: "${SETUP_CONTROL_TOKEN:-}",
  setupControlWakeEnabled: "${SETUP_CONTROL_WAKE_ENABLED:-false}",
  devTokenImportEnabled: "${TWITCH_DEV_TOKEN_IMPORT_ENABLED:-false}",
  installId: "${STREAMCLONE_INSTALL_ID:-}",
  hlsLowLatencyEnabled: "${VITE_HLS_LOW_LATENCY_ENABLED:-false}",
  adaptiveLiveLatencyEnabled: "${VITE_ADAPTIVE_LIVE_LATENCY_ENABLED:-false}"
};
EOF
