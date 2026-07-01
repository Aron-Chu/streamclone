package analytics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"streamclone/internal/config"
)

func TestClassifyTopRosterWatchResponse(t *testing.T) {
	tests := []struct {
		name string
		resp WatchResponse
		want string
	}{
		{
			name: "admitted",
			resp: WatchResponse{Tracking: true, Message: "tracking until stream ends"},
			want: TopRosterAdmissionAdmitted,
		},
		{
			name: "already tracking",
			resp: WatchResponse{Tracking: true, Message: "already tracking until stream ends"},
			want: TopRosterAdmissionAlreadyTracking,
		},
		{
			name: "capacity full",
			resp: WatchResponse{Tracking: false, Message: "analytics tracking pool is full"},
			want: TopRosterAdmissionCapacityFull,
		},
		{
			name: "watch error",
			resp: WatchResponse{Tracking: false, Message: "join failed"},
			want: TopRosterAdmissionWatchError,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyTopRosterWatchResponse(tc.resp); got != tc.want {
				t.Fatalf("classifyTopRosterWatchResponse() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTop500PriorityWatchPollerRecordsCapacityBlockedOutcome(t *testing.T) {
	globalTopRosterAdmissionRegistry = topRosterAdmissionRegistry{byLogin: make(map[string]TopRosterAdmissionAttempt)}
	streamA := "111"
	streamB := "222"
	streamC := "333"
	store := &fakeTop500PriorityStore{live: []Top500Current{
		{Login: "a", Rank: 1, IsLive: true, StreamID: &streamA, SampledAt: time.Now().UTC()},
		{Login: "b", Rank: 2, IsLive: true, StreamID: &streamB, SampledAt: time.Now().UTC()},
		{Login: "c", Rank: 3, IsLive: true, StreamID: &streamC, SampledAt: time.Now().UTC()},
	}}
	joiner := &fakeJoiner{}
	collector := NewCollector(&fakeStore{}, fakeProvider{}, joiner, nil, nilLogger(), 1, time.Hour, time.Hour, 200)
	p := NewTop500PriorityWatchPoller(store, collector, config.Config{
		PulseTop500AdmissionEnabled: true,
		PulseTop500AdmissionTopN:    100,
	}, nil)
	p.runOnce(context.Background())
	if len(joiner.joined) != 1 {
		t.Fatalf("expected one admission at cap=1, joined=%#v", joiner.joined)
	}
	attemptB, ok := getTopRosterAdmissionAttempt("b")
	if !ok {
		t.Fatal("expected admission attempt recorded for b")
	}
	if attemptB.Outcome != TopRosterAdmissionCapacityFull {
		t.Fatalf("b outcome = %q, want %q", attemptB.Outcome, TopRosterAdmissionCapacityFull)
	}
	if _, ok := getTopRosterAdmissionAttempt("c"); ok {
		t.Fatal("did not expect admission attempt for c after capacity block")
	}
}

func TestTop500PriorityWatchPollerRecordsAlreadyTracking(t *testing.T) {
	globalTopRosterAdmissionRegistry = topRosterAdmissionRegistry{byLogin: make(map[string]TopRosterAdmissionAttempt)}
	streamA := "111"
	store := &fakeTop500PriorityStore{live: []Top500Current{
		{Login: "live", Rank: 4, IsLive: true, StreamID: &streamA, SampledAt: time.Now().UTC()},
	}}
	joiner := &fakeJoiner{}
	collector := NewCollector(&fakeStore{}, fakeProvider{}, joiner, nil, nilLogger(), 2, time.Hour, time.Hour, 200)
	collector.Watch(context.Background(), "live")
	p := NewTop500PriorityWatchPoller(store, collector, config.Config{
		PulseTop500AdmissionEnabled: true,
		PulseTop500AdmissionTopN:    100,
	}, nil)
	p.runOnce(context.Background())
	attempt, ok := getTopRosterAdmissionAttempt("live")
	if !ok {
		t.Fatal("expected admission attempt for already-tracking channel")
	}
	if attempt.Outcome != TopRosterAdmissionAlreadyTracking {
		t.Fatalf("outcome = %q, want %q", attempt.Outcome, TopRosterAdmissionAlreadyTracking)
	}
	if len(joiner.joined) != 1 {
		t.Fatalf("expected no additional join, joined=%#v", joiner.joined)
	}
}

func TestBuildTop100ReadinessRowMetadataWithoutChat(t *testing.T) {
	globalTopRosterAdmissionRegistry = topRosterAdmissionRegistry{
		byLogin: make(map[string]TopRosterAdmissionAttempt),
	}
	streamID := "stream-1"
	now := time.Now().UTC()
	recordTopRosterAdmissionAttempt(TopRosterAdmissionAttempt{
		Login:       "creator",
		Rank:        7,
		StreamID:    streamID,
		SampledAt:   now.Add(-2 * time.Minute),
		AttemptedAt: now.Add(-1 * time.Minute),
		Outcome:     TopRosterAdmissionCapacityFull,
		Message:     "analytics tracking pool is full",
	})
	joiner := &fakeJoiner{}
	collector := NewCollector(&fakeStore{}, fakeProvider{}, joiner, nil, nilLogger(), 2, time.Hour, time.Hour, 200)
	h := &Handler{collector: collector}
	row := buildTop100ReadinessRow(context.Background(), h, Top500Current{
		Login:      "creator",
		Rank:       7,
		IsLive:     true,
		StreamID:   &streamID,
		SampledAt:  now.Add(-2 * time.Minute),
		StaleAfter: now.Add(15 * time.Minute),
		StartedAt:  ptrTime(now.Add(-30 * time.Minute)),
	}, now, false)
	if row.ReadinessState != Top100ReadinessCapacityBlocked {
		t.Fatalf("readiness state = %q, want %q", row.ReadinessState, Top100ReadinessCapacityBlocked)
	}
	if row.AdmissionOutcome != TopRosterAdmissionCapacityFull {
		t.Fatalf("admission outcome = %q, want %q", row.AdmissionOutcome, TopRosterAdmissionCapacityFull)
	}
	if row.CollectorTracking {
		t.Fatal("expected creator not to be collector tracking")
	}
}

func TestBuildTop100ReadinessRowMetadataOnly(t *testing.T) {
	globalTopRosterAdmissionRegistry = topRosterAdmissionRegistry{byLogin: make(map[string]TopRosterAdmissionAttempt)}
	streamID := "stream-meta"
	now := time.Now().UTC()
	joiner := &fakeJoiner{}
	collector := NewCollector(&fakeStore{}, fakeProvider{}, joiner, nil, nilLogger(), 2, time.Hour, time.Hour, 200)
	h := &Handler{collector: collector}
	row := buildTop100ReadinessRow(context.Background(), h, Top500Current{
		Login:      "freshmeta",
		Rank:       12,
		IsLive:     true,
		StreamID:   &streamID,
		SampledAt:  now.Add(-90 * time.Second),
		StaleAfter: now.Add(15 * time.Minute),
		StartedAt:  ptrTime(now.Add(-20 * time.Minute)),
	}, now, false)
	if row.ReadinessState != Top100ReadinessMetadataOnly {
		t.Fatalf("readiness state = %q, want %q", row.ReadinessState, Top100ReadinessMetadataOnly)
	}

	if row.AdmissionOutcome != "" {
		t.Fatalf("admission outcome = %q, want empty", row.AdmissionOutcome)
	}
	if row.CollectorTracking {
		t.Fatal("expected fresh metadata row not to be collector tracking")
	}
}

func TestRollupsHaveChatOrEmoteSignal(t *testing.T) {
	if rollupsHaveChatOrEmoteSignal([]MinuteRollup{{ViewerSamples: 1, ViewerLatest: 1200}}) {
		t.Fatal("viewer-only rollup should not count as chat/emote signal")
	}
	if !rollupsHaveChatOrEmoteSignal([]MinuteRollup{{ChatCount: 1}}) {
		t.Fatal("chat rollup should count as chat/emote signal")
	}
	if !rollupsHaveChatOrEmoteSignal([]MinuteRollup{{SevenTVEmoteCount: 1}}) {
		t.Fatal("7TV rollup should count as chat/emote signal")
	}
}

func TestTop100ReadinessRollupsFallsBackToTrackedLiveRollupRow(t *testing.T) {
	ctx, store := setupSessionStore(t)
	now := time.Now().UTC()
	insertTestStream(t, ctx, store, "metadata-live", "jynxzi", 0)
	insertTestStream(t, ctx, store, "collector-live", "jynxzi", 10)
	mustExec(t, ctx, store, `
		INSERT INTO analytics_minute_rollups (
			stream_id, minute_ts, viewer_avg, viewer_max, viewer_latest, viewer_samples,
			chat_count, total_emote_count, seventv_emote_count, emotes_json
		)
		VALUES ('collector-live', $1, 1200, 1300, 1250, 1, 42, 7, 5, '{}'::jsonb)`,
		now.Add(-time.Minute))

	collector := NewCollector(&fakeStore{}, fakeProvider{}, &fakeJoiner{}, nil, nilLogger(), 2, time.Hour, time.Hour, 200)
	collector.Watch(context.Background(), "jynxzi")
	h := &Handler{store: store, collector: collector}

	rollups := top100ReadinessRollups(ctx, h, "jynxzi", "metadata-live", now)
	if len(rollups) != 1 {
		t.Fatalf("rollup count = %d, want fallback collector rollup", len(rollups))
	}
	if rollups[0].ChatCount != 42 || rollups[0].TotalEmoteCount != 7 {
		t.Fatalf("fallback rollup = %+v, want collector chat/emote counts", rollups[0])
	}
}

func TestExpectedCollectorRowsCapsAtCollectorMax(t *testing.T) {
	if got := expectedCollectorRows(95, 50); got != 50 {
		t.Fatalf("expectedCollectorRows(95, 50) = %d, want 50", got)
	}
	if got := expectedCollectorRows(12, 50); got != 12 {
		t.Fatalf("expectedCollectorRows(12, 50) = %d, want 12", got)
	}
	if got := expectedCollectorRows(12, 0); got != 12 {
		t.Fatalf("expectedCollectorRows(12, 0) = %d, want 12", got)
	}
}

func TestTopRosterReadinessRouteAlias(t *testing.T) {
	h := &Handler{}
	r := chi.NewRouter()
	h.Routes(r)

	req := httptest.NewRequest(http.MethodGet, "/v1/analytics/top-roster/readiness?topN=500", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code == http.StatusNotFound {
		t.Fatal("expected neutral top-roster readiness route alias to be registered")
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 for nil store", rec.Code)
	}
}
