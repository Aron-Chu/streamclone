package analytics

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestTop500MetadataSamplerNotWiredInMain(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "cmd", "analytics", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, forbidden := range []string{
		"NewTop500MetadataSampler",
		"StartTop500MetadataSampler",
		"Top500MetadataProvider",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("Batch I3 must not wire runtime sampler startup in main.go; found %q", forbidden)
		}
	}
}

type fakeTop500SamplerStore struct {
	channels       []Top500Channel
	current        map[string]*Top500Current
	lastLimit      int
	currentLookups int
	snapshotWrites int
	currentWrites  int
}

func newFakeTop500SamplerStore(channels []Top500Channel) *fakeTop500SamplerStore {
	return &fakeTop500SamplerStore{channels: channels, current: map[string]*Top500Current{}}
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

type fakeTop500MetadataProvider struct {
	streamCalls int
	userCalls   int
	streamErr   error
	userErr     error
}

func (p *fakeTop500MetadataProvider) FetchStreams(_ context.Context, channels []Top500Channel) ([]Top500StreamMetadata, error) {
	p.streamCalls++
	if p.streamErr != nil {
		return nil, p.streamErr
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
