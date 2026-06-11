#!/usr/bin/env bash
set -euo pipefail
pip3 install --user -q git-filter-repo 2>/dev/null || sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -qq git-filter-repo
TMP="/tmp/streamclone-purge-test-$$"
git clone --quiet file:///mnt/c/Users/Aron/testclone/streamclone "$TMP"
cd "$TMP"
git filter-repo --force \
  --path tmp-metadata-app.bin --invert-paths \
  --path tmp-metadata-app-current.bin --invert-paths
echo "--- verify blobs gone ---"
if git rev-list --objects --all | grep -q tmp-metadata; then
  echo "FAIL: tmp-metadata still present"
  exit 1
fi
echo "OK: tmp-metadata binaries purged from test clone history"
rm -rf "$TMP"
