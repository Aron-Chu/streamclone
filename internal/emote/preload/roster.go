package preload

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"streamclone/internal/emote/seeder"
	"streamclone/internal/metadata/helix"
	"streamclone/internal/metrics"
)

type streamItem struct {
	ID    string `json:"id"`
	Login string `json:"login"`
}

type streamsResponse struct {
	Items []streamItem `json:"items"`
}

// RosterPreloader warms emote dictionaries for directory top-N plus always-tracked logins.
type RosterPreloader struct {
	metadataURL string
	topN        int
	always      []string
	seed        *seeder.Seeder
	twitch      *helix.Client
	log         *slog.Logger
	httpClient  *http.Client
}

func NewRosterPreloader(
	metadataURL string,
	topN int,
	always []string,
	seed *seeder.Seeder,
	twitch *helix.Client,
	log *slog.Logger,
) *RosterPreloader {
	if topN <= 0 {
		topN = 200
	}
	return &RosterPreloader{
		metadataURL: strings.TrimRight(strings.TrimSpace(metadataURL), "/"),
		topN:        topN,
		always:      normalizeLogins(always),
		seed:        seed,
		twitch:      twitch,
		log:         log,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
	}
}

func normalizeLogins(logins []string) []string {
	out := make([]string, 0, len(logins))
	seen := make(map[string]struct{}, len(logins))
	for _, raw := range logins {
		login := strings.ToLower(strings.TrimSpace(raw))
		if login == "" {
			continue
		}
		if _, ok := seen[login]; ok {
			continue
		}
		seen[login] = struct{}{}
		out = append(out, login)
	}
	return out
}

// MergeTargets returns deduplicated login→twitchID targets (always-tracked first).
func MergeTargets(top []streamItem, always []string) []streamItem {
	seen := make(map[string]struct{})
	out := make([]streamItem, 0, len(top)+len(always))

	add := func(login, id string) {
		login = strings.ToLower(strings.TrimSpace(login))
		id = strings.TrimSpace(id)
		if login == "" {
			return
		}
		if _, ok := seen[login]; ok {
			return
		}
		seen[login] = struct{}{}
		out = append(out, streamItem{ID: id, Login: login})
	}

	for _, login := range always {
		add(login, "")
	}
	for _, item := range top {
		add(item.Login, item.ID)
	}
	return out
}

func (p *RosterPreloader) fetchTopStreams(ctx context.Context) ([]streamItem, error) {
	if p.metadataURL == "" {
		return nil, fmt.Errorf("metadata service url not configured")
	}
	url := fmt.Sprintf("%s/v1/streams?limit=%d", p.metadataURL, p.topN)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("metadata streams status %d", resp.StatusCode)
	}
	var page streamsResponse
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, err
	}
	return page.Items, nil
}

func (p *RosterPreloader) resolveLogin(ctx context.Context, login string) (string, error) {
	if p.twitch == nil || !p.twitch.Enabled() {
		return "", fmt.Errorf("twitch helix not configured")
	}
	details, err := p.twitch.ChannelDetails(ctx, login)
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(details.ID)
	if id == "" {
		return "", fmt.Errorf("empty twitch id for %q", login)
	}
	return id, nil
}

func (p *RosterPreloader) RunOnce(ctx context.Context) (int, error) {
	top, err := p.fetchTopStreams(ctx)
	if err != nil {
		return 0, err
	}
	targets := MergeTargets(top, p.always)
	providers := []seeder.Provider{seeder.ProviderSevenTV, seeder.ProviderFFZ}
	var warmed int
	for _, target := range targets {
		if ctx.Err() != nil {
			return warmed, ctx.Err()
		}
		twitchID := target.ID
		login := target.Login
		if twitchID == "" {
			twitchID, err = p.resolveLogin(ctx, login)
			if err != nil {
				if p.log != nil {
					p.log.Warn("roster preload skip", "login", login, "err", err)
				}
				continue
			}
		}
		if _, err := p.seed.SeedChannelProviders(ctx, login, twitchID, providers); err != nil {
			if p.log != nil {
				p.log.Warn("roster preload seed failed", "login", login, "err", err)
			}
			continue
		}
		warmed++
		if p.log != nil {
			p.log.Info("roster preload warmed", "login", login, "twitch_id", twitchID)
		}
	}
	return warmed, nil
}

func StartRosterPreloader(ctx context.Context, p *RosterPreloader, interval time.Duration, log *slog.Logger) {
	StartRosterPreloaderAfterInitial(ctx, p, interval, log, nil)
}

// StartRosterPreloaderAfterInitial runs the active-roster preload immediately,
// invokes afterInitial once the attempt completes, then continues on the normal
// interval. The hook lets cache migrations protect the active roster before
// attaching expiries to historical keys without delaying service startup.
func StartRosterPreloaderAfterInitial(
	ctx context.Context,
	p *RosterPreloader,
	interval time.Duration,
	log *slog.Logger,
	afterInitial func(),
) {
	if p == nil || interval <= 0 {
		return
	}
	go func() {
		run := func() {
			runCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
			defer cancel()
			n, err := p.RunOnce(runCtx)
			if err != nil {
				metrics.EmoteRosterPreloadRuns.WithLabelValues("error").Inc()
				metrics.EmoteRosterPreloadLastWarmed.Set(float64(n))
				if log != nil {
					log.Warn("roster preload cycle failed", "warmed_channels", n, "err", err)
				}
				return
			}
			metrics.EmoteRosterPreloadRuns.WithLabelValues("success").Inc()
			metrics.EmoteRosterPreloadChannelsWarmed.Add(float64(n))
			metrics.EmoteRosterPreloadLastWarmed.Set(float64(n))
			if log != nil {
				log.Info("roster preload cycle complete", "channels", n)
			}
		}
		run()
		if afterInitial != nil {
			afterInitial()
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
}
