package analytics

import (
	"context"
	"errors"
	"strings"
	"time"
)

// HelixTop500MetadataProvider maps Helix stream/user responses into Top 500 metadata
// samples. When Helix credentials are absent the provider returns typed errors without
// performing network I/O.
type HelixTop500MetadataProvider struct {
	helix *HelixClient
}

func NewHelixTop500MetadataProvider(helix *HelixClient) *HelixTop500MetadataProvider {
	return &HelixTop500MetadataProvider{helix: helix}
}

func (p *HelixTop500MetadataProvider) FetchStreams(ctx context.Context, channels []Top500Channel) ([]Top500StreamMetadata, error) {
	if p == nil || p.helix == nil || !p.helix.Enabled() {
		return nil, ErrTop500ProviderAuthMissing
	}
	logins := top500ChannelLogins(channels)
	if len(logins) == 0 {
		return nil, nil
	}
	liveByLogin, err := p.helix.StreamsByLogin(ctx, logins)
	if err != nil {
		return nil, mapHelixErrorToTop500(err)
	}
	sampledAt := time.Now().UTC()
	out := make([]Top500StreamMetadata, 0, len(liveByLogin))
	for _, channel := range channels {
		login := normalizeLogin(channel.Login)
		stream, ok := liveByLogin[login]
		if !ok {
			continue
		}
		viewerCount := stream.ViewerCount
		startedAt := stream.StartedAt
		out = append(out, Top500StreamMetadata{
			ChannelID:    strings.TrimSpace(channel.ChannelID),
			Login:        firstNonEmpty(stream.Login, login),
			StreamID:     stream.ID,
			Title:        stream.Title,
			CategoryName: stream.GameName,
			StartedAt:    &startedAt,
			ViewerCount:  &viewerCount,
			Language:     stream.Language,
			Tags:         append([]string(nil), stream.Tags...),
			SampledAt:    sampledAt,
		})
	}
	return out, nil
}

func (p *HelixTop500MetadataProvider) FetchUsers(ctx context.Context, channels []Top500Channel) ([]Top500UserMetadata, error) {
	if p == nil || p.helix == nil || !p.helix.Enabled() {
		return nil, ErrTop500ProviderAuthMissing
	}
	logins := top500ChannelLogins(channels)
	if len(logins) == 0 {
		return nil, nil
	}
	usersByLogin, err := p.helix.UsersByLogin(ctx, logins)
	if err != nil {
		return nil, mapHelixErrorToTop500(err)
	}
	out := make([]Top500UserMetadata, 0, len(channels))
	for _, channel := range channels {
		login := normalizeLogin(channel.Login)
		user, ok := usersByLogin[login]
		if !ok {
			out = append(out, Top500UserMetadata{
				ChannelID:   strings.TrimSpace(channel.ChannelID),
				Login:       login,
				DisplayName: firstNonEmpty(channel.DisplayName, login),
			})
			continue
		}
		out = append(out, Top500UserMetadata{
			ChannelID:   strings.TrimSpace(channel.ChannelID),
			Login:       firstNonEmpty(user.Login, login),
			DisplayName: firstNonEmpty(user.DisplayName, channel.DisplayName, login),
		})
	}
	return out, nil
}

func top500ChannelLogins(channels []Top500Channel) []string {
	logins := make([]string, 0, len(channels))
	seen := make(map[string]struct{}, len(channels))
	for _, channel := range channels {
		login := normalizeLogin(channel.Login)
		if login == "" {
			continue
		}
		if _, ok := seen[login]; ok {
			continue
		}
		seen[login] = struct{}{}
		logins = append(logins, login)
	}
	return logins
}

func mapHelixErrorToTop500(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrHelixDisabled) {
		return ErrTop500ProviderAuthMissing
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "status 429"):
		return ErrTop500ProviderRateLimited
	case strings.Contains(msg, "status 401"), strings.Contains(msg, "status 403"):
		return ErrTop500ProviderAuthMissing
	case strings.Contains(msg, "status 404"):
		return ErrTop500ProviderNotFound
	default:
		return ErrTop500ProviderTransient
	}
}
