#!/usr/bin/env bash
source "$(dirname "$0")/lib/bearhost-ssh.sh"
bearhost_ssh_config
bearhost_ssh "test -f /opt/streamclone/app/deploy/env/profile-bearhost-corpus.env && cat /opt/streamclone/app/deploy/env/profile-bearhost-corpus.env | head -8; echo '--- container env ---'; docker exec streamclone-analytics-workers env | grep -E '^(SILVER|BRONZE|BACKFILL|CORPUS)' | sort"
