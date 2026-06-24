package analytics

import (
	"context"
	"log/slog"
	"strings"
	"time"
)

type ProtectedGoLivePoller struct {
	store     *Store
	helix     *HelixClient
	collector *Collector
	runtime   PulseRuntimeConfig
	log       *slog.Logger
	interval  time.Duration
	batchSize int
}

func NewProtectedGoLivePoller(store *Store, helix *HelixClient, collector *Collector, runtime PulseRuntimeConfig, log *slog.Logger) *ProtectedGoLivePoller {
	if log == nil {
		log = slog.Default()
	}
	runtime = runtime.withDefaults()
	interval := runtime.ProtectedGoLiveInterval
	if interval <= 0 {
		interval = 60 * time.Second
	}
	batchSize := runtime.GoLiveBatchSize
	if batchSize <= 0 {
		batchSize = 100
	}
	return &ProtectedGoLivePoller{
		store:     store,
		helix:     helix,
		collector: collector,
		runtime:   runtime,
		log:       log,
		interval:  interval,
		batchSize: batchSize,
	}
}

func (p *ProtectedGoLivePoller) Enabled() bool {
	if p == nil {
		return false
	}
	runtime := p.runtime.withDefaults()
	return runtime.ProtectedGoLiveEnabled && runtime.HelixGoLiveEnabled
}

func StartProtectedGoLivePoller(ctx context.Context, poller *ProtectedGoLivePoller, log *slog.Logger) {
	if poller == nil || !poller.Enabled() {
		return
	}
	if log == nil {
		log = slog.Default()
	}
	log.Info("protected go-live poller started", "interval", poller.interval.String(), "batch_size", poller.batchSize)
	go func() {
		ticker := time.NewTicker(poller.interval)
		defer ticker.Stop()
		poller.runOnce(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				poller.runOnce(ctx)
			}
		}
	}()
}

func (p *ProtectedGoLivePoller) runOnce(ctx context.Context) {
	if p == nil || p.store == nil || p.helix == nil || !p.helix.Enabled() {
		return
	}
	if err := p.store.RefreshProtectedGoLiveRoster(ctx); err != nil {
		p.log.Warn("protected go-live roster refresh failed", "err", err)
		return
	}
	rows, err := p.store.ListPulseRosterDue(ctx, p.batchSize)
	if err != nil {
		p.log.Warn("protected go-live roster list failed", "err", err)
		return
	}
	if len(rows) == 0 {
		return
	}
	logins := make([]string, 0, len(rows))
	byLogin := make(map[string]PulseRosterState, len(rows))
	for _, row := range rows {
		login := normalizeLogin(row.Login)
		if login == "" {
			continue
		}
		logins = append(logins, login)
		byLogin[login] = row
	}
	if len(logins) == 0 {
		return
	}

	nextPoll := time.Now().UTC().Add(p.interval)
	missingIDs := make([]string, 0)
	for _, login := range logins {
		row := byLogin[login]
		if strings.TrimSpace(row.BroadcasterID) == "" {
			missingIDs = append(missingIDs, login)
		}
	}
	if len(missingIDs) > 0 {
		users, err := p.helix.UsersByLogin(ctx, missingIDs)
		if err != nil {
			p.log.Warn("protected go-live broadcaster lookup failed", "err", err)
			for _, login := range missingIDs {
				_ = p.store.UpdatePulseRosterPoll(ctx, login, "", "", "helix_users_error", time.Time{}, nextPoll)
			}
		} else {
			for login, profile := range users {
				row := byLogin[login]
				row.BroadcasterID = profile.ID
				byLogin[login] = row
			}
		}
	}

	liveStreams, err := p.helix.StreamsByLogin(ctx, logins)
	if err != nil {
		p.log.Warn("protected go-live helix streams failed", "err", err)
		for _, login := range logins {
			_ = p.store.UpdatePulseRosterPoll(ctx, login, "", "", "helix_streams_error", time.Time{}, nextPoll)
		}
		return
	}

	now := time.Now().UTC()
	for _, login := range logins {
		row := byLogin[login]
		stream, live := liveStreams[login]
		broadcasterID := strings.TrimSpace(row.BroadcasterID)
		if live && strings.TrimSpace(stream.BroadcasterID) != "" {
			broadcasterID = strings.TrimSpace(stream.BroadcasterID)
		}

		lastStreamID := strings.TrimSpace(row.LastLiveStreamID)
		currentStreamID := ""
		if live {
			currentStreamID = strings.TrimSpace(stream.ID)
		}
		seenAt := time.Time{}
		if live {
			seenAt = now
		}

		goLive := live && currentStreamID != "" && currentStreamID != lastStreamID
		if goLive && p.collector != nil {
			priority := row.Priority
			if priority <= 0 {
				priority = TrackPriorityPrincipalAlwaysTrack
			}
			duplicate := p.collector.TrackedStreamID(login) == currentStreamID
			if duplicate {
				p.log.Debug("protected go-live duplicate stream observation", "login", login, "stream_id", currentStreamID)
			} else {
				p.log.Info("protected go-live detected", "login", login, "stream_id", currentStreamID, "priority", priority)
				p.collector.WatchWithPriority(ctx, login, "", priority)
			}
			p.collector.NoteGoLiveDetected(currentStreamID, login, row.Source, priority, duplicate)
		}

		updateStreamID := lastStreamID
		if currentStreamID != "" {
			updateStreamID = currentStreamID
		}
		_ = p.store.UpdatePulseRosterPoll(ctx, login, broadcasterID, updateStreamID, "", seenAt, nextPoll)
	}
}
