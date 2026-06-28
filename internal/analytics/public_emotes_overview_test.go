package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func TestPublicEmotesOverviewResponseShape(t *testing.T) {
	payload := PublicEmotesOverviewResponse{
		Range:         "7d",
		SchemaVersion: publicEmotesOverviewSchemaVersion,
		State:         "ready",
		AggregateOnly: true,
		ProviderSummaryPreview: []PublicProviderPreview{
			{Provider: "7TV", SharePct: 0, TotalUses: 0, TrackedMinutes: 300, CoveragePct: 80, Confidence: 90},
		},
		CreatorLeaderboardPreview: []PublicCreatorPreviewRow{{Placeholder: true}},
		RisingEmotePreview:        []PublicRisingEmotePreviewRow{{Placeholder: true}},
		SuppressionRules: PublicEmotesSuppressionRules{
			Mode:                  "suppress_below_minimums",
			MinimumTrackedMinutes: 300,
			MinimumCoveragePct:    60,
			MinimumConfidencePct:  60,
			MinimumTotalUses:      100,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureNoForbiddenPublicKeys(payload); err != nil {
		t.Fatalf("payload unexpectedly failed sanitizer: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		"range",
		"generatedAt",
		"schemaVersion",
		"state",
		"degraded",
		"stalenessSec",
		"trackedMinutes",
		"coveragePct",
		"confidence",
		"aggregateOnly",
		"providerSummaryPreview",
		"creatorLeaderboardPreview",
		"risingEmotePreview",
		"suppressionRules",
	} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("missing key %q", key)
		}
	}
}

func TestPublicEmoteProviderMigrationSchemaAggregateSafe(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "migrations", "000051_public_emote_provider_hourly_rollups.up.sql"))
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip("migration 000051 not present in this checkout")
		}
		t.Fatal(err)
	}
	schema := string(body)
	for _, required := range []string{
		"CREATE TABLE IF NOT EXISTS public_emote_provider_hourly_rollups",
		"bucket_hour TIMESTAMPTZ NOT NULL",
		"corpus_key TEXT NOT NULL DEFAULT '__all__'",
		"provider TEXT NOT NULL",
		"total_uses BIGINT NOT NULL CHECK (total_uses >= 0)",
		"tracked_minutes BIGINT NOT NULL CHECK (tracked_minutes >= 0)",
		"emote_minutes BIGINT NOT NULL CHECK (emote_minutes >= 0)",
		"coverage_pct DOUBLE PRECISION NOT NULL CHECK (coverage_pct >= 0 AND coverage_pct <= 100)",
		"confidence DOUBLE PRECISION NOT NULL CHECK (confidence >= 0 AND confidence <= 100)",
		"PRIMARY KEY (bucket_hour, corpus_key, provider)",
	} {
		if !strings.Contains(schema, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
	for _, forbidden := range []string{"stream_id", "login", "twitch_id", "broadcaster_id", "chatter", "user_id", "raw_chat", "message_text", "fragments", "rank"} {
		if strings.Contains(strings.ToLower(schema), forbidden) {
			t.Fatalf("migration contains forbidden field token %q", forbidden)
		}
	}
}

func TestPublicEmoteProviderMaterializerSQLUsesAggregateSources(t *testing.T) {
	for _, required := range []string{"FROM analytics_minute_rollups", "FROM emote_usage_minute_rollups", "COUNT(*)::bigint AS tracked_minutes", "COUNT(DISTINCT (stream_id, minute_ts))::bigint AS emote_minutes"} {
		if !strings.Contains(publicEmoteProviderMaterializeSQL, required) {
			t.Fatalf("materializer SQL missing %q", required)
		}
	}
	for _, forbidden := range []string{"raw_chat", "rawChat", "chat_text", "message_text", "chatter", "chatter_id", "chatter_login", "user_id", "userRankings", "chatterLeaderboard", "viewerList", "messages"} {
		if strings.Contains(publicEmoteProviderMaterializeSQL, forbidden) || strings.Contains(publicEmoteProviderRowsQuery, forbidden) || strings.Contains(publicEmoteProviderCorpusQuery, forbidden) {
			t.Fatalf("provider materialization SQL contains forbidden token %q", forbidden)
		}
	}
}

func TestPublicEmoteProviderRowsExposeBackendOwnedMetrics(t *testing.T) {
	payload := PublicEmotesOverviewResponse{
		ProviderSummaryPreview: []PublicProviderPreview{{Provider: "seventv", TotalUses: 120, SharePct: 75, TrackedMinutes: 320, CoveragePct: 65, Confidence: 94}},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"totalUses", "sharePct", "trackedMinutes", "coveragePct", "confidence"} {
		if !strings.Contains(string(body), key) {
			t.Fatalf("provider DTO missing %q in %s", key, body)
		}
	}
}

func TestPublicEmotesProviderLandscapeStates(t *testing.T) {
	base := PublicEmotesOverviewResponse{SuppressionRules: PublicEmotesSuppressionRules{
		MinimumTrackedMinutes: publicEmoteMinimumTrackedMinutes,
		MinimumCoveragePct:    int64(publicEmoteMinimumCoveragePct),
		MinimumConfidencePct:  int64(publicEmoteMinimumConfidencePct),
		MinimumTotalUses:      publicEmoteMinimumTotalUses,
	}}

	tests := []struct {
		name      string
		landscape PublicEmoteProviderLandscape
		state     string
		degraded  bool
	}{
		{name: "no rows", landscape: PublicEmoteProviderLandscape{}, state: "empty"},
		{name: "low coverage", landscape: PublicEmoteProviderLandscape{Rows: []PublicProviderPreview{{Provider: "seventv"}}, TotalUses: 200, TrackedMinutes: 400, CoveragePct: 30, Confidence: 90}, state: "degraded", degraded: true},
		{name: "low confidence", landscape: PublicEmoteProviderLandscape{Rows: []PublicProviderPreview{{Provider: "seventv"}}, TotalUses: 200, TrackedMinutes: 400, CoveragePct: 80, Confidence: 30}, state: "degraded", degraded: true},
		{name: "stale", landscape: PublicEmoteProviderLandscape{Rows: []PublicProviderPreview{{Provider: "seventv"}}, TotalUses: 200, TrackedMinutes: 400, CoveragePct: 80, Confidence: 90, StalenessSec: int64((publicEmoteProviderFreshnessMax + time.Second).Seconds())}, state: "degraded", degraded: true},
		{name: "ready", landscape: PublicEmoteProviderLandscape{Rows: []PublicProviderPreview{{Provider: "seventv"}}, TotalUses: 200, TrackedMinutes: 400, CoveragePct: 80, Confidence: 90}, state: "ready"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := base
			applyPublicEmotesProviderLandscape(&payload, tt.landscape)
			if payload.State != tt.state || payload.Degraded != tt.degraded {
				t.Fatalf("state = %q degraded=%v, want %q degraded=%v", payload.State, payload.Degraded, tt.state, tt.degraded)
			}
		})
	}
}

func TestPublicEmotesOverviewUnavailableHasZeroMetrics(t *testing.T) {
	h := &Handler{store: &Store{}}
	payload, err := h.buildPublicEmotesOverview(context.Background(), "7d")
	if err != nil {
		t.Fatal(err)
	}
	if payload.State != "unavailable" || payload.TrackedMinutes != 0 || payload.CoveragePct != 0 || payload.Confidence != 0 || len(payload.ProviderSummaryPreview) != 0 {
		t.Fatalf("unavailable payload = %+v", payload)
	}
}

func TestPublicEmotesOverviewPreviewsRemainPlaceholderOnly(t *testing.T) {
	payload := PublicEmotesOverviewResponse{}
	landscape := PublicEmoteProviderLandscape{Rows: []PublicProviderPreview{{Provider: "seventv", TotalUses: 200, TrackedMinutes: 400, CoveragePct: 80, Confidence: 90}}, TotalUses: 200, TrackedMinutes: 400, CoveragePct: 80, Confidence: 90}
	applyPublicEmotesProviderLandscape(&payload, landscape)
	if len(payload.CreatorLeaderboardPreview) != 0 || len(payload.RisingEmotePreview) != 0 {
		t.Fatalf("provider landscape must not populate creator/rising previews: %+v", payload)
	}
}

func TestPublicEmoteProviderMaterializerIntegration(t *testing.T) {
	ctx, store := setupEmoteHistoryStore(t)
	applyPublicEmoteProviderMigration(t, ctx, store)
	applyPublicEmoteMaterializationRunsMigration(t, ctx, store)
	base := time.Now().UTC().Truncate(time.Hour).Add(-2 * time.Hour)
	mustExec(t, ctx, store, `
		INSERT INTO analytics_streams (stream_id, broadcaster_id, login, display_name, started_at)
		VALUES ('provider-stream','71092938','xqc','xQc',$1)`, base)
	for i := 0; i < 4; i++ {
		mustExec(t, ctx, store, `
			INSERT INTO analytics_minute_rollups (stream_id, minute_ts, chat_count, total_emote_count, seventv_emote_count, emotes_json)
			VALUES ('provider-stream',$1,10,10,7,'{}'::jsonb)`, base.Add(time.Duration(i)*time.Minute))
	}
	mustExec(t, ctx, store, `
		INSERT INTO emote_usage_minute_rollups (stream_id, minute_ts, twitch_id, login, provider, provider_emote_id, emote_name, use_count, identity_resolution, confidence, source_key)
		VALUES
		('provider-stream',$1,'71092938','xqc','seventv','a','A',10,'provider_id',0.95,'seventv:a:A'),
		('provider-stream',$1,'71092938','xqc','twitch','b','B',5,'provider_id',0.90,'twitch:b:B'),
		('provider-stream',$1,'71092938','xqc','ffz','c','C',2,'provider_id',0.80,'ffz:c:C')`, base)

	if err := store.MaterializePublicEmoteProviderHourlyRollups(ctx, "24h", base.Add(3*time.Hour)); err != nil {
		t.Fatalf("materialize: %v", err)
	}
	landscape, err := store.PublicEmoteProviderLandscape(ctx, "24h", base.Add(3*time.Hour))
	if err != nil {
		t.Fatalf("read landscape: %v", err)
	}
	if landscape.TotalUses != 17 || landscape.TrackedMinutes != 4 || landscape.EmoteMinutes != 1 {
		t.Fatalf("corpus = uses %d tracked %d emoteMinutes %d", landscape.TotalUses, landscape.TrackedMinutes, landscape.EmoteMinutes)
	}
	if len(landscape.Rows) != 3 || landscape.Rows[0].Provider != "seventv" || landscape.Rows[0].TrackedMinutes != 4 {
		t.Fatalf("provider rows = %+v", landscape.Rows)
	}
	if math.Abs(landscape.Rows[0].SharePct-58.823) > 0.01 {
		t.Fatalf("seventv share = %f", landscape.Rows[0].SharePct)
	}
	var corpusRows int
	if err := store.db.QueryRow(ctx, `SELECT COUNT(*) FROM public_emote_provider_hourly_rollups WHERE corpus_key='__all__' AND provider='__all__'`).Scan(&corpusRows); err != nil {
		t.Fatal(err)
	}
	if corpusRows != 1 {
		t.Fatalf("__all__ corpus rows = %d, want 1", corpusRows)
	}
}

func TestPublicEmoteProviderRefreshWorkerDefaultsDisabled(t *testing.T) {
	worker := NewPublicEmoteProviderRefreshWorker(&fakePublicEmoteProviderRefreshStore{}, PublicEmoteProviderRefreshConfig{}, nil)
	if worker.Enabled() {
		t.Fatal("refresh worker default Enabled = true, want false")
	}
	if worker.cfg.Interval != 15*time.Minute {
		t.Fatalf("default interval = %s, want 15m", worker.cfg.Interval)
	}
}

func TestPublicEmoteProviderRefreshWorkerInvokesMaterializerWhenEnabled(t *testing.T) {
	fake := &fakePublicEmoteProviderRefreshStore{}
	worker := NewPublicEmoteProviderRefreshWorker(fake, PublicEmoteProviderRefreshConfig{Enabled: true, Interval: time.Minute, Range: "24h"}, nil)
	stats, err := worker.RunOnce(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if fake.calls.Load() != 1 {
		t.Fatalf("materializer calls = %d, want 1", fake.calls.Load())
	}
	if stats.Status != "success" || stats.RowsUpserted != 4 {
		t.Fatalf("stats = %+v", stats)
	}
}

func TestPublicEmoteProviderRefreshWorkerPreventsOverlappingRuns(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	fake := &fakePublicEmoteProviderRefreshStore{entered: entered, release: release}
	worker := NewPublicEmoteProviderRefreshWorker(fake, PublicEmoteProviderRefreshConfig{Enabled: true, Interval: time.Minute, Range: "24h"}, nil)
	done := make(chan error, 1)
	go func() {
		_, err := worker.RunOnce(context.Background(), "first")
		done <- err
	}()
	<-entered
	_, err := worker.RunOnce(context.Background(), "second")
	if !IsPublicEmoteProviderRefreshAlreadyRunning(err) {
		t.Fatalf("overlap err = %v, want already running", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("first run err = %v", err)
	}
	if fake.calls.Load() != 1 {
		t.Fatalf("materializer calls = %d, want 1", fake.calls.Load())
	}
}

func TestPublicEmoteProviderRefreshRunMetadataSuccess(t *testing.T) {
	ctx, store := setupEmoteHistoryStore(t)
	applyPublicEmoteProviderMigration(t, ctx, store)
	applyPublicEmoteMaterializationRunsMigration(t, ctx, store)
	base := time.Now().UTC().Truncate(time.Hour).Add(-2 * time.Hour)
	insertProviderRefreshFixture(t, ctx, store, base)
	stats, err := store.RefreshPublicEmoteProviderMaterialization(ctx, "24h", base.Add(3*time.Hour))
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if stats.Status != "success" || stats.RowsUpserted == 0 || stats.RunID == 0 {
		t.Fatalf("stats = %+v", stats)
	}
	var status string
	var rows int64
	if err := store.db.QueryRow(ctx, `SELECT status, rows_upserted FROM public_emote_materialization_runs WHERE run_id=$1`, stats.RunID).Scan(&status, &rows); err != nil {
		t.Fatal(err)
	}
	if status != "success" || rows == 0 {
		t.Fatalf("run status=%q rows=%d", status, rows)
	}
}

func TestPublicEmoteProviderRefreshFailureKeepsPriorRows(t *testing.T) {
	ctx, store := setupEmoteHistoryStore(t)
	applyPublicEmoteProviderMigration(t, ctx, store)
	applyPublicEmoteMaterializationRunsMigration(t, ctx, store)
	base := time.Now().UTC().Truncate(time.Hour).Add(-2 * time.Hour)
	insertProviderRefreshFixture(t, ctx, store, base)
	if _, err := store.RefreshPublicEmoteProviderMaterialization(ctx, "24h", base.Add(3*time.Hour)); err != nil {
		t.Fatalf("first refresh: %v", err)
	}
	before := scalarInt(t, ctx, store, `SELECT COUNT(*) FROM public_emote_provider_hourly_rollups`)
	mustExec(t, ctx, store, `DROP TABLE emote_usage_minute_rollups`)
	_, err := store.RefreshPublicEmoteProviderMaterialization(ctx, "24h", base.Add(3*time.Hour))
	if err == nil {
		t.Fatal("expected refresh failure")
	}
	after := scalarInt(t, ctx, store, `SELECT COUNT(*) FROM public_emote_provider_hourly_rollups`)
	if after != before {
		t.Fatalf("provider rows after failed refresh = %d, want prior %d", after, before)
	}
	var status string
	if err := store.db.QueryRow(ctx, `SELECT status FROM public_emote_materialization_runs ORDER BY run_id DESC LIMIT 1`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "failed" {
		t.Fatalf("latest run status = %q, want failed", status)
	}
}

func TestPublicEmotesOverviewBuildDoesNotCallMaterializerSynchronously(t *testing.T) {
	body, err := os.ReadFile("public_emotes_overview.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "MaterializePublicEmoteProviderHourlyRollups(ctx") {
		t.Fatal("overview build must not synchronously materialize provider rollups")
	}
}

func TestPublicEmotesProviderLandscapeStaleMetadataDegrades(t *testing.T) {
	payload := PublicEmotesOverviewResponse{SuppressionRules: PublicEmotesSuppressionRules{
		MinimumTrackedMinutes: publicEmoteMinimumTrackedMinutes,
		MinimumCoveragePct:    int64(publicEmoteMinimumCoveragePct),
		MinimumConfidencePct:  int64(publicEmoteMinimumConfidencePct),
		MinimumTotalUses:      publicEmoteMinimumTotalUses,
	}}
	landscape := PublicEmoteProviderLandscape{
		Rows:           []PublicProviderPreview{{Provider: "seventv"}},
		TotalUses:      200,
		TrackedMinutes: 400,
		CoveragePct:    80,
		Confidence:     90,
		StalenessSec:   int64((publicEmoteProviderFreshnessMax + time.Second).Seconds()),
	}
	applyPublicEmotesProviderLandscape(&payload, landscape)
	if payload.State != "degraded" || !payload.Degraded {
		t.Fatalf("payload = %+v, want degraded stale", payload)
	}
}

func applyPublicEmoteProviderMigration(t *testing.T, ctx context.Context, store *Store) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "migrations", "000051_public_emote_provider_hourly_rollups.up.sql"))
	if err != nil {
		t.Fatalf("read provider migration: %v", err)
	}
	if _, err := store.db.Exec(ctx, string(body)); err != nil {
		t.Fatalf("apply provider migration: %v", err)
	}
}

func applyPublicEmoteMaterializationRunsMigration(t *testing.T, ctx context.Context, store *Store) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "migrations", "000052_public_emote_materialization_runs.up.sql"))
	if err != nil {
		t.Fatalf("read materialization runs migration: %v", err)
	}
	if _, err := store.db.Exec(ctx, string(body)); err != nil {
		t.Fatalf("apply materialization runs migration: %v", err)
	}
}

func insertProviderRefreshFixture(t *testing.T, ctx context.Context, store *Store, base time.Time) {
	t.Helper()
	mustExec(t, ctx, store, `
		INSERT INTO analytics_streams (stream_id, broadcaster_id, login, display_name, started_at)
		VALUES ('provider-refresh-stream','71092938','xqc','xQc',$1)`, base)
	for i := 0; i < 4; i++ {
		mustExec(t, ctx, store, `
			INSERT INTO analytics_minute_rollups (stream_id, minute_ts, chat_count, total_emote_count, seventv_emote_count, emotes_json)
			VALUES ('provider-refresh-stream',$1,10,10,7,'{}'::jsonb)`, base.Add(time.Duration(i)*time.Minute))
	}
	mustExec(t, ctx, store, `
		INSERT INTO emote_usage_minute_rollups (stream_id, minute_ts, twitch_id, login, provider, provider_emote_id, emote_name, use_count, identity_resolution, confidence, source_key)
		VALUES
		('provider-refresh-stream',$1,'71092938','xqc','seventv','a','A',10,'provider_id',0.95,'seventv:a:A'),
		('provider-refresh-stream',$1,'71092938','xqc','twitch','b','B',5,'provider_id',0.90,'twitch:b:B')`, base)
}

type fakePublicEmoteProviderRefreshStore struct {
	calls   atomic.Int32
	entered chan struct{}
	release chan struct{}
	err     error
}

func (f *fakePublicEmoteProviderRefreshStore) RefreshPublicEmoteProviderMaterialization(context.Context, string, time.Time) (PublicEmoteProviderMaterializationStats, error) {
	f.calls.Add(1)
	if f.entered != nil {
		close(f.entered)
	}
	if f.release != nil {
		<-f.release
	}
	if f.err != nil {
		return PublicEmoteProviderMaterializationStats{Status: "failed", ErrorCode: publicEmoteProviderErrorCode(f.err)}, f.err
	}
	now := time.Now().UTC()
	return PublicEmoteProviderMaterializationStats{Status: "success", StartedAt: now.Add(-time.Second), FinishedAt: now, Duration: time.Second, RowsUpserted: 4}, nil
}

var errFakePublicEmoteProviderRefresh = errors.New("fake refresh failure")

func TestPublicEmotesOverviewSanitizerRejectsForbiddenKeys(t *testing.T) {
	poisoned := map[string]any{
		"range": "7d",
		"preview": map[string]any{
			"chatterLeaderboard": []any{"forbidden"},
			"nested": map[string]any{
				"rawChatText": "forbidden",
			},
			"users": []any{
				map[string]any{"userId": "forbidden"},
				map[string]any{"userRankings": []any{"forbidden"}},
			},
		},
	}

	err := ensureNoForbiddenPublicKeys(poisoned)
	if err == nil {
		t.Fatal("expected sanitizer error for poisoned payload")
	}
	for _, key := range []string{"chatterLeaderboard", "rawChatText", "userId", "userRankings"} {
		if !strings.Contains(err.Error(), key) {
			t.Fatalf("sanitizer error %q did not include forbidden key %q", err.Error(), key)
		}
	}
}

func TestPublicEmotesOverviewFreshAndCachedDTORejectForbiddenKeys(t *testing.T) {
	freshDTO := map[string]any{
		"providerSummaryPreview": []any{map[string]any{"provider": "seventv", "totalUses": 100}},
		"rawChat":                "forbidden",
	}
	cachedDTO := map[string]any{
		"providerSummaryPreview": []any{map[string]any{"provider": "twitch", "totalUses": 25}},
		"nested": map[string]any{
			"messages": []any{map[string]any{"chatterLogin": "forbidden"}},
		},
	}
	for name, dto := range map[string]any{"fresh": freshDTO, "cached": cachedDTO} {
		t.Run(name, func(t *testing.T) {
			if err := ensureNoForbiddenPublicKeys(dto); err == nil {
				t.Fatal("expected forbidden public key rejection")
			}
		})
	}
}

func TestPublicEmotesOverviewRouteIsPublicAndReturnsContract(t *testing.T) {
	h := &Handler{
		pulseHosted: PulseHostedConfig{Hosted: true, BetaKeys: []string{"secret-one"}},
	}
	r := chi.NewRouter()
	h.Routes(r)

	req := httptest.NewRequest(http.MethodGet, "/v1/public/emotes/overview?range=7d", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("public emotes overview should not require auth")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var payload PublicEmotesOverviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Range != "7d" {
		t.Fatalf("range = %q, want 7d", payload.Range)
	}
	if payload.SchemaVersion != publicEmotesOverviewSchemaVersion {
		t.Fatalf("schemaVersion = %q", payload.SchemaVersion)
	}
	if !payload.AggregateOnly {
		t.Fatal("aggregateOnly must be true")
	}
	if err := ensureNoForbiddenPublicKeys(payload); err != nil {
		t.Fatalf("route payload failed sanitizer: %v", err)
	}
}
