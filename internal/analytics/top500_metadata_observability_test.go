package analytics

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"streamclone/internal/metrics"
)

func TestTop500MetadataSamplerConfigAndSuccessMetrics(t *testing.T) {
	now := time.Date(2026, 6, 24, 20, 0, 0, 0, time.UTC)
	store := newFakeTop500SamplerStore(makeTop500SamplerChannels(2))
	provider := &fakeTop500MetadataProvider{}
	sampler := NewTop500MetadataSampler(Top500SamplerConfig{Enabled: true, DryRun: false, WriteEnabled: true, TopN: 2, BatchSize: 2}, store, provider, &fakeTop500SamplerLocker{acquire: true})

	plannedBefore := testutil.ToFloat64(metrics.Top500MetadataChannelsPlannedTotal.WithLabelValues("planned", "write_enabled"))
	sampledBefore := testutil.ToFloat64(metrics.Top500MetadataChannelsSampledTotal.WithLabelValues("success", "write_enabled"))
	streamCallsBefore := testutil.ToFloat64(metrics.Top500MetadataProviderCallsTotal.WithLabelValues("fetch_streams", "success", "helix"))
	userCallsBefore := testutil.ToFloat64(metrics.Top500MetadataProviderCallsTotal.WithLabelValues("fetch_users", "success", "helix"))
	snapshotWritesBefore := testutil.ToFloat64(metrics.Top500MetadataSnapshotWritesTotal.WithLabelValues("success", "write_enabled"))
	currentUpsertsBefore := testutil.ToFloat64(metrics.Top500MetadataCurrentUpsertsTotal.WithLabelValues("success", "write_enabled"))

	result, err := sampler.RunTick(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if result.WritesAttempted != 2 {
		t.Fatalf("writes attempted = %d, want 2", result.WritesAttempted)
	}
	if got := testutil.ToFloat64(metrics.Top500MetadataSamplerEnabled); got != 1 {
		t.Fatalf("sampler enabled gauge = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.Top500MetadataDryRun); got != 0 {
		t.Fatalf("dry-run gauge = %v, want 0", got)
	}
	if got := testutil.ToFloat64(metrics.Top500MetadataWriteEnabled); got != 1 {
		t.Fatalf("write-enabled gauge = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.Top500MetadataTopNConfigured); got != 2 {
		t.Fatalf("top n gauge = %v, want 2", got)
	}
	if got := testutil.ToFloat64(metrics.Top500MetadataRosterSize); got != 2 {
		t.Fatalf("roster size gauge = %v, want 2", got)
	}
	if delta := testutil.ToFloat64(metrics.Top500MetadataChannelsPlannedTotal.WithLabelValues("planned", "write_enabled")) - plannedBefore; delta != 2 {
		t.Fatalf("planned delta = %v, want 2", delta)
	}
	if delta := testutil.ToFloat64(metrics.Top500MetadataChannelsSampledTotal.WithLabelValues("success", "write_enabled")) - sampledBefore; delta != 2 {
		t.Fatalf("sampled delta = %v, want 2", delta)
	}
	if delta := testutil.ToFloat64(metrics.Top500MetadataProviderCallsTotal.WithLabelValues("fetch_streams", "success", "helix")) - streamCallsBefore; delta != 1 {
		t.Fatalf("stream provider call delta = %v, want 1", delta)
	}
	if delta := testutil.ToFloat64(metrics.Top500MetadataProviderCallsTotal.WithLabelValues("fetch_users", "success", "helix")) - userCallsBefore; delta != 1 {
		t.Fatalf("user provider call delta = %v, want 1", delta)
	}
	if delta := testutil.ToFloat64(metrics.Top500MetadataSnapshotWritesTotal.WithLabelValues("success", "write_enabled")) - snapshotWritesBefore; delta != 2 {
		t.Fatalf("snapshot write delta = %v, want 2", delta)
	}
	if delta := testutil.ToFloat64(metrics.Top500MetadataCurrentUpsertsTotal.WithLabelValues("success", "write_enabled")) - currentUpsertsBefore; delta != 2 {
		t.Fatalf("current upsert delta = %v, want 2", delta)
	}
	if got := testutil.ToFloat64(metrics.Top500MetadataWriteBatchSize.WithLabelValues("success", "write_enabled", "write_samples")); got != 2 {
		t.Fatalf("write batch size = %v, want 2", got)
	}
	if got := testutil.ToFloat64(metrics.Top500MetadataWriteLatencySeconds.WithLabelValues("success", "write_enabled", "write_samples")); got < 0 {
		t.Fatalf("write latency = %v, want non-negative", got)
	}
	if got := testutil.ToFloat64(metrics.Top500MetadataFreshnessSeconds); got != 0 {
		t.Fatalf("freshness seconds = %v, want 0", got)
	}
}

func TestTop500MetadataSamplerProviderAndRollbackMetrics(t *testing.T) {
	now := time.Date(2026, 6, 24, 20, 5, 0, 0, time.UTC)
	tests := []struct {
		name              string
		err               error
		classification    string
		wantRateLimitBump bool
	}{
		{name: "rate limit", err: ErrTop500ProviderRateLimited, classification: Top500SamplerClassHelixRateLimited, wantRateLimitBump: true},
		{name: "auth missing", err: ErrTop500ProviderAuthMissing, classification: Top500SamplerClassHelixAuthMissing},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeTop500SamplerStore(makeTop500SamplerChannels(1))
			provider := &fakeTop500MetadataProvider{streamErr: tt.err}
			sampler := NewTop500MetadataSampler(Top500SamplerConfig{Enabled: true, DryRun: true}, store, provider, &fakeTop500SamplerLocker{acquire: true})

			providerErrorsBefore := testutil.ToFloat64(metrics.Top500MetadataProviderErrorsTotal.WithLabelValues("fetch_streams", tt.classification, "helix"))
			rateLimitsBefore := testutil.ToFloat64(metrics.Top500MetadataProviderRateLimitsTotal.WithLabelValues("fetch_streams", "helix"))
			degradedBefore := testutil.ToFloat64(metrics.Top500MetadataSamplesDegradedTotal.WithLabelValues(tt.classification, "dry_run"))

			result, err := sampler.RunTick(context.Background(), now)
			if err != nil {
				t.Fatal(err)
			}
			if !hasTop500SamplerClass(result, tt.classification) {
				t.Fatalf("classes = %#v", result.Classifications)
			}
			if delta := testutil.ToFloat64(metrics.Top500MetadataProviderErrorsTotal.WithLabelValues("fetch_streams", tt.classification, "helix")) - providerErrorsBefore; delta != 1 {
				t.Fatalf("provider error delta = %v, want 1", delta)
			}
			rateLimitDelta := testutil.ToFloat64(metrics.Top500MetadataProviderRateLimitsTotal.WithLabelValues("fetch_streams", "helix")) - rateLimitsBefore
			if tt.wantRateLimitBump && rateLimitDelta != 1 {
				t.Fatalf("rate-limit delta = %v, want 1", rateLimitDelta)
			}
			if !tt.wantRateLimitBump && rateLimitDelta != 0 {
				t.Fatalf("rate-limit delta = %v, want 0", rateLimitDelta)
			}
			if delta := testutil.ToFloat64(metrics.Top500MetadataSamplesDegradedTotal.WithLabelValues(tt.classification, "dry_run")) - degradedBefore; delta != 1 {
				t.Fatalf("degraded delta = %v, want 1", delta)
			}
			if got := testutil.ToFloat64(metrics.Top500MetadataRollbackState.WithLabelValues(tt.classification, "dry_run")); got != 1 {
				t.Fatalf("rollback state = %v, want 1", got)
			}
		})
	}
}

func TestTop500MetadataSamplerDryRunWriteDisabledAndLockMetrics(t *testing.T) {
	now := time.Date(2026, 6, 24, 20, 10, 0, 0, time.UTC)
	dryRunStore := newFakeTop500SamplerStore(makeTop500SamplerChannels(1))
	dryRun := NewTop500MetadataSampler(Top500SamplerConfig{Enabled: true, DryRun: true}, dryRunStore, &fakeTop500MetadataProvider{}, nil)
	dryRunDegradedBefore := testutil.ToFloat64(metrics.Top500MetadataSamplesDegradedTotal.WithLabelValues(Top500SamplerClassDryRun, "dry_run"))
	if _, err := dryRun.RunTick(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if dryRunStore.snapshotRows() != 0 || dryRunStore.currentRows() != 0 {
		t.Fatalf("dry-run wrote snapshots=%d current=%d", dryRunStore.snapshotRows(), dryRunStore.currentRows())
	}
	if delta := testutil.ToFloat64(metrics.Top500MetadataSamplesDegradedTotal.WithLabelValues(Top500SamplerClassDryRun, "dry_run")) - dryRunDegradedBefore; delta != 1 {
		t.Fatalf("dry-run degraded delta = %v, want 1", delta)
	}

	writeDisabledStore := newFakeTop500SamplerStore(makeTop500SamplerChannels(1))
	writeDisabled := NewTop500MetadataSampler(Top500SamplerConfig{Enabled: true, DryRun: false, WriteEnabled: false}, writeDisabledStore, &fakeTop500MetadataProvider{}, nil)
	writeDisabledBefore := testutil.ToFloat64(metrics.Top500MetadataSamplesDegradedTotal.WithLabelValues(Top500SamplerClassWriteDisabled, "write_disabled"))
	if _, err := writeDisabled.RunTick(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if writeDisabledStore.snapshotRows() != 0 || writeDisabledStore.currentRows() != 0 {
		t.Fatalf("write-disabled wrote snapshots=%d current=%d", writeDisabledStore.snapshotRows(), writeDisabledStore.currentRows())
	}
	if delta := testutil.ToFloat64(metrics.Top500MetadataSamplesDegradedTotal.WithLabelValues(Top500SamplerClassWriteDisabled, "write_disabled")) - writeDisabledBefore; delta != 1 {
		t.Fatalf("write-disabled degraded delta = %v, want 1", delta)
	}

	lockedOut := NewTop500MetadataSampler(Top500SamplerConfig{Enabled: true, DryRun: false, WriteEnabled: true}, newFakeTop500SamplerStore(makeTop500SamplerChannels(1)), &fakeTop500MetadataProvider{}, &fakeTop500SamplerLocker{acquire: false})
	lockBefore := testutil.ToFloat64(metrics.Top500MetadataLockUnavailableTotal.WithLabelValues(Top500SamplerClassLockUnavailable, "write_enabled"))
	if _, err := lockedOut.RunTick(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if delta := testutil.ToFloat64(metrics.Top500MetadataLockUnavailableTotal.WithLabelValues(Top500SamplerClassLockUnavailable, "write_enabled")) - lockBefore; delta != 1 {
		t.Fatalf("lock unavailable delta = %v, want 1", delta)
	}
}

func TestTop500MetadataSamplerStoreErrorMetrics(t *testing.T) {
	now := time.Date(2026, 6, 24, 20, 15, 0, 0, time.UTC)
	store := newFakeTop500SamplerStore(makeTop500SamplerChannels(1))
	store.writeErr = os.ErrPermission
	sampler := NewTop500MetadataSampler(Top500SamplerConfig{Enabled: true, DryRun: false, WriteEnabled: true}, store, &fakeTop500MetadataProvider{}, &fakeTop500SamplerLocker{acquire: true})

	degradedBefore := testutil.ToFloat64(metrics.Top500MetadataSamplesDegradedTotal.WithLabelValues("store_error", "write_enabled"))
	_, err := sampler.RunTick(context.Background(), now)
	if err == nil {
		t.Fatal("expected store error")
	}
	if store.snapshotRows() != 0 || store.currentRows() != 0 {
		t.Fatalf("store error left partial writes snapshots=%d current=%d", store.snapshotRows(), store.currentRows())
	}
	if delta := testutil.ToFloat64(metrics.Top500MetadataSamplesDegradedTotal.WithLabelValues("store_error", "write_enabled")) - degradedBefore; delta != 1 {
		t.Fatalf("store error degraded delta = %v, want 1", delta)
	}
	if got := testutil.ToFloat64(metrics.Top500MetadataRollbackState.WithLabelValues("store_error", "write_enabled")); got != 1 {
		t.Fatalf("store rollback state = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.Top500MetadataWriteBatchSize.WithLabelValues("error", "write_enabled", "write_samples")); got != 1 {
		t.Fatalf("error batch size = %v, want 1", got)
	}
}

func TestTop500MetadataRollbackStateClearsOnNextTick(t *testing.T) {
	now := time.Date(2026, 6, 24, 20, 20, 0, 0, time.UTC)
	store := newFakeTop500SamplerStore(makeTop500SamplerChannels(1))
	store.writeErr = os.ErrPermission
	sampler := NewTop500MetadataSampler(Top500SamplerConfig{Enabled: true, DryRun: false, WriteEnabled: true}, store, &fakeTop500MetadataProvider{}, &fakeTop500SamplerLocker{acquire: true})
	if _, err := sampler.RunTick(context.Background(), now); err == nil {
		t.Fatal("expected store error")
	}
	if got := testutil.ToFloat64(metrics.Top500MetadataRollbackState.WithLabelValues("store_error", "write_enabled")); got != 1 {
		t.Fatalf("rollback state after error = %v, want 1", got)
	}

	store.writeErr = nil
	if _, err := sampler.RunTick(context.Background(), now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if got := testutil.ToFloat64(metrics.Top500MetadataRollbackState.WithLabelValues("store_error", "write_enabled")); got != 0 {
		t.Fatalf("rollback state after success = %v, want 0", got)
	}
}

func TestTop500MetadataAlertProposalReferencesKnownMetrics(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "deploy", "prometheus", "alerts", "top500-hosted.proposal.yml"))
	if err != nil {
		t.Fatalf("read alert proposal: %v", err)
	}
	known := map[string]bool{
		"streamclone_top500_metadata_sampler_enabled":            true,
		"streamclone_top500_metadata_dry_run":                    true,
		"streamclone_top500_metadata_write_enabled":              true,
		"streamclone_top500_metadata_roster_size":                true,
		"streamclone_top500_metadata_top_n_configured":           true,
		"streamclone_top500_metadata_channels_planned_total":     true,
		"streamclone_top500_metadata_channels_sampled_total":     true,
		"streamclone_top500_metadata_provider_calls_total":       true,
		"streamclone_top500_metadata_provider_errors_total":      true,
		"streamclone_top500_metadata_provider_rate_limits_total": true,
		"streamclone_top500_metadata_freshness_seconds":          true,
		"streamclone_top500_metadata_snapshot_writes_total":      true,
		"streamclone_top500_metadata_current_upserts_total":      true,
		"streamclone_top500_metadata_write_batch_size":           true,
		"streamclone_top500_metadata_write_latency_seconds":      true,
		"streamclone_top500_metadata_samples_degraded_total":     true,
		"streamclone_top500_metadata_rollback_state":             true,
		"streamclone_top500_metadata_lock_unavailable_total":     true,
	}
	re := regexp.MustCompile(`streamclone_top500_metadata_[a-z_]+(?:_total|_seconds)?`)
	seen := map[string]bool{}
	for _, metricName := range re.FindAllString(string(raw), -1) {
		seen[metricName] = true
		if !known[metricName] {
			t.Fatalf("alert proposal references unknown Top 500 metadata metric %q", metricName)
		}
	}
	for _, required := range []string{
		"streamclone_top500_metadata_channels_sampled_total",
		"streamclone_top500_metadata_provider_rate_limits_total",
		"streamclone_top500_metadata_provider_errors_total",
		"streamclone_top500_metadata_freshness_seconds",
		"streamclone_top500_metadata_write_latency_seconds",
		"streamclone_top500_metadata_rollback_state",
		"streamclone_top500_metadata_lock_unavailable_total",
	} {
		if !seen[required] {
			t.Fatalf("alert proposal missing required Top 500 metadata metric %q", required)
		}
	}
	for _, forbidden := range []string{"channel", "channel_id", "login", "stream_id", "vod_id", "title", "user", "rank"} {
		if strings.Contains(string(raw), forbidden+"=") {
			t.Fatalf("alert proposal uses forbidden label %q", forbidden)
		}
	}
}
