#!/usr/bin/env bash
# Pull latest master, resynth env, recreate core dev stack on laptopworker.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=laptopworker-env.sh
source "$ROOT/scripts/laptopworker-env.sh"
cd "$ROOT"

if ! laptopworker_required_files "$ROOT"; then
  echo "laptopworker files missing — merge laptopworker commit before update." >&2
  exit 1
fi

BRANCH="${LAPTOPWORKER_GIT_BRANCH:-master}"
REMOTE="${LAPTOPWORKER_GIT_REMOTE:-origin}"

echo "==> git fetch $REMOTE $BRANCH"
git fetch "$REMOTE" "$BRANCH"

local_sha="$(git rev-parse HEAD)"
remote_sha="$(git rev-parse "$REMOTE/$BRANCH")"
if [ "$local_sha" = "$remote_sha" ]; then
  echo "Already at $REMOTE/$BRANCH ($local_sha)"
else
  echo "Updating $local_sha -> $remote_sha"
  git merge --ff-only "$REMOTE/$BRANCH"
fi

if ! laptopworker_required_files "$ROOT"; then
  echo "Pulled commit still missing laptopworker files." >&2
  exit 1
fi

echo "==> merge .env.local (preserve user overrides)"
laptopworker_synth_env "$ROOT"

echo "==> compose up"
bash "$ROOT/scripts/laptopworker-stack.sh" start
bash "$ROOT/scripts/laptopworker-stack.sh" smoke

echo "Update complete at $(git rev-parse --short HEAD)"
