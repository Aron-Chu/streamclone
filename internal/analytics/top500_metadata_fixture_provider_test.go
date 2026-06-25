package analytics

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestFixtureTop500MetadataProviderLiveAndOfflineClassification(t *testing.T) {
	now := time.Date(2026, 6, 24, 22, 30, 0, 0, time.UTC)
	channels := []Top500Channel{
		{ChannelID: "37402112", Login: "shroud", DisplayName: "shroud", Rank: 1, Source: Top500ChannelSourceOperatorSeed, Enabled: true},
		{ChannelID: "26490461", Login: "summit1g", DisplayName: "summit1g", Rank: 2, Source: Top500ChannelSourceOperatorSeed, Enabled: true},
		{ChannelID: "36340767", Login: "tarik", DisplayName: "tarik", Rank: 3, Source: Top500ChannelSourceOperatorSeed, Enabled: true},
	}
	store := newFakeTop500SamplerStore(channels)
	provider := NewFixtureTop500MetadataProvider()
	sampler := NewTop500MetadataSampler(Top500SamplerConfig{Enabled: true, DryRun: true}, store, provider, &fakeTop500SamplerLocker{acquire: true})

	result, err := sampler.RunTick(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(result.Planned); got != 3 {
		t.Fatalf("planned = %d, want 3", got)
	}
	if result.StreamsFetched != 1 || result.UsersFetched != 3 {
		t.Fatalf("fetches streams=%d users=%d, want 1/3", result.StreamsFetched, result.UsersFetched)
	}
	if !hasTop500SamplerClass(result, Top500SamplerClassDryRun) {
		t.Fatalf("classes = %#v", result.Classifications)
	}
	if hasTop500SamplerClass(result, Top500SamplerClassHelixAuthMissing) {
		t.Fatal("fixture provider must not classify as helix_auth_missing")
	}
	if result.WritesAttempted != 0 || store.snapshotWrites != 0 || store.currentWrites != 0 {
		t.Fatalf("dry-run wrote result=%d snapshots=%d current=%d", result.WritesAttempted, store.snapshotWrites, store.currentWrites)
	}
}

func TestFixtureTop500MetadataProviderFetchStreamsOnlyLiveLogin(t *testing.T) {
	provider := NewFixtureTop500MetadataProvider()
	streams, err := provider.FetchStreams(context.Background(), []Top500Channel{
		{ChannelID: "37402112", Login: "shroud", Rank: 1},
		{ChannelID: "26490461", Login: "summit1g", Rank: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(streams) != 1 {
		t.Fatalf("streams = %d, want 1 live fixture", len(streams))
	}
	if streams[0].Login != "shroud" || streams[0].StreamID != "fixture-stream-live" {
		t.Fatalf("live stream = %+v", streams[0])
	}
}

func TestNewTop500MetadataProviderSelectsFixture(t *testing.T) {
	helix := NewHelixClient("", "", "", "", "")
	provider := NewTop500MetadataProvider(true, helix)
	if _, ok := provider.(*FixtureTop500MetadataProvider); !ok {
		t.Fatalf("provider = %T, want *FixtureTop500MetadataProvider", provider)
	}
	helixProvider := NewTop500MetadataProvider(false, helix)
	if _, ok := helixProvider.(*HelixTop500MetadataProvider); !ok {
		t.Fatalf("provider = %T, want *HelixTop500MetadataProvider", helixProvider)
	}
}

func TestHelixTop500MetadataProviderDisabledStillAuthMissing(t *testing.T) {
	provider := NewHelixTop500MetadataProvider(NewHelixClient("", "", "", "", ""))
	_, err := provider.FetchStreams(context.Background(), []Top500Channel{{ChannelID: "1", Login: "chan1"}})
	if !errors.Is(err, ErrTop500ProviderAuthMissing) {
		t.Fatalf("err = %v", err)
	}
}
