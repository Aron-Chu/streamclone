package analytics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func sampleHubResponse() PublicHubResponse {
	now := time.Date(2026, 6, 26, 18, 0, 0, 0, time.UTC)
	return PublicHubResponse{
		GeneratedAt: now,
		PoolSize:    2,
		Corpus: HubCorpus{
			StreamsTracked:        1200,
			MomentsDetected:       340,
			ChatMessagesProcessed: 9_000_000,
			EmotesIndexed:         52000,
			VodsAnalyzed:          410,
		},
		Coverage: HubCoverage{
			LiveChannels: 2, TrackingMax: 100, BackfillActive: 1, BackfillMax: 3,
			SyncActive: 1, EmotesIndexed: 52000, DatabaseOK: true, State: "operational",
		},
		Activity: HubActivity{
			Points:        []HubActivityPoint{{T: now.UnixMilli(), Chat: 120, Emotes: 55, SevenTV: 40, Viewers: 50000}},
			WindowMinutes: hubActivityWindowMinutes,
			ChannelCount:  2,
		},
		EmoteIntel: HubEmoteIntel{EmotesPerMin: 88.5, TopEmoteShare: 22.1, UniqueEmotes: 140, BiggestPeak: 320, SevenTVSharePct: 61.2},
		TopEmotes: []HubEmote{
			{Name: "KEKW", Provider: "7tv", ImageURL: "https://cdn.example/7tv/abc/2x.webp", Count: 900, SharePct: 22.1},
		},
		TopMovers: []HubMover{
			{Login: "xqc", DisplayName: "xQc", Category: "Just Chatting", Viewers: 48000, SevenTVPerMin: 31.2, ChatPerMin: 210, TrendPct: 44.0},
		},
		LiveChannels: []HubLiveChannel{
			{Login: "xqc", DisplayName: "xQc", Category: "Just Chatting", ProfileImageURL: "https://cdn.example/p.png", Viewers: 48000, ChatPerMin: 210, SevenTVPerMin: 31.2, CoverageState: "synced", TrendPct: 44.0},
		},
		Moments: []HubMoment{
			{Kind: "live_attach", Login: "xqc", DisplayName: "xQc", Label: "xQc went live", Detail: "Just Chatting", At: now.UnixMilli()},
		},
	}
}

func TestPublicHubResponseOmitsSensitiveKeys(t *testing.T) {
	resp := sampleHubResponse()
	// Populate the corpus pipeline so the forbidden-key scan covers it too.
	resp.CorpusPipeline = HubCorpusPipeline{
		GeneratedAt:               resp.GeneratedAt,
		State:                     CorpusStatusDegraded,
		TopN:                      500,
		LiveAdmissionEnabled:      true,
		LiveAdmissionTopN:         500,
		MaxActiveIRCChannels:      100,
		CollectorActive:           80,
		CollectorMax:              100,
		MetadataSampledAgoSeconds: hubTestIntPtr(90),
		Roster: HubTrackerSummary{
			Live: 60, CollectorTracking: 40, ExpectedCollectorRows: 60, LiveCollectorDeficitRows: 20,
			MetadataOnly: 18, MetadataStale: 0, AdmissionFeatureDisabled: 0, AdmissionDisabled: 0, CapacityBlocked: 2,
			Warming: 5, Collecting: 35, ViewerOnly: 1, ZeroChatAfterAge: 0,
		},
		Silver: HubTierCounts{Queued: 4, Running: 2, Done: 120, Skipped: 3, Failed: 1, Total: 130},
		Gold:   HubTierCounts{Queued: 1, Running: 1, Done: 48, Skipped: 0, Failed: 0, Total: 50},
	}
	body, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	raw := strings.ToLower(string(body))
	// The hub is hosted-safe: it must never leak raw rollups, per-minute emote
	// maps, storage URIs, principals, beta keys, or scraper/GQL internals. The
	// corpus pipeline must expose aggregate counts ONLY — never per-channel
	// readiness rows, admission attempts/messages, stream IDs, or job errors.
	for _, forbidden := range []string{
		"rollups", `"emotes":{`, "gcs", "gs://", "principal", "betakey", "beta_key", "operatorkey", "gql", "tracker",
		`"rows"`, "recentadmissions", "admissionoutcome", "admissionmessage", `"streamid"`, "metadatasampledat",
		"freshnessseconds", `"error"`, "categoryname",
	} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("public hub payload must not contain %q", forbidden)
		}
	}
}

func TestAdmissionFeatureDisabledRows(t *testing.T) {
	report := Top100ReadinessReport{Summary: Top100ReadinessSummary{LiveRows: 12}}
	if got := admissionFeatureDisabledRows(CorpusRuntimeConfig{LiveAdmissionEnabled: true}, report); got != 0 {
		t.Fatalf("enabled rows = %d, want 0", got)
	}
	if got := admissionFeatureDisabledRows(CorpusRuntimeConfig{}, report); got != 12 {
		t.Fatalf("disabled rows = %d, want 12", got)
	}
}

func TestTop500ReportMetadataSampledAgoSeconds(t *testing.T) {
	if got := top500ReportMetadataSampledAgoSeconds(Top100ReadinessReport{}); got != nil {
		t.Fatalf("empty report age = %v, want nil", *got)
	}
	oldest := 120
	newest := 30
	got := top500ReportMetadataSampledAgoSeconds(Top100ReadinessReport{Rows: []Top100ReadinessRow{
		{MetadataFreshnessSeconds: &newest},
		{MetadataFreshnessSeconds: &oldest},
	}})
	if got == nil || *got != oldest {
		t.Fatalf("metadata age = %v, want %d", got, oldest)
	}
}

func hubTestIntPtr(v int) *int {
	return &v
}

func TestSummarizeChannelWindowSynced(t *testing.T) {
	start := time.Date(2026, 6, 26, 17, 0, 0, 0, time.UTC)
	rec := &StreamRecord{StreamID: "s1", Login: "xqc", DisplayName: "xQc", CurrentViewers: 1000}
	rollups := []MinuteRollup{
		{MinuteTS: start, ChatCount: 40, TotalEmoteCount: 10, SevenTVEmoteCount: 6, ViewerSamples: 1, ViewerLatest: 900},
		{MinuteTS: start.Add(time.Minute), ChatCount: 60, TotalEmoteCount: 20, SevenTVEmoteCount: 12, ViewerSamples: 1, ViewerLatest: 1100},
	}
	win := summarizeChannelWindow(rec, rollups)
	if win.coverageState != "synced" {
		t.Fatalf("coverageState = %q, want synced", win.coverageState)
	}
	if win.viewers != 1100 {
		t.Fatalf("viewers = %d, want 1100 (latest rollup)", win.viewers)
	}
	if win.chatPerMin != 50 {
		t.Fatalf("chatPerMin = %v, want 50", win.chatPerMin)
	}
}

func TestHubLiveActivityCountsExcludeCorpusRollups(t *testing.T) {
	chat, emotes, seven := hubLiveActivityCounts(MinuteRollup{
		ChatCount: 100, ChatSource: RollupChatSourceGQL, SourceConfidence: SourceConfidenceCanonical,
		TotalEmoteCount: 50, SevenTVEmoteCount: 10,
	})
	if chat != 0 || emotes != 0 || seven != 0 {
		t.Fatalf("gql rollup leaked into hub activity: chat=%d emotes=%d seven=%d", chat, emotes, seven)
	}
	chat, emotes, seven = hubLiveActivityCounts(MinuteRollup{
		ChatCount: 12, ChatSource: RollupChatSourceLive, SourceConfidence: SourceConfidenceVerified,
		TotalEmoteCount: 8, SevenTVEmoteCount: 3,
	})
	if chat != 12 || emotes != 8 || seven != 3 {
		t.Fatalf("live rollup counts wrong: chat=%d emotes=%d seven=%d", chat, emotes, seven)
	}
	if hubLiveViewerRollup(MinuteRollup{ChatSource: RollupChatSourceGQL}) {
		t.Fatal("gql viewer rollup should be excluded")
	}
	if !hubLiveViewerRollup(MinuteRollup{ChatSource: RollupChatSourceLive}) {
		t.Fatal("live viewer rollup should be included")
	}
}

func TestHubRollupProviderCounts(t *testing.T) {
	ru := MinuteRollup{
		ChatCount:         10,
		ChatSource:        RollupChatSourceLive,
		SourceConfidence:  SourceConfidenceVerified,
		SevenTVEmoteCount: 5,
		Emotes: map[string]int{
			"twitch:1:LUL":    4,
			"bttv:2:OMEGALUL": 3,
			"ffz:3:PepeLaugh": 2,
			"seventv:4:KEKW":  99,
		},
	}
	tw, bt, fz := hubRollupProviderCounts(ru)
	if tw != 4 || bt != 3 || fz != 2 {
		t.Fatalf("provider counts = (%d,%d,%d), want (4,3,2)", tw, bt, fz)
	}
	gql := MinuteRollup{
		ChatCount: 100, ChatSource: RollupChatSourceGQL, SourceConfidence: SourceConfidenceCanonical,
		Emotes: map[string]int{"twitch:1:LUL": 50},
	}
	if tw, bt, fz := hubRollupProviderCounts(gql); tw != 0 || bt != 0 || fz != 0 {
		t.Fatalf("gql rollup leaked provider counts: (%d,%d,%d)", tw, bt, fz)
	}
}

func TestFilterPublicHubLiveRollupsExcludesCorpusImports(t *testing.T) {
	start := time.Date(2026, 6, 26, 17, 0, 0, 0, time.UTC)
	rollups := []MinuteRollup{
		{MinuteTS: start, ChatCount: 100, ChatSource: RollupChatSourceGQL, SourceConfidence: SourceConfidenceCanonical, TotalEmoteCount: 50},
		{MinuteTS: start.Add(time.Minute), ChatCount: 12, ChatSource: RollupChatSourceLive, SourceConfidence: SourceConfidenceVerified, TotalEmoteCount: 8},
		{MinuteTS: start.Add(2 * time.Minute), ChatCount: 5, ChatSource: ChatSourceMixed, SourceConfidence: SourceConfidenceProvisional, TotalEmoteCount: 3},
	}
	filtered := filterPublicHubLiveRollups(rollups)
	if len(filtered) != 1 || filtered[0].ChatSource != RollupChatSourceLive {
		t.Fatalf("filtered rollups = %+v, want live only", filtered)
	}
	rec := &StreamRecord{StreamID: "s1", Login: "xqc", CurrentViewers: 1000}
	win := summarizeChannelWindow(rec, rollups)
	if win.chatPerMin != 12 {
		t.Fatalf("chatPerMin = %v, want 12 from live rollup only", win.chatPerMin)
	}
}

func TestShouldForceRefreshPublicHubAtRespectsLongWindowTTL(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	opts7d := publicHubOptions{ActivityWindowMinutes: 7 * 24 * 60}
	last := now.Add(-2 * time.Minute)
	if shouldForceRefreshPublicHubAt(last, now, opts7d) {
		t.Fatal("7d hub should not refresh after 2m")
	}
	if !shouldForceRefreshPublicHubAt(last, now.Add(6*time.Minute), opts7d) {
		t.Fatal("7d hub should refresh after 5m TTL")
	}
	opts30m := publicHubOptions{}
	last30 := now.Add(-20 * time.Second)
	if shouldForceRefreshPublicHubAt(last30, now, opts30m) {
		t.Fatal("30m hub should not refresh after 20s")
	}
	if !shouldForceRefreshPublicHubAt(last30, now.Add(31*time.Second), opts30m) {
		t.Fatal("30m hub should refresh after 30s TTL")
	}
}

func TestPublicHubCacheTTLForOptions(t *testing.T) {
	if got := publicHubCacheTTLForOptions(publicHubOptions{}); got != publicHubCacheTTL {
		t.Fatalf("default window TTL = %s, want %s", got, publicHubCacheTTL)
	}
	if got := publicHubCacheTTLForOptions(publicHubOptions{ActivityWindowMinutes: hubActivityWindowMinutes}); got != publicHubCacheTTL {
		t.Fatalf("30m window TTL = %s, want %s", got, publicHubCacheTTL)
	}
	if got := publicHubCacheTTLForOptions(publicHubOptions{ActivityWindowMinutes: 7 * 24 * 60}); got != publicHubLongCacheTTL {
		t.Fatalf("7d window TTL = %s, want %s", got, publicHubLongCacheTTL)
	}
}

func TestHubRollupEmoteCountFallsBackToMapAndSevenTV(t *testing.T) {
	if got := hubRollupEmoteCount(MinuteRollup{
		TotalEmoteCount:   0,
		SevenTVEmoteCount: 7,
		Emotes:            map[string]int{"seventv:1:KEKW": 3, "twitch:2:Kappa": 2},
	}); got != 7 {
		t.Fatalf("emote count = %d, want max(map sum 5, 7TV 7)", got)
	}
	if got := hubRollupEmoteCount(MinuteRollup{
		TotalEmoteCount:   12,
		SevenTVEmoteCount: 7,
		Emotes:            map[string]int{"seventv:1:KEKW": 3},
	}); got != 12 {
		t.Fatalf("emote count = %d, want total 12", got)
	}
}

func TestSummarizeChannelWindowUsesSevenTVFallbackForEmotes(t *testing.T) {
	start := time.Date(2026, 6, 26, 17, 0, 0, 0, time.UTC)
	rec := &StreamRecord{StreamID: "s1", Login: "xqc", DisplayName: "xQc", CurrentViewers: 1000}
	win := summarizeChannelWindow(rec, []MinuteRollup{
		{MinuteTS: start, ChatCount: 40, TotalEmoteCount: 0, SevenTVEmoteCount: 6, ViewerSamples: 1},
		{MinuteTS: start.Add(time.Minute), ChatCount: 60, TotalEmoteCount: 0, SevenTVEmoteCount: 12, ViewerSamples: 1},
	})
	if win.emotesPerMin != 9 {
		t.Fatalf("emotesPerMin = %v, want 9 from 7TV fallback", win.emotesPerMin)
	}
	if win.seventvPerMin != 9 {
		t.Fatalf("seventvPerMin = %v, want 9", win.seventvPerMin)
	}
}

func TestWindowTrendPctRising(t *testing.T) {
	base := time.Date(2026, 6, 26, 17, 0, 0, 0, time.UTC)
	rollups := make([]MinuteRollup, 0, 10)
	// prior 5 minutes low, recent 5 minutes high -> strong positive trend.
	for i := 0; i < 5; i++ {
		rollups = append(rollups, MinuteRollup{
			MinuteTS: base.Add(time.Duration(i) * time.Minute), ChatCount: 10,
			ChatSource: RollupChatSourceLive, SourceConfidence: SourceConfidenceVerified,
		})
	}
	for i := 5; i < 10; i++ {
		rollups = append(rollups, MinuteRollup{
			MinuteTS: base.Add(time.Duration(i) * time.Minute), ChatCount: 40,
			ChatSource: RollupChatSourceLive, SourceConfidence: SourceConfidenceVerified,
		})
	}
	if got := windowTrendPct(rollups); got <= 0 {
		t.Fatalf("expected positive trend, got %v", got)
	}
}

func TestGetPublicHubRouteIsOpen(t *testing.T) {
	// Hosted handler with no store/collector wired: route must still be public
	// (no auth middleware) and return a well-formed, non-nil-array payload.
	h := &Handler{pulseHosted: PulseHostedConfig{Hosted: true, BetaKeys: []string{"secret"}}}
	r := chi.NewRouter()
	h.Routes(r)

	req := httptest.NewRequest(http.MethodGet, "/v1/public/hub", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var payload PublicHubResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.TopEmotes == nil || payload.LiveChannels == nil || payload.Moments == nil || payload.Activity.Points == nil {
		t.Fatalf("hub arrays must serialize as [] not null: %+v", payload)
	}
}

func TestGetPublicHubCacheControlHeaders(t *testing.T) {
	h := &Handler{pulseHosted: PulseHostedConfig{Hosted: true, BetaKeys: []string{"secret"}}}
	r := chi.NewRouter()
	h.Routes(r)

	req := httptest.NewRequest(http.MethodGet, "/v1/public/hub?activityWindow=30m", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	want := "public, max-age=15, s-maxage=30, stale-while-revalidate=60"
	if got := rec.Header().Get("Cache-Control"); got != want {
		t.Fatalf("Cache-Control = %q, want %q", got, want)
	}
	if xCache := rec.Header().Get("X-Cache"); xCache != "HIT" && xCache != "MISS" {
		t.Fatalf("X-Cache = %q, want HIT or MISS", xCache)
	}
}

func TestPublicHubPayloadListCaps(t *testing.T) {
	// Guardrails for hosted-safe fanout payload size — keep in sync with portal UI expectations.
	if hubLiveCap != 96 {
		t.Fatalf("hubLiveCap = %d, want 96", hubLiveCap)
	}
	if hubMoversCap != 12 {
		t.Fatalf("hubMoversCap = %d, want 12", hubMoversCap)
	}
	if hubEmotesCap != 12 {
		t.Fatalf("hubEmotesCap = %d, want 12", hubEmotesCap)
	}
	if hubMomentsCap != 12 {
		t.Fatalf("hubMomentsCap = %d, want 12", hubMomentsCap)
	}
	if hubActivityMaxPoints != 240 {
		t.Fatalf("hubActivityMaxPoints = %d, want 240", hubActivityMaxPoints)
	}
	if hubActivityBucketTopEmotesCap != 10 {
		t.Fatalf("hubActivityBucketTopEmotesCap = %d, want 10", hubActivityBucketTopEmotesCap)
	}
}

func TestForwardFillTop500ViewerTrail(t *testing.T) {
	bucketT := int64(1_719_000_000_000)
	nextT := bucketT + 6*60_000
	activity := map[int64]*HubActivityPoint{
		bucketT: {T: bucketT, Viewers: 450_000, HasViewerRollup: true},
		nextT:   {T: nextT, Viewers: 43_000, HasChatRollup: true, HasViewerRollup: true},
	}
	forwardFillTop500ViewerTrail(activity, []Top500ViewerBucket{
		{T: bucketT, Viewers: 450_000},
		{T: nextT, Viewers: 43_000},
	})
	if activity[nextT].Viewers != 43_000 {
		t.Fatalf("forward fill viewers = %d, want 43000 (explicit top500 sample must not be forward-filled)", activity[nextT].Viewers)
	}
}

func TestMergeTop500ViewerBucketsPrefersSnapshotOverIRCInflation(t *testing.T) {
	bucketT := int64(1_719_000_000_000)
	activity := map[int64]*HubActivityPoint{
		bucketT: {T: bucketT, Viewers: 1_038_669, Chat: 28_675, HasChatRollup: true, HasViewerRollup: true},
	}
	mergeTop500ViewerBucketsIntoActivity(activity, []Top500ViewerBucket{
		{T: bucketT, Viewers: 526_895},
	})
	if activity[bucketT].Viewers != 526_895 {
		t.Fatalf("viewers = %d, want 526895 (top500 snapshot caps IRC inflation)", activity[bucketT].Viewers)
	}
	if activity[bucketT].Chat != 28_675 {
		t.Fatalf("chat = %d, want unchanged IRC chat rollup", activity[bucketT].Chat)
	}
}

func TestMergeCurrentHelixLiveViewersIntoOpenBucket(t *testing.T) {
	now := time.Date(2026, 7, 5, 1, 10, 0, 0, time.UTC)
	windowMinutes := 24 * 60
	bucketMin := hubActivityBucketMinutes(windowMinutes)
	bucketMs := int64(bucketMin) * 60_000
	openKey := (now.UnixMilli() / bucketMs) * bucketMs
	activity := map[int64]*HubActivityPoint{
		openKey: {T: openKey, Viewers: 43_000, Chat: 1000, HasChatRollup: true},
	}
	mergeCurrentHelixLiveViewersIntoOpenBucket(activity, 360_000, now, windowMinutes)
	if activity[openKey].Viewers != 360_000 {
		t.Fatalf("open bucket viewers = %d, want 360000", activity[openKey].Viewers)
	}
}
