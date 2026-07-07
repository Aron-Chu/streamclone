# Roster naming truth table (Pulse / Streamclone)

**Read this first** when an agent sees `top500` in a filename, env var, or JSON field. The number **500 is not fixed** — it is legacy naming from an early corpus tier.

Related:

- [live-coverage-requirements.md](../../streamclone-pulse/docs/pulse-extension/live-coverage-requirements.md)
- [top-roster-awareness-requirements.md](top-roster-awareness-requirements.md)
- [ops-migration-truth-table.md](../ops-migration-truth-table.md)

---

## One-line summary

| Question | Answer |
|----------|--------|
| Does `top500` mean exactly 500 channels? | **No.** Check `LIVE_ADMISSION_TOP_N`, `PULSE_MAX_ACTIVE_CHANNELS`, and `CORPUS_TARGET_TOP_N`. |
| What is the canonical name for IRC cap admission? | **`live_admission`** / **`PULSE_LIVE_ADMISSION_*`** env |
| What is legacy but still valid in ops overlays? | **`PULSE_TOP500_ADMISSION_*`**, **`PULSE_TOP_ROSTER_*`** |
| Extension JSON: `top500Eligible` or `rosterEligible`? | **`rosterEligible`** is canonical; **`top500Eligible`** is deprecated dual-emit |
| Can I rename DB tables `top500_*`? | **No** — applied migrations stay as-is |

---

## Three roster domains (do not conflate)

| Domain | Purpose | Typical N (hosted) | Canonical code / env | Legacy names |
|--------|---------|-------------------|----------------------|--------------|
| **Live IRC admission** | Helix top-live → IRC join for cap-tier live chat | 250 (`LIVE_ADMISSION_TOP_N`) | `LiveAdmissionPoller`, `PULSE_LIVE_ADMISSION_*` | `Top500PriorityWatchPoller`, `PULSE_TOP500_ADMISSION_*` |
| **Extension eligibility** | BFF: channel on Pulse roster / in cap tier | IRC cap | `rosterEligible` JSON, `extensionRosterEligible()` | `top500Eligible`, `extensionTop500Eligible()` |
| **Corpus metadata roster** | Metadata sampling, silver/gold gates (not IRC chat) | 100–1000 (`CORPUS_TARGET_TOP_N`) | `Top500MetadataSampler` (Phase 4 rename deferred) | `TOP500_METADATA_*` env |

**IRC cap ≠ metadata corpus size.** Hosted may run `PULSE_MAX_ACTIVE_CHANNELS=250` with `CORPUS_TARGET_TOP_N=1000`.

---

## Env var map (admission)

| Canonical (preferred in new docs) | Legacy (still read) |
|-----------------------------------|---------------------|
| `PULSE_LIVE_ADMISSION_ENABLED` | `PULSE_TOP500_ADMISSION_ENABLED`, `PULSE_TOP_ROSTER_ADMISSION_ENABLED`, `PULSE_TOP_ROSTER_POLL_ENABLED` |
| `PULSE_LIVE_ADMISSION_TOP_N` | `PULSE_TOP500_ADMISSION_TOP_N`, `PULSE_TOP_ROSTER_ADMISSION_TOP_N`, `LIVE_ADMISSION_TOP_N` |
| `PULSE_LIVE_ADMISSION_INTERVAL` | `PULSE_TOP500_ADMISSION_INTERVAL`, `PULSE_TOP_ROSTER_ADMISSION_INTERVAL` |
| `PULSE_LIVE_ADMISSION_SOURCE` | `PULSE_TOP500_ADMISSION_SOURCE` (`helix_top_live` \| `roster`) |
| `PULSE_LIVE_ADMISSION_MISS_GRACE_CYCLES` | `PULSE_TOP500_ADMISSION_MISS_GRACE_CYCLES`, `PULSE_TOP_ROSTER_*` |

Go struct fields: **`PulseLiveAdmission*`** (canonical). **`PulseTop500Admission*`** type aliases remain one release for compile compat in external importers only if needed.

---

## File name map (IRC admission — Phase 1)

| Legacy file | Canonical file |
|-------------|----------------|
| `top500_priority_watch.go` | `live_admission_poller.go` |
| `top500_live_admission.go` | `live_admission_source.go` |
| `top500_admission_outcomes.go` | `live_admission_outcomes.go` |
| `top500_admission_state.go` | `live_admission_state.go` |
| `internal/metrics/top500_admission.go` | `live_admission_metrics.go` |

Corpus metadata files (`top500_metadata_*.go`, migrations `000044_top500_metadata`) — **unchanged** until Phase 4.

---

## Coverage tier strings

| Canonical | Legacy (still emitted) |
|-----------|------------------------|
| `roster_metadata_only` | `top500_metadata_only` |
| `active_live_coverage` | (unchanged) |

---

## Agent guardrails

1. Before changing admission top-N, read **`PULSE_MAX_ACTIVE_CHANNELS`** — admission cannot exceed IRC cap.
2. Do not assume **`top500Eligible: false`** means “not in top 500” — it means **not on hosted Pulse roster / cap tier**.
3. Prefer **`live_admission`** / **`roster`** in new docs, commits, and log messages; keep legacy env keys in **ops overlays** until streampulse-ops migrates explicitly.
