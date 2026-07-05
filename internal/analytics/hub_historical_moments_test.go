package analytics

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestParseHubBucketT(t *testing.T) {
	if _, ok := parseHubBucketT(""); ok {
		t.Fatal("empty bucketT should fail")
	}
	if _, ok := parseHubBucketT("nope"); ok {
		t.Fatal("invalid bucketT should fail")
	}
	got, ok := parseHubBucketT("1719900000000")
	if !ok || got != 1719900000000 {
		t.Fatalf("parseHubBucketT = (%d, %v)", got, ok)
	}
}

func TestParseHubMomentsLimit(t *testing.T) {
	if parseHubMomentsLimit("") != hubHistoricalMomentsCap {
		t.Fatal("default limit")
	}
	if parseHubMomentsLimit("999") != hubHistoricalMomentsCap {
		t.Fatal("cap limit")
	}
	if parseHubMomentsLimit("3") != 3 {
		t.Fatal("valid limit")
	}
}

func TestHubBucketTimeRangeMatchesActivityWindow(t *testing.T) {
	bucketT := int64(1_719_000_000_000)
	start, end := hubBucketTimeRange(bucketT, 7*24*60)
	if !end.After(start) {
		t.Fatalf("range = %v .. %v", start, end)
	}
	bucketMinutes := hubActivityBucketMinutes(7 * 24 * 60)
	wantEnd := start.Add(time.Duration(bucketMinutes) * time.Minute)
	if !end.Equal(wantEnd) {
		t.Fatalf("end = %v, want %v", end, wantEnd)
	}
}

func TestHubBucketTimeRangeMatchesAggregateRollupEpochFloor(t *testing.T) {
	samples := []time.Time{
		time.Date(2026, 7, 2, 12, 34, 56, 0, time.UTC),
		time.Date(2026, 7, 2, 0, 0, 1, 0, time.UTC),
		time.Date(2026, 7, 1, 23, 59, 59, 0, time.UTC),
	}
	for _, windowMinutes := range []int{30, 24 * 60, 7 * 24 * 60} {
		bucketMinutes := hubActivityBucketMinutes(windowMinutes)
		bucketSeconds := int64(bucketMinutes) * 60
		for _, sample := range samples {
			wantStart := time.Unix((sample.Unix()/bucketSeconds)*bucketSeconds, 0).UTC()
			start, end := hubBucketTimeRange(sample.UnixMilli(), windowMinutes)
			if !start.Equal(wantStart) {
				t.Fatalf("window=%d sample=%v start=%v, want %v", windowMinutes, sample, start, wantStart)
			}
			wantEnd := wantStart.Add(time.Duration(bucketMinutes) * time.Minute)
			if !end.Equal(wantEnd) {
				t.Fatalf("window=%d sample=%v end=%v, want %v", windowMinutes, sample, end, wantEnd)
			}
		}
	}
}

func TestBuildPublicHubMomentsEmptyWithoutStore(t *testing.T) {
	h := &Handler{}
	payload, err := h.buildPublicHubMoments(context.Background(), publicHubOptions{ActivityWindowMinutes: 24 * 60}, 1_719_000_000_000, 10)
	if err != nil {
		t.Fatal(err)
	}
	if payload.Status != "empty" || payload.Reason != "store_unavailable" {
		t.Fatalf("payload = %+v", payload)
	}
	if payload.Source != "corpus_historical" {
		t.Fatalf("source = %q", payload.Source)
	}
	if payload.HubGeneratedAt.IsZero() {
		t.Fatal("hubGeneratedAt should be set")
	}
}

func TestPublicHubMomentsStoreErrorReturns503(t *testing.T) {
	cfg, err := pgxpool.ParseConfig("postgres://streamclone:streamclone@127.0.0.1:1/streamclone?connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	h := &Handler{store: NewStore(pool)}
	req := httptest.NewRequest(http.MethodGet, "/v1/public/hub/moments?bucketT=1719000000000&activityWindow=7d", nil)
	ctx, cancel := context.WithTimeout(req.Context(), 2*time.Second)
	defer cancel()
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.getPublicHubMoments(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "hub_moments_unavailable") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestPublicHubMomentsCacheTTLForEmptyBucketIsShort(t *testing.T) {
	empty := PublicHubMomentsResponse{Status: "empty", Reason: "no_corpus_peaks_in_bucket"}
	if got := publicHubMomentsCacheTTLForPayload(empty); got != publicHubMomentsEmptyTTL {
		t.Fatalf("empty ttl = %v, want %v", got, publicHubMomentsEmptyTTL)
	}
	if publicHubMomentsEmptyTTL >= publicHubMomentsCacheTTL {
		t.Fatalf("empty ttl %v must be shorter than normal ttl %v", publicHubMomentsEmptyTTL, publicHubMomentsCacheTTL)
	}
	ready := PublicHubMomentsResponse{Status: "ready"}
	if got := publicHubMomentsCacheTTLForPayload(ready); got != publicHubMomentsCacheTTL {
		t.Fatalf("ready ttl = %v, want %v", got, publicHubMomentsCacheTTL)
	}
	storeUnavailable := PublicHubMomentsResponse{Status: "empty", Reason: "store_unavailable"}
	if got := publicHubMomentsCacheTTLForPayload(storeUnavailable); got != publicHubMomentsCacheTTL {
		t.Fatalf("store unavailable ttl = %v, want %v", got, publicHubMomentsCacheTTL)
	}
}

func TestPublicHubMomentsJSONSafe(t *testing.T) {
	payload := PublicHubMomentsResponse{
		BucketT:               1_719_000_000_000,
		BucketStart:           time.UnixMilli(1_719_000_000_000).UTC(),
		BucketEnd:             time.UnixMilli(1_719_002_520_000).UTC(),
		HubGeneratedAt:        time.UnixMilli(1_719_000_001_000).UTC(),
		Source:                "corpus_historical",
		Status:                "ready",
		ActivityWindowMinutes: 7 * 24 * 60,
		Moments: []HubLivePulseMoment{
			{
				Login:         "xqc",
				DisplayName:   "xQc",
				StreamID:      "stream-1",
				OffsetSeconds: 120,
				Score:         88,
				Label:         "Chat spike",
				Source:        "corpus_historical",
				TopEmotes:     []HubEmote{{Name: "Nope", Provider: "7tv", Count: 40}},
				Confidence:    100,
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	raw := strings.ToLower(string(body))
	for _, forbidden := range []string{"rollups", `"emotes":{`, "principal", "gql", "messages", "rawchat"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("payload must not contain %q", forbidden)
		}
	}
	if !strings.Contains(raw, "hubgeneratedat") {
		t.Fatal("payload should include hubGeneratedAt")
	}
}

func TestHistoricalMomentScoreAndLabel(t *testing.T) {
	if historicalMomentScore(0, 0) != 0 {
		t.Fatal("zero score")
	}
	if historicalMomentScore(100, 10) != 20 {
		t.Fatalf("chat score")
	}
	label, kind := historicalMomentLabel(hubHistoricalMinuteCandidate{
		SevenTVEmoteCount: 60,
		ChatCount:         100,
	})
	if label != "Emote spike" || kind != "emotes" {
		t.Fatalf("label = (%q, %q)", label, kind)
	}
}

func TestSortHubHistoricalMoments(t *testing.T) {
	moments := []HubLivePulseMoment{
		{Score: 10, ChatPerMin: 100},
		{Score: 90, ChatPerMin: 50},
		{Score: 90, ChatPerMin: 200},
	}
	sortHubHistoricalMoments(moments)
	if moments[0].ChatPerMin != 200 || moments[1].ChatPerMin != 50 {
		t.Fatalf("sort order = %+v", moments)
	}
}
