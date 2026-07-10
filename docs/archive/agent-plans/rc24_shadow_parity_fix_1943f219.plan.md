> **HISTORICAL (archived from .cursor/plans).** Not product law. Do not use for routing analytics, ingest, hub, ops, or Pulse work into public Streamclone. See docs/archive/agent-plans/README.md and docs/streampulse-product-boundary.md.
---
name: rc24 shadow parity fix
overview: "APPROVED. Fix ingest-core closed-minute rollup parity (rc24): explicit AddChatMessage/AddEmote ring API, emote occurrence parity, extended closed chat+emote gate metrics, shadow-only deploy on xqc,ludwig, closed-only debug gate (100 samples). Phase D NO-GO until 1000-sample production gate."
todos:
  - id: rc24-compare-diagnostics
    content: "Extend compare + mismatch-report: 10 closed mismatches, adjacent-minute, chat/emote/combined match rates, top_emote_key_mismatch_count"
    status: completed
  - id: rc24-count-parity
    content: ring.go AddChatMessage + AddEmote; aggregate one chat per PRIVMSG, emote per occurrence (not deduped)
    status: completed
  - id: rc24-debug-tests
    content: "Tests: 8 emotes, plain text, 5x same emote, minute boundary, closed key match; optional INGEST_SHADOW_DEBUG counters"
    status: completed
  - id: rc24-deploy-vps
    content: Tag rc24, rotate artifact, production-up --no-deps analytics (xqc,ludwig), limits guard PASS
    status: completed
  - id: rc24-regate
    content: Soak 30-60m, closed-only debug gate min 100; 06-rc24-parity before/after mismatch tables; PHASE_D GO_NOGO
    status: completed
isProject: false
---

# rc24 ingest-core shadow parity fix

**Status: APPROVED** (with amendments below). **Phase D: NO-GO** until full 1000-sample closed gate passes.

## Decision (locked)

- **Stop soaking on rc23** — closed keys intersect, 61 closed records, **0% closed match** (after `key.Closed` casing fix).
- **rc24 parity workstream** — issue is ingest-core rollup semantics, not deploy safety or compare policy alone.
- Example: same closed key, legacy chat **23**, shadow chat **31** → shadow over-counting (likely per-emote `AddChat` loop).

**Hard stops:** `INGEST_CORE_ENABLED=0`, dual+shadow on, legacy PG writer, allowlist **`xqc,ludwig` only**, limits guard before gates, `production-up.sh --no-deps analytics` only. **No allowlist expansion until closed-only debug gate ≥99% on ≥100 closed samples.**

**Supersedes:** [Shadow Compare Diagnostics](c:/Users/Aron/twitch-7tv-clone/.cursor/plans/shadow_compare_diagnosis_867490e3.plan.md) (COMPLETE).

---

## Target semantics (approved)

```text
PRIVMSG accepted → ChatCount +1 once
each emote occurrence → TotalEmoteCount +1 (preserve repeats, not unique-key dedupe)
chat-only message → ChatCount +1, TotalEmoteCount +0, TopEmotes empty
```

---

## Root cause hypothesis (code-confirmed)

Legacy: one `chatCount++` per PRIVMSG, emotes from fragments only ([`collector.addChat`](c:/Users/Aron/twitch-7tv-clone/internal/analytics/collector.go)).

Shadow bug: `ring.AddChat` called **once per emote key**; `AddChat` increments **both** `ChatCount` and `TotalEmoteCount` even for chat-only path ([`aggregate.go`](c:/Users/Aron/twitch-7tv-clone/internal/analytics/ingestcore/aggregate.go), [`ring.go`](c:/Users/Aron/twitch-7tv-clone/internal/analytics/ingestcore/ring.go)).

**Verify:** `EmoteKeysFromFragments` must preserve **occurrences** (same emote 5× → 5 keys), not dedupe.

---

## Phase 1 — Extended mismatch diagnostics (scripts)

Enhance [`scripts/ingest-shadow-compare.sh`](c:/Users/Aron/twitch-7tv-clone/scripts/ingest-shadow-compare.sh) and add [`scripts/ingest-shadow-mismatch-report.sh`](c:/Users/Aron/twitch-7tv-clone/scripts/ingest-shadow-mismatch-report.sh):

**Required rc24 gate output (closed records only):**

```text
closed_chat_match_rate
closed_total_emote_match_rate
closed_both_chat_and_emote_match_rate
top_emote_key_mismatch_count
gate_total / gate_match (chat tolerance gate unchanged at 2%)
```

- First **10 closed mismatches** table: key, legacy vs shadow chat/emote/viewer, deltas, diff pct, `recordedAt`
- **Adjacent-minute check** (prev/next) for same stream+channel — classify `over_count`, `under_count`, `boundary_shift_suspect`, `viewer_only`
- Emote parity is **required debug signal** on every gate run (not optional). First rc24 debug pass gates on **chat** ≥99%; emote deltas must be reported clearly. Phase D later requires emote parity or documented harmless delta.

Wire [`scripts/ingest-phase-c-gates.sh`](c:/Users/Aron/twitch-7tv-clone/scripts/ingest-phase-c-gates.sh) to emit all metrics to evidence dir.

---

## Phase 2 — rc24 core parity patch (streamclone)

### 2a. Explicit ring API (no ambiguous combined method)

Refactor [`internal/analytics/ingestcore/ring.go`](c:/Users/Aron/twitch-7tv-clone/internal/analytics/ingestcore/ring.go):

```go
ring.AddChatMessage(timestamp)           // ChatCount +1 only
ring.AddEmote(timestamp, key, isSevenTV) // TotalEmoteCount +1 per occurrence; map[key]++
```

Remove or deprecate combined `AddChat` that mutates multiple counters invisibly.

Refactor [`aggregate.go`](c:/Users/Aron/twitch-7tv-clone/internal/analytics/ingestcore/aggregate.go) `shardWorker.process`:

1. `AddChatMessage` once per accepted PRIVMSG
2. `AddEmote` for **each emote occurrence** in fragments/keys (same order as legacy fragment loop)
3. Plain text: `AddChatMessage` only — **no** emote increment

### 2b. Parser / feed parity (verify only)

[`parse.ParseLine`](c:/Users/Aron/twitch-7tv-clone/internal/chat/parse/parse.go) — PRIVMSG only. Single IRC callback in [`cmd/analytics/main.go`](c:/Users/Aron/twitch-7tv-clone/cmd/analytics/main.go) — no duplicate feed.

### 2c. Optional debug counters (`INGEST_SHADOW_DEBUG=1`)

Allowlist channels only; off by default. Raw lines, parsed PRIVMSG, counted messages, parse failures, missing bind, duplicate hash, non-PRIVMSG ignored.

### 2d. Shadow compare tolerance

Keep [`withinTolerance`](c:/Users/Aron/twitch-7tv-clone/internal/analytics/ingestcore/shadow.go) chat-focused for pass/fail; emote metrics reported separately via gate scripts.

---

## Phase 3 — Tests (required)

| Test | Asserts |
|------|---------|
| `TestRing_oneChatPerMessage_multiEmote` | 8 emotes → `ChatCount==1`, `TotalEmoteCount==8` |
| `TestRing_plainTextNoEmotes` | plain PRIVMSG → `ChatCount==1`, `TotalEmoteCount==0`, top emotes empty |
| `TestRing_repeatedSameEmote` | same emote 5× → `TotalEmoteCount==5`, map[key]==5, `ChatCount==1` |
| `TestEmoteKeysFromFragments_preservesOccurrences` | dedupe must NOT collapse repeats |
| `TestMinuteBucket_messageTimestamp` | 12:00:59 vs 12:01:00 correct buckets |
| `TestShadowComparer_closedMinuteMatch` | aligned snapshots → `match=true` |

Run: `go test ./internal/analytics/ingestcore/...`

---

## Phase 4 — Release rc24 + VPS deploy (shadow only)

1. Tag **`v0.3.0-rc24`** after CI pass
2. VPS: `IMAGE_TAG=v0.3.0-rc24`, allowlist `xqc,ludwig`, dual+shadow, `INGEST_CORE_ENABLED=0`
3. **`production-up.sh --no-deps analytics`** + limits guard PASS
4. Rotate artifact: `latest.jsonl` → `latest-rc23.jsonl` for clean rc24 window
5. Soak **30–60m** (2 channels only)

---

## Phase 5 — Re-gate + evidence (`06-rc24-parity/`)

**Required evidence artifacts:**

```text
before-mismatch-sample.md   # rc23 example: legacy 23 / shadow 31 closed minute
after-mismatch-report.txt   # first 10 closed mismatches post-rc24 (comparable table)
closed_chat_match_rate
closed_total_emote_match_rate
closed_both_chat_and_emote_match_rate
PHASE_D_GO_NOGO
```

```bash
bash scripts/ingest-phase-c-gates.sh 2 100 docs/.../06-rc24-parity/03-gates-debug
```

**Debug pass:** closed chat ≥99% on ≥100 closed samples → **then** expand allowlist top 50.

**Production pass:** 1000 closed samples ≥99% → `03-gates-final/` → Phase D discussion.

**PHASE_D_GO_NOGO: NO-GO** until production gate passes.

---

## Decision tree after rc24

| Outcome | Next |
|---------|------|
| Closed chat ≥99%, emote metrics reported | Expand allowlist top 50 |
| Chat improved, emote still off | Document delta; fix emote path before Phase D |
| Chat still &lt;99% | Mismatch report + debug counters |
| Shadow still higher | duplicate feed / boundary audit |

---

## Files to change

| Repo | Files |
|------|-------|
| **streamclone** | `ring.go`, `aggregate.go`, `parse.go` (verify), `shadow_test.go`, `ring_test.go`; compare/mismatch/gates scripts; runbook |
| **streampulse-ops** | `docs/deployments/.../06-rc24-parity/` |
