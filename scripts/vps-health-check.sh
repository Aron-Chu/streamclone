#!/usr/bin/env bash
grep STREAMCLONE_VERSION /opt/streamclone/app/.env /opt/streamclone/app/deploy/env/profile-bearhost-pulse.env 2>/dev/null || true
docker inspect streamclone-analytics-1 --format '{{range .Config.Env}}{{println .}}{{end}}' | grep -E 'STREAMCLONE|PULSE_MAX|PULSE_HOSTED' || true
curl -sf http://127.0.0.1:8090/v1/extension/health
