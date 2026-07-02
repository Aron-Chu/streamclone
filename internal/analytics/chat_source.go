package analytics

import (
	"encoding/json"
	"strings"
	"time"
)

// Chat state / source enums for stream-level chat rollup metadata.
const (
	ChatStateNone        = "none"
	ChatStateLivePartial = "live_partial"
	ChatStateIVRLite     = "ivr_lite"
	ChatStateMixedLite   = "mixed_lite"
	ChatStateGQLGold     = "gql_gold"
	ChatStateFailed      = "failed"

	ChatSourceNone  = "none"
	ChatSourceLive  = "live"
	ChatSourceIVR   = "ivr"
	ChatSourceGQL   = "gql"
	ChatSourceMixed = "mixed"

	SourceConfidenceNone        = "none"
	SourceConfidenceProvisional = "provisional"
	SourceConfidenceVerified    = "verified"
	SourceConfidenceCanonical   = "canonical"

	RollupChatSourceLive = "live"
	RollupChatSourceIVR  = "ivr"
	RollupChatSourceGQL  = "gql"

	// RollupDetailIVRPeaksOnly marks sparse top-N peak minutes — not full timeline rollups.
	RollupDetailIVRPeaksOnly = "ivr_peaks_only"
)

// sqlPublicLiveChatMinutePredicate matches isLiveChatRollup for public hub SQL aggregations.
const sqlPublicLiveChatMinutePredicate = `(
	chat_count > 0
	AND COALESCE(chat_source, '') NOT IN ('gql', 'ivr', 'mixed')
	AND (
		COALESCE(chat_source, '') = 'live'
		OR COALESCE(source_confidence, '') = 'verified'
		OR (COALESCE(chat_source, '') = '' AND viewer_samples > 0)
	)
)`

// sqlPublicLiveViewerRollupPredicate excludes corpus/import chat rows from viewer aggregates.
const sqlPublicLiveViewerRollupPredicate = `COALESCE(chat_source, '') NOT IN ('gql', 'ivr', 'mixed')`

// StreamChatSourceMetadata is persisted on analytics_streams for API honesty.
type StreamChatSourceMetadata struct {
	ChatState         string          `json:"chatState"`
	ChatSource        string          `json:"chatSource"`
	SourceConfidence  string          `json:"sourceConfidence"`
	ChatCoveragePct   float64         `json:"chatCoveragePct"`
	IVRCoveragePct    float64         `json:"ivrCoveragePct"`
	LiveCoveragePct   float64         `json:"liveCoveragePct"`
	GQLCoveragePct    float64         `json:"gqlCoveragePct"`
	MissingWindows    json.RawMessage `json:"missingWindows,omitempty"`
	SourceWindows     json.RawMessage `json:"sourceWindows,omitempty"`
	LastSourceUpgrade *time.Time      `json:"lastSourceUpgradeAt,omitempty"`
	ChatSourceDetail  string          `json:"chatSourceDetail,omitempty"`
}

// ChatSourceWindow describes one covered time range and its provenance.
type ChatSourceWindow struct {
	Source       string    `json:"source"`
	Confidence   string    `json:"confidence"`
	Start        time.Time `json:"start"`
	End          time.Time `json:"end"`
	MessageCount int       `json:"messageCount"`
	ParserErrors int       `json:"parserErrors"`
}

// ChatMissingWindow is a gap not covered by any chat rollup source.
type ChatMissingWindow struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// rollupSourcePriority returns higher = stronger write authority.
func rollupSourcePriority(confidence string) int {
	switch confidence {
	case SourceConfidenceCanonical:
		return 3
	case SourceConfidenceVerified:
		return 2
	case SourceConfidenceProvisional:
		return 1
	default:
		return 0
	}
}

// canIVROverwriteRollup returns false when existing rollup is GQL canonical.
func canIVROverwriteRollup(existingConfidence string) bool {
	return rollupSourcePriority(existingConfidence) < rollupSourcePriority(SourceConfidenceCanonical)
}

// canGQLUpgradeRollup allows GQL to replace provisional or verified rollups.
func canGQLUpgradeRollup(existingConfidence string) bool {
	return rollupSourcePriority(existingConfidence) <= rollupSourcePriority(SourceConfidenceVerified)
}

// deriveStreamChatState picks the stream-level chat_state from coverage mix.
func deriveStreamChatState(livePct, ivrPct, gqlPct float64, failed bool) string {
	if failed {
		return ChatStateFailed
	}
	if gqlPct >= 99.0 {
		return ChatStateGQLGold
	}
	if livePct > 0 && ivrPct > 0 {
		return ChatStateMixedLite
	}
	if ivrPct > 0 {
		return ChatStateIVRLite
	}
	if livePct > 0 && livePct < 95.0 {
		return ChatStateLivePartial
	}
	if livePct >= 95.0 {
		return ChatStateLivePartial
	}
	return ChatStateNone
}

// deriveStreamChatSource maps contributing sources to chat_source.
func deriveStreamChatSource(livePct, ivrPct, gqlPct float64) (source, confidence string) {
	switch {
	case gqlPct >= 99.0:
		return ChatSourceGQL, SourceConfidenceCanonical
	case livePct > 0 && ivrPct > 0:
		return ChatSourceMixed, SourceConfidenceProvisional
	case ivrPct > 0:
		return ChatSourceIVR, SourceConfidenceProvisional
	case livePct > 0:
		return ChatSourceLive, SourceConfidenceVerified
	default:
		return ChatSourceNone, SourceConfidenceNone
	}
}

// isLiveChatRollup returns true when a minute rollup came from live IRC collection.
func isLiveChatRollup(r MinuteRollup) bool {
	if r.ChatCount <= 0 {
		return false
	}
	switch r.ChatSource {
	case RollupChatSourceLive:
		return true
	case RollupChatSourceGQL, RollupChatSourceIVR, ChatSourceMixed:
		return false
	}
	if r.SourceConfidence == SourceConfidenceVerified {
		return true
	}
	// Legacy rows before migration 000050: viewer samples imply live IRC path.
	return r.ViewerSamples > 0
}

// isIVRChatRollup returns true for provisional IVR minute rollups.
func isIVRChatRollup(r MinuteRollup) bool {
	return r.ChatSource == RollupChatSourceIVR || r.SourceConfidence == SourceConfidenceProvisional
}

// IsGQLCanonicalRollup reports whether a minute rollup is GQL canonical.
func IsGQLCanonicalRollup(r MinuteRollup) bool {
	return isGQLCanonicalRollup(r)
}

// isGQLCanonicalRollup returns true for GQL canonical minute rollups.
func isGQLCanonicalRollup(r MinuteRollup) bool {
	return r.ChatSource == RollupChatSourceGQL || r.SourceConfidence == SourceConfidenceCanonical
}

// IsIVRProvisionalRollup reports provisional IVR minute rollups (excluding peaks-only detail).
func IsIVRProvisionalRollup(r MinuteRollup) bool {
	return isIVRChatRollup(r) && !isProvisionalPeakCandidateRollup(r)
}

// isProvisionalPeakCandidateRollup returns true for IVR peaks-only sparse rows.
func isProvisionalPeakCandidateRollup(r MinuteRollup) bool {
	return r.ChatSourceDetail == RollupDetailIVRPeaksOnly
}

// bulkUpsertChatSourceForRollup returns chat provenance fields for BulkUpsertMinuteRollups.
// Explicit rollup metadata wins; chat-bearing rows default to GQL canonical (historical sync path).
func bulkUpsertChatSourceForRollup(rollup MinuteRollup) (chatSource, confidence, detail string) {
	if rollup.ChatCount <= 0 {
		return "", "", ""
	}
	if strings.TrimSpace(rollup.ChatSource) != "" {
		src := rollup.ChatSource
		conf := rollup.SourceConfidence
		if conf == "" {
			switch src {
			case RollupChatSourceGQL:
				conf = SourceConfidenceCanonical
			case RollupChatSourceIVR:
				conf = SourceConfidenceProvisional
			case RollupChatSourceLive:
				conf = SourceConfidenceVerified
			}
		}
		return src, conf, rollup.ChatSourceDetail
	}
	return RollupChatSourceGQL, SourceConfidenceCanonical, rollup.ChatSourceDetail
}

// filterTimelineRollups removes peaks-only provisional rows from chart/coverage paths.
func filterTimelineRollups(rollups []MinuteRollup) []MinuteRollup {
	if len(rollups) == 0 {
		return rollups
	}
	out := make([]MinuteRollup, 0, len(rollups))
	for _, r := range rollups {
		if isProvisionalPeakCandidateRollup(r) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// provisionalPeakCandidateRollups returns peaks-only rows for explicit provisional views.
func provisionalPeakCandidateRollups(rollups []MinuteRollup) []MinuteRollup {
	if len(rollups) == 0 {
		return nil
	}
	out := make([]MinuteRollup, 0, len(rollups))
	for _, r := range rollups {
		if isProvisionalPeakCandidateRollup(r) {
			out = append(out, r)
		}
	}
	return out
}

// deriveStreamChatStateFromRollups uses minute-level source metadata when available.
func deriveStreamChatStateFromRollups(rollups []MinuteRollup, livePct, ivrPct, gqlPct float64, failed bool) string {
	if failed {
		return ChatStateFailed
	}
	hasLive, hasIVR, hasGQL := false, false, false
	for _, r := range rollups {
		if r.ChatCount <= 0 {
			continue
		}
		if isGQLCanonicalRollup(r) {
			hasGQL = true
		} else if isIVRChatRollup(r) {
			hasIVR = true
		} else if isLiveChatRollup(r) {
			hasLive = true
		}
	}
	if hasGQL && gqlPct >= 99.0 {
		return ChatStateGQLGold
	}
	if hasLive && hasIVR {
		return ChatStateMixedLite
	}
	if hasIVR || ivrPct > 0 {
		if hasLive || livePct > 0 {
			return ChatStateMixedLite
		}
		return ChatStateIVRLite
	}
	return deriveStreamChatState(livePct, ivrPct, gqlPct, failed)
}

func portalChatSourceLabel(meta StreamChatSourceMetadata) (sourceLabel, statusLabel string) {
	switch meta.ChatSource {
	case ChatSourceGQL:
		return "GQL Gold", "Canonical"
	case ChatSourceIVR:
		return "IVR accelerated", "Provisional rollups, GQL verification pending"
	case ChatSourceMixed:
		return "Mixed live + IVR", "Provisional, GQL pending"
	case ChatSourceLive:
		return "Live IRC", "Verified for tracked window"
	default:
		return "", ""
	}
}
