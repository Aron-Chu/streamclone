# Admission top-N config clamp fix (2026-07)

## Symptom

VPS env had `PULSE_TOP500_ADMISSION_TOP_N=5000` and `PULSE_MAX_ACTIVE_CHANNELS=5000`, but public hub showed `liveAdmissionTopN: 100` and IRC fill stuck ~115.

## Root cause

`config.Load()` tied `PulseTop500AdmissionTopN` validation to `maxCorpusTopN` (1000). Values above 1000 were **reset to 100**, not clamped to corpus max. Docker `printenv` still showed 5000; runtime admission poller used 100.

Secondary: `corpus_readiness.go` capped hub/readiness `LiveAdmissionTopN` at `MaxTop500MetadataTopN` (1000).

## Fix

- `ClampLiveAdmissionTopN` in `internal/config/config.go` with `MaxLiveAdmissionTopN=5000`, separate from corpus metadata caps.
- Admission clamped to `PulseMaxActiveChannels` when set (cannot admit more live IRC candidates than slot ceiling).
- Hub/readiness uses same helper; metadata `TargetTopN` stays capped at 1000.

## Ship

Tag `v0.3.0-rc15` — analytics-only redeploy; env block unchanged on VPS.
