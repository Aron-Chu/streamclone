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
  clipperToken: "${VITE_CLIPPER_TOKEN:-}",
  maxRetainedMessages: "${VITE_MAX_RETAINED_MESSAGES:-250}",
  streamcloneProfile: "${STREAMCLONE_PROFILE:-core}",
  setupControlToken: "${SETUP_CONTROL_TOKEN:-}",
  devTokenImportEnabled: "${TWITCH_DEV_TOKEN_IMPORT_ENABLED:-false}"
};
EOF
