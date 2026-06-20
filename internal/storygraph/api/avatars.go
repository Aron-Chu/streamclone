package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"streamclone/internal/config"
	"streamclone/internal/storygraph/store"
)

const avatarCacheTTL = 24 * time.Hour

type avatarEnricher struct {
	cfg    config.Config
	redis  *redis.Client
	client *http.Client
}

func newAvatarEnricher(cfg config.Config, rdb *redis.Client) *avatarEnricher {
	if rdb == nil {
		return nil
	}
	return &avatarEnricher{
		cfg:    cfg,
		redis:  rdb,
		client: &http.Client{Timeout: 8 * time.Second},
	}
}

func (e *avatarEnricher) enrichCards(ctx context.Context, cards []store.StoryCard) {
	if e == nil {
		return
	}
	for i := range cards {
		if cards[i].Entity == nil {
			continue
		}
		login := strings.TrimSpace(cards[i].Entity.TwitchLogin)
		if login == "" {
			continue
		}
		if url, err := e.avatarForLogin(ctx, login); err == nil && url != "" {
			cards[i].Entity.AvatarURL = url
		}
	}
}

func (e *avatarEnricher) enrichCard(ctx context.Context, card *store.StoryCard) {
	if e == nil || card == nil || card.Entity == nil {
		return
	}
	login := strings.TrimSpace(card.Entity.TwitchLogin)
	if login == "" {
		return
	}
	if url, err := e.avatarForLogin(ctx, login); err == nil && url != "" {
		card.Entity.AvatarURL = url
	}
}

func (e *avatarEnricher) avatarForLogin(ctx context.Context, login string) (string, error) {
	key := "pulsewire:avatar:" + strings.ToLower(login)
	if cached, err := e.redis.Get(ctx, key).Result(); err == nil && strings.TrimSpace(cached) != "" {
		return cached, nil
	}
	base := strings.TrimRight(strings.TrimSpace(e.cfg.MetadataServiceURL), "/")
	if base == "" {
		return "", fmt.Errorf("metadata url not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1/channels/"+login+"/details", nil)
	if err != nil {
		return "", err
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("metadata status %d", resp.StatusCode)
	}
	var details struct {
		ProfileImage string `json:"profileImage"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&details); err != nil {
		return "", err
	}
	url := strings.TrimSpace(details.ProfileImage)
	if url == "" {
		return "", nil
	}
	_ = e.redis.Set(ctx, key, url, avatarCacheTTL).Err()
	return url, nil
}
