package analytics

import (
	"context"
	"errors"
	"testing"
	"time"

	"streamclone/internal/config"
)

func TestShouldStartTop500MetadataSamplerDefaultOff(t *testing.T) {
	if ShouldStartTop500MetadataSampler(config.Config{}) {
		t.Fatal("default config must not start top500 metadata sampler")
	}
	if ShouldStartTop500MetadataSampler(config.Config{Top500MetadataEnabled: false}) {
		t.Fatal("TOP500_METADATA_ENABLED=false must not start sampler")
	}
	if !ShouldStartTop500MetadataSampler(config.Config{Top500MetadataEnabled: true}) {
		t.Fatal("TOP500_METADATA_ENABLED=true should allow sampler startup")
	}
}

func TestStartTop500MetadataSamplerDisabledIsNoOp(t *testing.T) {
	store := newFakeTop500SamplerStore(makeTop500SamplerChannels(1))
	provider := &fakeTop500MetadataProvider{}
	sampler := NewTop500MetadataSampler(Top500SamplerConfig{Enabled: false}, store, provider, &fakeTop500SamplerLocker{acquire: true})

	StartTop500MetadataSampler(context.Background(), sampler, time.Millisecond, nil)
	time.Sleep(5 * time.Millisecond)
	if provider.streamCalls != 0 || provider.userCalls != 0 {
		t.Fatalf("disabled startup must not tick provider: streams=%d users=%d", provider.streamCalls, provider.userCalls)
	}
}

func TestStartTop500MetadataSamplerNilIsNoOp(t *testing.T) {
	StartTop500MetadataSampler(context.Background(), nil, time.Millisecond, nil)
}

func TestStartTop500MetadataSamplerHonoursContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := newFakeTop500SamplerStore(nil)
	provider := &fakeTop500MetadataProvider{}
	sampler := NewTop500MetadataSampler(Top500SamplerConfig{Enabled: true, DryRun: true}, store, provider, nil)
	StartTop500MetadataSampler(ctx, sampler, 5*time.Millisecond, nil)

	time.Sleep(20 * time.Millisecond)
	cancel()
	time.Sleep(20 * time.Millisecond)
}

func TestHelixTop500MetadataProviderDisabledReturnsAuthMissing(t *testing.T) {
	provider := NewHelixTop500MetadataProvider(NewHelixClient("", "", "", "", ""))
	_, err := provider.FetchStreams(context.Background(), []Top500Channel{{ChannelID: "1", Login: "chan1"}})
	if !errors.Is(err, ErrTop500ProviderAuthMissing) {
		t.Fatalf("FetchStreams err = %v, want ErrTop500ProviderAuthMissing", err)
	}
	_, err = provider.FetchUsers(context.Background(), []Top500Channel{{ChannelID: "1", Login: "chan1"}})
	if !errors.Is(err, ErrTop500ProviderAuthMissing) {
		t.Fatalf("FetchUsers err = %v, want ErrTop500ProviderAuthMissing", err)
	}
}

func TestTop500SamplerConfigFromAppMapsFields(t *testing.T) {
	app := config.Config{
		Top500MetadataEnabled:         true,
		Top500MetadataDryRun:          true,
		Top500MetadataWriteEnabled:    false,
		Top500MetadataTopN:            100,
		Top500MetadataBatchSize:       100,
		Top500MetadataLiveInterval:    DefaultTop500MetadataLiveInterval,
		Top500MetadataOfflineInterval: DefaultTop500MetadataOfflineInterval,
	}
	cfg := Top500SamplerConfigFromApp(app)
	if !cfg.Enabled || !cfg.DryRun || cfg.WriteEnabled {
		t.Fatalf("unexpected booleans: enabled=%v dryRun=%v write=%v", cfg.Enabled, cfg.DryRun, cfg.WriteEnabled)
	}
	if cfg.TopN != DefaultTop500MetadataTopN || cfg.BatchSize != DefaultTop500MetadataBatchSize {
		t.Fatalf("unexpected caps: topN=%d batch=%d", cfg.TopN, cfg.BatchSize)
	}
	if cfg.LiveInterval != DefaultTop500MetadataLiveInterval || cfg.OfflineInterval != DefaultTop500MetadataOfflineInterval {
		t.Fatalf("unexpected intervals: live=%s offline=%s", cfg.LiveInterval, cfg.OfflineInterval)
	}
}
