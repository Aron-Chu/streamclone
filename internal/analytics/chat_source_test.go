package analytics

import (
	"testing"
	"time"
)

func TestCanIVROverwriteRollup(t *testing.T) {
	if !canIVROverwriteRollup("") {
		t.Fatal("empty confidence should allow IVR")
	}
	if !canIVROverwriteRollup(SourceConfidenceProvisional) {
		t.Fatal("provisional should allow IVR")
	}
	if !canIVROverwriteRollup(SourceConfidenceVerified) {
		t.Fatal("verified should allow IVR gap fill")
	}
	if canIVROverwriteRollup(SourceConfidenceCanonical) {
		t.Fatal("canonical GQL must block IVR overwrite")
	}
}

func TestCanGQLUpgradeRollup(t *testing.T) {
	if !canGQLUpgradeRollup(SourceConfidenceProvisional) {
		t.Fatal("GQL should upgrade provisional")
	}
	if !canGQLUpgradeRollup(SourceConfidenceVerified) {
		t.Fatal("GQL should upgrade verified")
	}
	if canGQLUpgradeRollup(SourceConfidenceCanonical) {
		t.Fatal("GQL should not downgrade canonical")
	}
}

func TestDeriveStreamChatState(t *testing.T) {
	if got := deriveStreamChatState(0, 0, 100, false); got != ChatStateGQLGold {
		t.Fatalf("gql gold: got %q", got)
	}
	if got := deriveStreamChatState(40, 50, 0, false); got != ChatStateMixedLite {
		t.Fatalf("mixed: got %q", got)
	}
	if got := deriveStreamChatState(0, 80, 0, false); got != ChatStateIVRLite {
		t.Fatalf("ivr lite: got %q", got)
	}
	if got := deriveStreamChatState(0, 0, 0, true); got != ChatStateFailed {
		t.Fatalf("failed: got %q", got)
	}
}

func TestDeriveStreamChatSource(t *testing.T) {
	src, conf := deriveStreamChatSource(0, 0, 100)
	if src != ChatSourceGQL || conf != SourceConfidenceCanonical {
		t.Fatalf("gql: src=%q conf=%q", src, conf)
	}
	src, conf = deriveStreamChatSource(30, 40, 0)
	if src != ChatSourceMixed || conf != SourceConfidenceProvisional {
		t.Fatalf("mixed: src=%q conf=%q", src, conf)
	}
}

func TestPortalChatSourceLabel(t *testing.T) {
	label, status := portalChatSourceLabel(StreamChatSourceMetadata{ChatSource: ChatSourceIVR})
	if label != "IVR accelerated" || status == "" {
		t.Fatalf("ivr label=%q status=%q", label, status)
	}
}

func TestIsLiveChatRollup(t *testing.T) {
	if !isLiveChatRollup(MinuteRollup{ChatCount: 5, ChatSource: RollupChatSourceLive}) {
		t.Fatal("live source should be live rollup")
	}
	if !isLiveChatRollup(MinuteRollup{ChatCount: 5, SourceConfidence: SourceConfidenceVerified}) {
		t.Fatal("verified confidence should be live rollup")
	}
	if isLiveChatRollup(MinuteRollup{ChatCount: 5, ChatSource: RollupChatSourceGQL, SourceConfidence: SourceConfidenceCanonical}) {
		t.Fatal("gql canonical must not be live")
	}
	if !isLiveChatRollup(MinuteRollup{ChatCount: 3, ViewerSamples: 10}) {
		t.Fatal("legacy viewer_samples should imply live")
	}
}

func TestBulkUpsertChatSourceForRollup(t *testing.T) {
	src, conf, _ := bulkUpsertChatSourceForRollup(MinuteRollup{ChatCount: 12})
	if src != RollupChatSourceGQL || conf != SourceConfidenceCanonical {
		t.Fatalf("chat default = %q/%q, want gql/canonical", src, conf)
	}
	src, conf, _ = bulkUpsertChatSourceForRollup(MinuteRollup{ViewerSamples: 3})
	if src != "" || conf != "" {
		t.Fatalf("viewer-only = %q/%q, want empty", src, conf)
	}
	src, conf, _ = bulkUpsertChatSourceForRollup(MinuteRollup{
		ChatCount: 5, ChatSource: RollupChatSourceLive,
	})
	if src != RollupChatSourceLive || conf != SourceConfidenceVerified {
		t.Fatalf("explicit live = %q/%q", src, conf)
	}
}

func TestIsIVRAndGQLRollup(t *testing.T) {
	if !isIVRChatRollup(MinuteRollup{ChatSource: RollupChatSourceIVR}) {
		t.Fatal("ivr source")
	}
	if !isGQLCanonicalRollup(MinuteRollup{ChatSource: RollupChatSourceGQL, SourceConfidence: SourceConfidenceCanonical}) {
		t.Fatal("gql canonical")
	}
	if isIVRChatRollup(MinuteRollup{ChatSource: RollupChatSourceGQL, SourceConfidence: SourceConfidenceCanonical}) {
		t.Fatal("gql is not ivr")
	}
}

func TestDeriveStreamChatStateFromRollups(t *testing.T) {
	start := time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC)
	gqlRollups := []MinuteRollup{{
		MinuteTS: start, ChatCount: 10,
		ChatSource: RollupChatSourceGQL, SourceConfidence: SourceConfidenceCanonical,
	}}
	if got := deriveStreamChatStateFromRollups(gqlRollups, 0, 0, 100, false); got != ChatStateGQLGold {
		t.Fatalf("gql canonical rollups: got %q", got)
	}
	mixed := []MinuteRollup{
		{MinuteTS: start, ChatCount: 5, ChatSource: RollupChatSourceLive, SourceConfidence: SourceConfidenceVerified},
		{MinuteTS: start.Add(time.Minute), ChatCount: 8, ChatSource: RollupChatSourceIVR, SourceConfidence: SourceConfidenceProvisional},
	}
	if got := deriveStreamChatStateFromRollups(mixed, 40, 50, 0, false); got != ChatStateMixedLite {
		t.Fatalf("live+ivr mixed: got %q", got)
	}
	ivrOnly := []MinuteRollup{{MinuteTS: start, ChatCount: 4, ChatSource: RollupChatSourceIVR, SourceConfidence: SourceConfidenceProvisional}}
	if got := deriveStreamChatStateFromRollups(ivrOnly, 0, 80, 0, false); got != ChatStateIVRLite {
		t.Fatalf("ivr only: got %q", got)
	}
	legacy := []MinuteRollup{{MinuteTS: start, ChatCount: 2, ViewerSamples: 5}}
	if got := deriveStreamChatStateFromRollups(legacy, 50, 0, 0, false); got != ChatStateLivePartial {
		t.Fatalf("legacy live via viewer_samples: got %q", got)
	}
}

func TestFilterTimelineRollupsExcludesPeaksOnly(t *testing.T) {
	start := time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC)
	rollups := []MinuteRollup{
		{MinuteTS: start, ChatCount: 100, ChatSource: RollupChatSourceGQL, SourceConfidence: SourceConfidenceCanonical},
		{MinuteTS: start.Add(5 * time.Minute), ChatCount: 80, ChatSource: RollupChatSourceIVR, SourceConfidence: SourceConfidenceProvisional, ChatSourceDetail: RollupDetailIVRPeaksOnly},
		{MinuteTS: start.Add(10 * time.Minute), ChatCount: 60, ChatSource: RollupChatSourceIVR, SourceConfidence: SourceConfidenceProvisional},
	}
	filtered := filterTimelineRollups(rollups)
	if len(filtered) != 2 {
		t.Fatalf("timeline rollups = %d, want 2 (peaks-only excluded)", len(filtered))
	}
	peaks := provisionalPeakCandidateRollups(rollups)
	if len(peaks) != 1 || peaks[0].ChatCount != 80 {
		t.Fatalf("provisional peaks = %+v", peaks)
	}
}

func TestSummarizeStreamMetricsIgnoresPeaksOnlyCoverage(t *testing.T) {
	start := time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC)
	end := start.Add(120 * time.Minute)
	stream := &StreamRecord{StartedAt: start, EndedAt: &end}
	rollups := []MinuteRollup{
		{MinuteTS: start.Add(10 * time.Minute), ChatCount: 50, ChatSource: RollupChatSourceIVR, SourceConfidence: SourceConfidenceProvisional, ChatSourceDetail: RollupDetailIVRPeaksOnly},
	}
	metrics := summarizeStreamMetrics(stream, filterTimelineRollups(rollups))
	if metrics.MinutesWithData != 0 {
		t.Fatalf("peaks-only must not inflate minutes with data: %+v", metrics)
	}
	if metrics.DataCoveragePct != 0 {
		t.Fatalf("peaks-only must not inflate coverage pct: %+v", metrics)
	}
}

func TestChatCoverageSummaryIgnoresPeaksOnly(t *testing.T) {
	start := time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC)
	end := start.Add(60 * time.Minute)
	stream := &StreamRecord{StartedAt: start, EndedAt: &end}
	rollups := []MinuteRollup{
		{MinuteTS: start.Add(5 * time.Minute), ChatCount: 40, ChatSourceDetail: RollupDetailIVRPeaksOnly},
	}
	summary := chatCoverageSummary(rollups, stream, 0)
	if summary.ChatSpanMinutes != 0 || summary.CoveragePct != 0 {
		t.Fatalf("peaks-only must not inflate chat coverage: %+v", summary)
	}
}

func TestPortalMinutesExcludesPeaksUnlessRequested(t *testing.T) {
	start := time.Date(2026, 6, 25, 18, 0, 0, 0, time.UTC)
	stream := &StreamRecord{StreamID: "1", Login: "ludwig", StartedAt: start}
	rollups := []MinuteRollup{
		{MinuteTS: start.Add(2 * time.Minute), ChatCount: 90, ChatSourceDetail: RollupDetailIVRPeaksOnly},
		{MinuteTS: start.Add(4 * time.Minute), ChatCount: 30, ChatSource: RollupChatSourceGQL, SourceConfidence: SourceConfidenceCanonical},
	}
	defaultResp := portalMinutesFromRollups(stream, rollups, false)
	if len(defaultResp.Minutes) != 1 || defaultResp.Minutes[0].ChatCount != 30 {
		t.Fatalf("default timeline = %+v", defaultResp.Minutes)
	}
	if len(defaultResp.ProvisionalPeakMinutes) != 0 {
		t.Fatalf("provisional peaks should be omitted by default")
	}
	withPeaks := portalMinutesFromRollups(stream, rollups, true)
	if len(withPeaks.ProvisionalPeakMinutes) != 1 || withPeaks.ProvisionalPeakMinutes[0].ChatCount != 90 {
		t.Fatalf("explicit peaks = %+v", withPeaks.ProvisionalPeakMinutes)
	}
}
