#!/usr/bin/env bash
# LOAD-001 stub: synthetic multi-channel Pulse load harness (local/dev only).
# Full implementation: 25 channels, tiered message rates, one concurrent backfill, PG/redis metrics.
# Blocked for cap-25 until CAP-001 soak passes.
set -euo pipefail

echo "LOAD-001: synthetic harness not yet implemented."
echo "Planned checks: memory <85%, PG write p95 <250ms, rollup flush p95 <5s, BFF hit/miss p95."
echo "See docs/website-portal/tasks.md CAP-001 before raising PULSE_MAX_ACTIVE_CHANNELS."
exit 0
