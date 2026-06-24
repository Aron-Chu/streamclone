package analytics

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"streamclone/internal/metrics"
)

func TestTop500MetadataSamplerDisabledNoop(t *testing.T) {
	store := newFakeTop500SamplerStore(makeTop500SamplerChannels(5))
	provider := &fakeTop500MetadataProvider{}
	locker := &fakeTop500SamplerLocker{acquire: true}
	sampler := NewTop500MetadataSampler(Top500SamplerConfig{Enabled: false}, store, provider, locker)

	result, err := sampler.RunTick(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if !hasTop500SamplerClass(result, Top500SamplerClassSamplerDisabled) {
		t.Fatalf("classes = %#v", result.Classifications)
	}
	if provider.streamCalls != 0 || provider.userCalls != 0 || store.currentLookups != 0 || locker.tryCalls != 0 {
		t.Fatalf("disabled sampler touched dependencies provider=%d/%d store=%d lock=%d", provider.streamCalls, provider.userCalls, store.currentLookups, locker.tryCalls)
	}
}

func TestTop500MetadataSamplerDryRunNoWrites(t *testing.T) {
	now := time.Date(2026, 6, 24, 18, 0, 0, 0, time.UTC)
	store := newFakeTop500SamplerStore(makeTop500SamplerChannels(3))
	provider := &fakeTop500MetadataProvider{}
	sampler := NewTop500MetadataSampler(Top500SamplerConfig{Enabled: true, DryRun: true}, store, provider, &fakeTop500SamplerLocker{acquire: true})

	result, err := sampler.RunTick(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(result.Planned); got != 3 {
		t.Fatalf("planned = %d, want 3", got)
	}
	if !hasTop500SamplerClass(result, Top500SamplerClassDryRun) {
		t.Fatalf("classes = %#v", result.Classifications)
	}
	if result.WritesAttempted != 0 || store.snapshotWrites != 0 || store.currentWrites != 0 {
		t.Fatalf("dry-run wrote result=%d snapshots=%d current=%d", result.WritesAttempted, store.snapshotWrites, store.currentWrites)
	}
	if provider.streamCalls != 1 || provider.userCalls != 1 {
		t.Fatalf("provider calls = streams %d users %d", provider.streamCalls, provider.userCalls)
	}
}

func TestTop500MetadataSamplerWriteDisabledNoWrites(t *testing.T) {
	store := newFakeTop500SamplerStore(makeTop500SamplerChannels(2))
	sampler := NewTop500MetadataSampler(Top500SamplerConfig{Enabled: true, DryRun: false, WriteEnabled: false}, store, &fakeTop500MetadataProvider{}, &fakeTop500SamplerLocker{acquire: true})

	result, err := sampler.RunTick(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if !hasTop500SamplerClass(result, Top500SamplerClassWriteDisabled) {
		t.Fatalf("classes = %#v", result.Classifications)
	}
	if result.WritesAttempted != 0 || store.snapshotWrites != 0 || store.currentWrites != 0 {
		t.Fatalf("write-disabled path wrote result=%d snapshots=%d current=%d", result.WritesAttempted, store.snapshotWrites, store.currentWrites)
	}
}

func TestTop500MetadataSamplerWriteEnabledLiveSampleWrites(t *testing.T) {
	now := time.Date(2026, 6, 24, 19, 0, 0, 0, time.UTC)
	startedAt := now.Add(-90 * time.Minute)
	viewerCount := 42000
	store := newFakeTop500SamplerStore(makeTop500SamplerChannels(1))
	provider := &fakeTop500MetadataProvider{
		streams: []Top500StreamMetadata{{
			ChannelID:    "1",
			Login:        "chan1",
			StreamID:     "stream-1",
			Title:        "live title",
			CategoryID:   "509658",
			CategoryName: "Just Chatting",
			StartedAt:    &startedAt,
			ViewerCount:  &viewerCount,
			Language:     "en",
			Tags:         []string{"English"},
			SampledAt:    now,
		}},
		users: []Top500UserMetadata{{ChannelID: "1", Login: "chan1", DisplayName: "Chan One"}},
	}
	sampler := NewTop500MetadataSampler(Top500SamplerConfig{Enabled: true, DryRun: false, WriteEnabled: true}, store, provider, &fakeTop500SamplerLocker{acquire: true})

	result, err := sampler.RunTick(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if result.WritesAttempted != 1 || store.snapshotRows() != 1 || store.currentRows() != 1 {
		t.Fatalf("writes result=%d snapshots=%d current=%d", result.WritesAttempted, store.snapshotRows(), store.currentRows())
	}
	snapshot := store.snapshotFor("1", now)
	if snapshot == nil || !snapshot.IsLive || snapshot.StreamID == nil || *snapshot.StreamID != "stream-1" || snapshot.Source != Top500SnapshotSourceHelixStreams {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	current := store.current["1"]
	if current == nil || !current.IsLive || current.StreamID == nil || *current.StreamID != "stream-1" || !current.StaleAfter.Equal(now.Add(DefaultTop500MetadataStaleAfter)) {
		t.Fatalf("current = %#v", current)
	}
}

func TestTop500MetadataSamplerWriteEnabledOfflineSampleWrites(t *testing.T) {
	now := time.Date(2026, 6, 24, 19, 5, 0, 0, time.UTC)
	store := newFakeTop500SamplerStore(makeTop500SamplerChannels(1))
	provider := &fakeTop500MetadataProvider{
		streams: []Top500StreamMetadata{},
		users:   []Top500UserMetadata{{ChannelID: "1", Login: "chan1", DisplayName: "Chan One"}},
	}
	sampler := NewTop500MetadataSampler(Top500SamplerConfig{Enabled: true, DryRun: false, WriteEnabled: true}, store, provider, &fakeTop500SamplerLocker{acquire: true})

	result, err := sampler.RunTick(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := store.snapshotFor("1", now)
	current := store.current["1"]
	if result.WritesAttempted != 1 || snapshot == nil || snapshot.IsLive || snapshot.StreamID != nil || snapshot.Source != Top500SnapshotSourceHelixUsers {
		t.Fatalf("result=%+v snapshot=%#v", result, snapshot)
	}
	if current == nil || current.IsLive || current.StreamID != nil || current.CoverageSource != Top500CoverageSourceMetadata || !current.StaleAfter.Equal(now.Add(DefaultTop500MetadataStaleAfter)) {
		t.Fatalf("current = %#v", current)
	}
}

func TestTop500MetadataSamplerWritePathIdempotency(t *testing.T) {
	now := time.Date(2026, 6, 24, 19, 10, 0, 0, time.UTC)
	store := newFakeTop500SamplerStore(makeTop500SamplerChannels(1))
	viewerCount := 100
	samples := buildTop500MetadataSamples(store.channels, []Top500StreamMetadata{{ChannelID: "1", Login: "chan1", StreamID: "stream-1", ViewerCount: &viewerCount, SampledAt: now}}, []Top500UserMetadata{{ChannelID: "1", Login: "chan1", DisplayName: "Chan One"}}, now)
	if err := store.WriteTop500MetadataSamples(context.Background(), samples); err != nil {
		t.Fatal(err)
	}
	updatedViewerCount := 200
	retry := buildTop500MetadataSamples(store.channels, []Top500StreamMetadata{{ChannelID: "1", Login: "chan1", StreamID: "stream-1", ViewerCount: &updatedViewerCount, SampledAt: now.Add(5 * time.Second)}}, []Top500UserMetadata{{ChannelID: "1", Login: "chan1", DisplayName: "Chan One"}}, now)
	if err := store.WriteTop500MetadataSamples(context.Background(), retry); err != nil {
		t.Fatal(err)
	}
	if got := store.snapshotRows(); got != 1 {
		t.Fatalf("duplicate tick snapshot rows = %d, want 1", got)
	}
	if snapshot := store.snapshotFor("1", now); snapshot == nil || snapshot.ViewerCount == nil || *snapshot.ViewerCount != updatedViewerCount {
		t.Fatalf("retry snapshot = %#v", snapshot)
	}
	later := buildTop500MetadataSamples(store.channels, []Top500StreamMetadata{{ChannelID: "1", Login: "chan1", StreamID: "stream-2", SampledAt: now.Add(time.Minute)}}, []Top500UserMetadata{{ChannelID: "1", Login: "chan1", DisplayName: "Chan One"}}, now.Add(time.Minute))
	if err := store.WriteTop500MetadataSamples(context.Background(), later); err != nil {
		t.Fatal(err)
	}
	if store.snapshotRows() != 2 || store.currentRows() != 1 {
		t.Fatalf("rows snapshots=%d current=%d, want 2/1", store.snapshotRows(), store.currentRows())
	}
}

func TestTop500MetadataSamplerTopNAndBatchSizeEnforced(t *testing.T) {
	channels := makeTop500SamplerChannels(130)
	channels[4].Enabled = false
	channels[9].Source = "helix"
	store := newFakeTop500SamplerStore(channels)
	sampler := NewTop500MetadataSampler(Top500SamplerConfig{Enabled: true, DryRun: true, TopN: 150, BatchSize: 150}, store, &fakeTop500MetadataProvider{}, &fakeTop500SamplerLocker{acquire: true})

	result, err := sampler.RunTick(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if got := len(result.Planned); got != DefaultTop500MetadataTopN {
		t.Fatalf("planned = %d, want %d valid enabled rows", got, DefaultTop500MetadataTopN)
	}
	for _, channel := range result.Planned {
		if !channel.Enabled || !allowedTop500ChannelSource(channel.Source) {
			t.Fatalf("planned invalid channel: %+v", channel)
		}
	}
	if store.lastLimit != DefaultTop500MetadataTopN {
		t.Fatalf("store limit = %d, want %d", store.lastLimit, DefaultTop500MetadataTopN)
	}

	batchStore := newFakeTop500SamplerStore(makeTop500SamplerChannels(100))
	batchSampler := NewTop500MetadataSampler(Top500SamplerConfig{Enabled: true, DryRun: true, TopN: 100, BatchSize: 10}, batchStore, &fakeTop500MetadataProvider{}, &fakeTop500SamplerLocker{acquire: true})
	batchResult, err := batchSampler.RunTick(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if got := len(batchResult.Planned); got != 10 {
		t.Fatalf("batch planned = %d, want 10", got)
	}
}

func TestTop500MetadataSamplerCadencePlanning(t *testing.T) {
	now := time.Date(2026, 6, 24, 18, 0, 0, 0, time.UTC)
	channels := makeTop500SamplerChannels(5)
	store := newFakeTop500SamplerStore(channels)
	store.current["1"] = &Top500Current{ChannelID: "1", IsLive: true, SampledAt: now.Add(-61 * time.Second)}
	store.current["2"] = &Top500Current{ChannelID: "2", IsLive: true, SampledAt: now.Add(-30 * time.Second)}
	store.current["3"] = &Top500Current{ChannelID: "3", IsLive: false, SampledAt: now.Add(-11 * time.Minute)}
	store.current["4"] = &Top500Current{ChannelID: "4", IsLive: false, SampledAt: now.Add(-5 * time.Minute)}
	// Channel 5 is never sampled and should be due immediately.
	sampler := NewTop500MetadataSampler(Top500SamplerConfig{Enabled: true, DryRun: true}, store, &fakeTop500MetadataProvider{}, nil)

	plan, err := sampler.PlanTick(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if got := channelIDs(plan.Planned); fmt.Sprint(got) != "[1 3 5]" {
		t.Fatalf("planned ids = %v, want [1 3 5]", got)
	}
	if got := channelIDs(plan.SkippedNotDue); fmt.Sprint(got) != "[2 4]" {
		t.Fatalf("skipped ids = %v, want [2 4]", got)
	}
}

func TestTop500MetadataSamplerProviderClassifications(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "rate limit", err: ErrTop500ProviderRateLimited, want: Top500SamplerClassHelixRateLimited},
		{name: "auth missing", err: ErrTop500ProviderAuthMissing, want: Top500SamplerClassHelixAuthMissing},
		{name: "transient", err: ErrTop500ProviderTransient, want: Top500SamplerClassHelixTransientError},
		{name: "not found", err: ErrTop500ProviderNotFound, want: Top500SamplerClassHelixNotFound},
		{name: "unavailable", err: ErrTop500ProviderUnavailable, want: Top500SamplerClassProviderUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeTop500SamplerStore(makeTop500SamplerChannels(1))
			provider := &fakeTop500MetadataProvider{streamErr: tt.err}
			sampler := NewTop500MetadataSampler(Top500SamplerConfig{Enabled: true, DryRun: true}, store, provider, &fakeTop500SamplerLocker{acquire: true})
			result, err := sampler.RunTick(context.Background(), time.Now().UTC())
			if err != nil {
				t.Fatal(err)
			}
			if !hasTop500SamplerClass(result, tt.want) {
				t.Fatalf("classes = %#v, want %q", result.Classifications, tt.want)
			}
			if provider.streamCalls != 1 || provider.userCalls != 0 {
				t.Fatalf("provider calls = streams %d users %d", provider.streamCalls, provider.userCalls)
			}
		})
	}
}

func TestTop500MetadataSamplerProviderErrorsDoNotWrite(t *testing.T) {
	tests := []struct {
		name      string
		streamErr error
		userErr   error
		want      string
	}{
		{name: "stream rate limit", streamErr: ErrTop500ProviderRateLimited, want: Top500SamplerClassHelixRateLimited},
		{name: "stream auth missing", streamErr: ErrTop500ProviderAuthMissing, want: Top500SamplerClassHelixAuthMissing},
		{name: "stream transient", streamErr: ErrTop500ProviderTransient, want: Top500SamplerClassHelixTransientError},
		{name: "stream not found", streamErr: ErrTop500ProviderNotFound, want: Top500SamplerClassHelixNotFound},
		{name: "user transient", userErr: ErrTop500ProviderTransient, want: Top500SamplerClassHelixTransientError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeTop500SamplerStore(makeTop500SamplerChannels(1))
			provider := &fakeTop500MetadataProvider{streamErr: tt.streamErr, userErr: tt.userErr}
			sampler := NewTop500MetadataSampler(Top500SamplerConfig{Enabled: true, DryRun: false, WriteEnabled: true}, store, provider, &fakeTop500SamplerLocker{acquire: true})
			result, err := sampler.RunTick(context.Background(), time.Now().UTC())
			if err != nil {
				t.Fatal(err)
			}
			if !hasTop500SamplerClass(result, tt.want) {
				t.Fatalf("classes = %#v, want %q", result.Classifications, tt.want)
			}
			if result.WritesAttempted != 0 || store.snapshotRows() != 0 || store.currentRows() != 0 {
				t.Fatalf("provider error wrote result=%d snapshots=%d current=%d", result.WritesAttempted, store.snapshotRows(), store.currentRows())
			}
		})
	}
}

func TestTop500MetadataSamplerWriteEnabledRequiresAdvisoryLock(t *testing.T) {
	store := newFakeTop500SamplerStore(makeTop500SamplerChannels(1))
	provider := &fakeTop500MetadataProvider{}
	sampler := NewTop500MetadataSampler(Top500SamplerConfig{Enabled: true, DryRun: false, WriteEnabled: true}, store, provider, nil)
	result, err := sampler.RunTick(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if !hasTop500SamplerClass(result, Top500SamplerClassLockUnavailable) {
		t.Fatalf("classes = %#v", result.Classifications)
	}
	if provider.streamCalls != 0 || provider.userCalls != 0 || store.snapshotRows() != 0 || store.currentRows() != 0 {
		t.Fatalf("unlocked write path touched dependencies provider=%d/%d snapshots=%d current=%d", provider.streamCalls, provider.userCalls, store.snapshotRows(), store.currentRows())
	}
}

func TestTop500MetadataSamplerAdvisoryLockBehavior(t *testing.T) {
	store := newFakeTop500SamplerStore(makeTop500SamplerChannels(1))
	provider := &fakeTop500MetadataProvider{}
	locker := &fakeTop500SamplerLocker{acquire: true}
	sampler := NewTop500MetadataSampler(Top500SamplerConfig{Enabled: true, DryRun: true}, store, provider, locker)

	result, err := sampler.RunTick(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if !result.LockAcquired || !result.LockReleased || locker.releaseCalls != 1 {
		t.Fatalf("lock result = %+v releaseCalls=%d", result, locker.releaseCalls)
	}

	lockedOutProvider := &fakeTop500MetadataProvider{}
	lockedOutLocker := &fakeTop500SamplerLocker{acquire: false}
	lockedOutSampler := NewTop500MetadataSampler(Top500SamplerConfig{Enabled: true, DryRun: true}, store, lockedOutProvider, lockedOutLocker)
	lockedOutResult, err := lockedOutSampler.RunTick(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if !hasTop500SamplerClass(lockedOutResult, Top500SamplerClassLockUnavailable) {
		t.Fatalf("classes = %#v", lockedOutResult.Classifications)
	}
	if lockedOutProvider.streamCalls != 0 || lockedOutProvider.userCalls != 0 {
		t.Fatalf("lock unavailable still called provider streams=%d users=%d", lockedOutProvider.streamCalls, lockedOutProvider.userCalls)
	}
}

func TestTop500MetadataSamplerLockReleasedAfterProviderAndStoreErrors(t *testing.T) {
	providerLocker := &fakeTop500SamplerLocker{acquire: true}
	providerSampler := NewTop500MetadataSampler(Top500SamplerConfig{Enabled: true, DryRun: false, WriteEnabled: true}, newFakeTop500SamplerStore(makeTop500SamplerChannels(1)), &fakeTop500MetadataProvider{streamErr: ErrTop500ProviderTransient}, providerLocker)
	providerResult, err := providerSampler.RunTick(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if !providerResult.LockReleased || providerLocker.releaseCalls != 1 {
		t.Fatalf("provider error lock result=%+v releases=%d", providerResult, providerLocker.releaseCalls)
	}

	storeLocker := &fakeTop500SamplerLocker{acquire: true}
	store := newFakeTop500SamplerStore(makeTop500SamplerChannels(1))
	store.writeErr = errors.New("snapshot write failed")
	storeSampler := NewTop500MetadataSampler(Top500SamplerConfig{Enabled: true, DryRun: false, WriteEnabled: true}, store, &fakeTop500MetadataProvider{}, storeLocker)
	storeResult, err := storeSampler.RunTick(context.Background(), time.Now().UTC())
	if err == nil {
		t.Fatal("expected store error")
	}
	if !storeResult.LockReleased || storeLocker.releaseCalls != 1 {
		t.Fatalf("store error lock result=%+v releases=%d", storeResult, storeLocker.releaseCalls)
	}
	if store.snapshotRows() != 0 || store.currentRows() != 0 {
		t.Fatalf("store error left partial writes snapshots=%d current=%d", store.snapshotRows(), store.currentRows())
	}
}

func TestTop500MetadataSamplerNoBroadRetryLoop(t *testing.T) {
	store := newFakeTop500SamplerStore(makeTop500SamplerChannels(1))
	provider := &fakeTop500MetadataProvider{streamErr: ErrTop500ProviderTransient}
	sampler := NewTop500MetadataSampler(Top500SamplerConfig{Enabled: true, DryRun: true}, store, provider, &fakeTop500SamplerLocker{acquire: true})

	result, err := sampler.RunTick(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if !hasTop500SamplerClass(result, Top500SamplerClassHelixTransientError) {
		t.Fatalf("classes = %#v", result.Classifications)
	}
	if provider.streamCalls != 1 {
		t.Fatalf("stream calls = %d, want one attempt without retry", provider.streamCalls)
	}
}

func TestTop500MetadataSamplerUsesFakeProviderOnly(t *testing.T) {
	store := newFakeTop500SamplerStore(makeTop500SamplerChannels(1))
	provider := &fakeTop500MetadataProvider{}
	sampler := NewTop500MetadataSampler(Top500SamplerConfig{Enabled: true, DryRun: true}, store, provider, nil)
	if _, err := sampler.RunTick(context.Background(), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if provider.streamCalls != 1 || provider.userCalls != 1 {
		t.Fatalf("fake provider calls = streams %d users %d", provider.streamCalls, provider.userCalls)
	}
}

func TestTop500MetadataSamplerIntegrationWritePath(t *testing.T) {
	ctx, store := setupTop500Store(t)
	now := time.Now().UTC().Truncate(time.Second)
	startedAt := now.Add(-30 * time.Minute)
	viewerCount := 1234
	channels := []Top500Channel{
		{ChannelID: "100", Login: "livechan", DisplayName: "LiveChan", Rank: 1, Source: Top500ChannelSourceOperatorSeed, SourceVersion: "test", SeededBy: "test", EffectiveAt: now, Enabled: true},
		{ChannelID: "101", Login: "offlinechan", DisplayName: "OfflineChan", Rank: 2, Source: Top500ChannelSourceOperatorSeed, SourceVersion: "test", SeededBy: "test", EffectiveAt: now, Enabled: true},
	}
	if err := store.UpsertTop500Channels(ctx, channels); err != nil {
		t.Fatalf("seed channels: %v", err)
	}
	provider := &fakeTop500MetadataProvider{
		streams: []Top500StreamMetadata{{ChannelID: "100", Login: "livechan", StreamID: "stream-100", Title: "live", StartedAt: &startedAt, ViewerCount: &viewerCount, SampledAt: now}},
		users: []Top500UserMetadata{
			{ChannelID: "100", Login: "livechan", DisplayName: "LiveChan"},
			{ChannelID: "101", Login: "offlinechan", DisplayName: "OfflineChan"},
		},
	}
	sampler := NewTop500MetadataSampler(Top500SamplerConfig{Enabled: true, DryRun: false, WriteEnabled: true, TopN: 2, BatchSize: 2}, store, provider, &fakeTop500SamplerLocker{acquire: true})
	snapshotWritesBefore := testutil.ToFloat64(metrics.Top500MetadataSnapshotWritesTotal.WithLabelValues("success", "write_enabled"))
	currentUpsertsBefore := testutil.ToFloat64(metrics.Top500MetadataCurrentUpsertsTotal.WithLabelValues("success", "write_enabled"))
	result, err := sampler.RunTick(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.WritesAttempted != 2 {
		t.Fatalf("writes attempted = %d, want 2", result.WritesAttempted)
	}
	if delta := testutil.ToFloat64(metrics.Top500MetadataSnapshotWritesTotal.WithLabelValues("success", "write_enabled")) - snapshotWritesBefore; delta != 2 {
		t.Fatalf("snapshot write metric delta = %v, want 2", delta)
	}
	if delta := testutil.ToFloat64(metrics.Top500MetadataCurrentUpsertsTotal.WithLabelValues("success", "write_enabled")) - currentUpsertsBefore; delta != 2 {
		t.Fatalf("current upsert metric delta = %v, want 2", delta)
	}
	if got := testutil.ToFloat64(metrics.Top500MetadataWriteBatchSize.WithLabelValues("success", "write_enabled", "write_samples")); got != 2 {
		t.Fatalf("write batch size metric = %v, want 2", got)
	}

	var snapshots, currents int
	if err := store.db.QueryRow(ctx, `SELECT COUNT(*) FROM top500_live_snapshots`).Scan(&snapshots); err != nil {
		t.Fatalf("count snapshots: %v", err)
	}
	if err := store.db.QueryRow(ctx, `SELECT COUNT(*) FROM top500_current`).Scan(&currents); err != nil {
		t.Fatalf("count current: %v", err)
	}
	if snapshots != 2 || currents != 2 {
		t.Fatalf("rows snapshots=%d current=%d, want 2/2", snapshots, currents)
	}

	provider.streams[0].Title = "updated live"
	if err := store.WriteTop500MetadataSamples(ctx, buildTop500MetadataSamples(channels, provider.streams, provider.users, now)); err != nil {
		t.Fatalf("retry same tick: %v", err)
	}
	if err := store.db.QueryRow(ctx, `SELECT COUNT(*) FROM top500_live_snapshots WHERE channel_id='100'`).Scan(&snapshots); err != nil {
		t.Fatalf("count duplicate snapshots: %v", err)
	}
	if snapshots != 1 {
		t.Fatalf("duplicate tick rows = %d, want 1", snapshots)
	}
	if err := store.WriteTop500MetadataSamples(ctx, buildTop500MetadataSamples(channels[:1], provider.streams, provider.users, now.Add(time.Minute))); err != nil {
		t.Fatalf("later tick: %v", err)
	}
	if err := store.db.QueryRow(ctx, `SELECT COUNT(*) FROM top500_live_snapshots WHERE channel_id='100'`).Scan(&snapshots); err != nil {
		t.Fatalf("count later snapshots: %v", err)
	}
	if snapshots != 2 {
		t.Fatalf("later tick rows = %d, want 2", snapshots)
	}
	if err := store.db.QueryRow(ctx, `SELECT COUNT(*) FROM top500_current WHERE channel_id='100'`).Scan(&currents); err != nil {
		t.Fatalf("count current channel: %v", err)
	}
	if currents != 1 {
		t.Fatalf("current rows for channel = %d, want 1", currents)
	}
}

func TestTop500MetadataSamplerRuntimeWiringInMain(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "cmd", "analytics", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"Top500MetadataEnabled",
		"Top500SamplerConfigFromApp",
		"InitTop500MetadataSamplerMetrics",
		"NewTop500MetadataSampler",
		"StartTop500MetadataSampler",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("Batch L1 runtime wiring missing %q in cmd/analytics/main.go", required)
		}
	}
	if !strings.Contains(source, "top500 metadata sampler disabled") {
		t.Fatal("expected disabled-default startup log in cmd/analytics/main.go")
	}
}

type fakeTop500SamplerStore struct {
	channels       []Top500Channel
	current        map[string]*Top500Current
	snapshots      map[string]Top500LiveSnapshot
	lastLimit      int
	currentLookups int
	snapshotWrites int
	currentWrites  int
	writeErr       error
}

func newFakeTop500SamplerStore(channels []Top500Channel) *fakeTop500SamplerStore {
	return &fakeTop500SamplerStore{channels: channels, current: map[string]*Top500Current{}, snapshots: map[string]Top500LiveSnapshot{}}
}

func (s *fakeTop500SamplerStore) ListEnabledTop500Channels(_ context.Context, limit int) ([]Top500Channel, error) {
	s.lastLimit = limit
	filtered := filterTop500SamplerRoster(s.channels, limit)
	out := make([]Top500Channel, len(filtered))
	copy(out, filtered)
	return out, nil
}

func (s *fakeTop500SamplerStore) GetTop500CurrentByChannelID(_ context.Context, channelID string) (*Top500Current, error) {
	s.currentLookups++
	return s.current[channelID], nil
}

func (s *fakeTop500SamplerStore) WriteTop500MetadataSamples(_ context.Context, samples []Top500MetadataSample) error {
	if s.writeErr != nil {
		return s.writeErr
	}
	for _, sample := range samples {
		s.snapshots[top500SnapshotKey(sample.Snapshot.ChannelID, sample.Snapshot.SampleTickAt)] = sample.Snapshot
		current := sample.Current
		s.current[current.ChannelID] = &current
		s.snapshotWrites++
		s.currentWrites++
	}
	return nil
}

func (s *fakeTop500SamplerStore) snapshotFor(channelID string, sampleTickAt time.Time) *Top500LiveSnapshot {
	snapshot, ok := s.snapshots[top500SnapshotKey(channelID, sampleTickAt)]
	if !ok {
		return nil
	}
	return &snapshot
}

func (s *fakeTop500SamplerStore) snapshotRows() int {
	return len(s.snapshots)
}

func (s *fakeTop500SamplerStore) currentRows() int {
	return len(s.current)
}

func top500SnapshotKey(channelID string, sampleTickAt time.Time) string {
	return channelID + ":" + sampleTickAt.UTC().Format(time.RFC3339Nano)
}

type fakeTop500MetadataProvider struct {
	streamCalls int
	userCalls   int
	streamErr   error
	userErr     error
	streams     []Top500StreamMetadata
	users       []Top500UserMetadata
}

func (p *fakeTop500MetadataProvider) FetchStreams(_ context.Context, channels []Top500Channel) ([]Top500StreamMetadata, error) {
	p.streamCalls++
	if p.streamErr != nil {
		return nil, p.streamErr
	}
	if p.streams != nil {
		return append([]Top500StreamMetadata(nil), p.streams...), nil
	}
	out := make([]Top500StreamMetadata, 0, len(channels))
	for _, channel := range channels {
		out = append(out, Top500StreamMetadata{ChannelID: channel.ChannelID, Login: channel.Login})
	}
	return out, nil
}

func (p *fakeTop500MetadataProvider) FetchUsers(_ context.Context, channels []Top500Channel) ([]Top500UserMetadata, error) {
	p.userCalls++
	if p.userErr != nil {
		return nil, p.userErr
	}
	if p.users != nil {
		return append([]Top500UserMetadata(nil), p.users...), nil
	}
	out := make([]Top500UserMetadata, 0, len(channels))
	for _, channel := range channels {
		out = append(out, Top500UserMetadata{ChannelID: channel.ChannelID, Login: channel.Login, DisplayName: channel.DisplayName})
	}
	return out, nil
}

type fakeTop500SamplerLocker struct {
	acquire      bool
	tryCalls     int
	releaseCalls int
}

func (l *fakeTop500SamplerLocker) TryTop500MetadataSamplerLock(context.Context) (Top500MetadataSamplerLock, bool, error) {
	l.tryCalls++
	if !l.acquire {
		return nil, false, nil
	}
	return fakeTop500SamplerLock{locker: l}, true, nil
}

type fakeTop500SamplerLock struct {
	locker *fakeTop500SamplerLocker
}

func (l fakeTop500SamplerLock) Release(context.Context) error {
	l.locker.releaseCalls++
	return nil
}

func makeTop500SamplerChannels(count int) []Top500Channel {
	channels := make([]Top500Channel, 0, count)
	for i := 1; i <= count; i++ {
		channels = append(channels, Top500Channel{
			ChannelID:   fmt.Sprintf("%d", i),
			Login:       fmt.Sprintf("chan%d", i),
			DisplayName: fmt.Sprintf("Chan%d", i),
			Rank:        i,
			Source:      Top500ChannelSourceOperatorSeed,
			Enabled:     true,
		})
	}
	return channels
}

func hasTop500SamplerClass(result Top500SamplerTickResult, want string) bool {
	for _, class := range result.Classifications {
		if class == want {
			return true
		}
	}
	return false
}

func channelIDs(channels []Top500Channel) []string {
	out := make([]string, 0, len(channels))
	for _, channel := range channels {
		out = append(out, channel.ChannelID)
	}
	return out
}
