package analytics

import (
	"context"
	"sort"
	"strings"
	"time"
)

const top500FixtureLiveLogin = "shroud"

// FixtureTop500MetadataProvider returns deterministic local-only metadata for dry-run
// rehearsals. It is gated by TOP500_METADATA_FIXTURE_PROVIDER and must not be enabled
// in production profiles.
type FixtureTop500MetadataProvider struct{}

func NewFixtureTop500MetadataProvider() *FixtureTop500MetadataProvider {
	return &FixtureTop500MetadataProvider{}
}

func (p *FixtureTop500MetadataProvider) FetchStreams(_ context.Context, channels []Top500Channel) ([]Top500StreamMetadata, error) {
	if p == nil || len(channels) == 0 {
		return nil, nil
	}
	sampledAt := time.Now().UTC()
	startedAt := sampledAt.Add(-45 * time.Minute)
	viewerCount := 12500
	live := pickTop500FixtureLiveChannel(channels)
	if live.ChannelID == "" {
		return nil, nil
	}
	return []Top500StreamMetadata{{
		ChannelID:    strings.TrimSpace(live.ChannelID),
		Login:        firstNonEmpty(live.Login, top500FixtureLiveLogin),
		StreamID:     "fixture-stream-live",
		Title:        "LOAD-002a.1 fixture live metadata",
		CategoryName: "Just Chatting",
		StartedAt:    &startedAt,
		ViewerCount:  &viewerCount,
		Language:     "en",
		Tags:         []string{"English", "fixture"},
		SampledAt:    sampledAt,
	}}, nil
}

func (p *FixtureTop500MetadataProvider) FetchUsers(_ context.Context, channels []Top500Channel) ([]Top500UserMetadata, error) {
	if p == nil || len(channels) == 0 {
		return nil, nil
	}
	out := make([]Top500UserMetadata, 0, len(channels))
	for _, channel := range channels {
		login := normalizeLogin(channel.Login)
		if login == "" {
			continue
		}
		out = append(out, Top500UserMetadata{
			ChannelID:   strings.TrimSpace(channel.ChannelID),
			Login:       login,
			DisplayName: firstNonEmpty(channel.DisplayName, login),
		})
	}
	return out, nil
}

func pickTop500FixtureLiveChannel(channels []Top500Channel) Top500Channel {
	sorted := append([]Top500Channel(nil), channels...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Rank != sorted[j].Rank {
			return sorted[i].Rank < sorted[j].Rank
		}
		return normalizeLogin(sorted[i].Login) < normalizeLogin(sorted[j].Login)
	})
	for _, channel := range sorted {
		if normalizeLogin(channel.Login) == top500FixtureLiveLogin {
			return channel
		}
	}
	if len(sorted) > 0 {
		return sorted[0]
	}
	return Top500Channel{}
}
