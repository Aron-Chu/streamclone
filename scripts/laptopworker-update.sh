#!/usr/bin/env bash
# Pull latest master, resynth env, rebuild/recreate when relevant paths changed.
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

old_sha="$(git rev-parse HEAD)"
remote_sha="$(git rev-parse "$REMOTE/$BRANCH")"

if [ "$old_sha" = "$remote_sha" ]; then
  echo "Already at $REMOTE/$BRANCH ($(git rev-parse --short HEAD))"
else
  echo "Updating $(git rev-parse --short "$old_sha") -> $(git rev-parse --short "$remote_sha")"
  git merge --ff-only "$REMOTE/$BRANCH"
fi

new_sha="$(git rev-parse HEAD)"

if ! laptopworker_required_files "$ROOT"; then
  echo "Pulled commit still missing laptopworker files." >&2
  exit 1
fi

laptopworker_ensure_scripts_executable "$ROOT"

echo "==> merge .env.local (preserve user overrides)"
laptopworker_synth_env "$ROOT"

laptopworker_plan_update "$ROOT" "$old_sha" "$new_sha"

echo "==> compose plan (old=$(git rev-parse --short "$old_sha") new=$(git rev-parse --short "$new_sha"))"
if [ "$LW_RUN_MIGRATE" = 1 ]; then
  laptopworker_run_migrate "$ROOT"
fi

if [ "$LW_FULL_COMPOSE" = 1 ]; then
  echo "    compose files changed — full up --build"
  laptopworker_compose_up "$ROOT" --build
elif [ "${#LW_BUILD[@]}" -gt 0 ]; then
  echo "    rebuild: ${LW_BUILD[*]}"
  laptopworker_compose_up "$ROOT" --build "${LW_BUILD[@]}"
elif [ "$old_sha" != "$new_sha" ]; then
  echo "    refresh compose (no image rebuild)"
  laptopworker_compose_up "$ROOT"
else
  echo "    git unchanged — refresh compose state"
  laptopworker_compose_up "$ROOT"
fi

if [ "${#LW_RECREATE[@]}" -gt 0 ]; then
  echo "    recreate: ${LW_RECREATE[*]}"
  laptopworker_compose "$ROOT" up -d --no-deps --force-recreate "${LW_RECREATE[@]}"
fi

laptopworker_compose "$ROOT" ps

echo "==> smoke"
bash "$ROOT/scripts/laptopworker-stack.sh" smoke

echo "Update complete at $(git rev-parse --short HEAD)"
