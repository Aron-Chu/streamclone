package analytics

import (
	"context"
	"log/slog"
	"time"
)

type EmoteHistoryJobConfig struct {
	SnapshotEnabled    bool
	SnapshotInterval   time.Duration
	SnapshotBatchSize  int
	NormalizeEnabled   bool
	NormalizeInterval  time.Duration
	NormalizeSince     time.Duration
	NormalizeBatchSize int
}

type EmoteSnapshotCandidate struct {
	TwitchID string
	Login    string
}

func StartEmoteHistoryJobs(ctx context.Context, store *Store, snapshotProvider EmoteSnapshotProvider, cfg EmoteHistoryJobConfig, log *slog.Logger) {
	if store == nil || (!cfg.SnapshotEnabled && !cfg.NormalizeEnabled) {
		return
	}
	if log == nil {
		log = slog.Default()
	}
	if cfg.SnapshotEnabled && snapshotProvider != nil {
		poller := NewEmoteSnapshotPoller(store, snapshotProvider, log)
		interval := cfg.SnapshotInterval
		if interval <= 0 {
			interval = 6 * time.Hour
		}
		batchSize := cfg.SnapshotBatchSize
		if batchSize <= 0 {
			batchSize = 25
		}
		log.Info("emote history snapshot poller started", "interval", interval.String(), "batch_size", batchSize)
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			runEmoteSnapshotBatch(ctx, poller, batchSize, log)
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					runEmoteSnapshotBatch(ctx, poller, batchSize, log)
				}
			}
		}()
	} else if cfg.SnapshotEnabled {
		log.Warn("emote history snapshot poller disabled; no provider configured")
	}
	if cfg.NormalizeEnabled {
		normalizer := NewEmoteUsageNormalizer(store, log)
		interval := cfg.NormalizeInterval
		if interval <= 0 {
			interval = 15 * time.Minute
		}
		batchSize := cfg.NormalizeBatchSize
		if batchSize <= 0 {
			batchSize = 25
		}
		since := cfg.NormalizeSince
		if since <= 0 {
			since = 30 * 24 * time.Hour
		}
		log.Info("emote history usage normalizer started", "interval", interval.String(), "since", since.String(), "batch_size", batchSize)
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			runEmoteUsageNormalizeBatch(ctx, normalizer, since, batchSize, log)
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					runEmoteUsageNormalizeBatch(ctx, normalizer, since, batchSize, log)
				}
			}
		}()
	}
}

func runEmoteSnapshotBatch(ctx context.Context, poller *EmoteSnapshotPoller, batchSize int, log *slog.Logger) {
	if poller == nil || poller.store == nil {
		return
	}
	candidates, err := poller.store.ListEmoteSnapshotCandidates(ctx, batchSize)
	if err != nil {
		log.Warn("emote snapshot candidate list failed", "err", err)
		return
	}
	for _, candidate := range candidates {
		result, err := poller.SnapshotChannel(ctx, candidate.TwitchID, candidate.Login)
		if err != nil {
			log.Warn("emote snapshot failed", "login", candidate.Login, "twitch_id", candidate.TwitchID, "err", err)
			continue
		}
		if result.Created {
			log.Info("emote snapshot saved", "login", candidate.Login, "twitch_id", candidate.TwitchID, "snapshot_id", result.SnapshotID)
		}
	}
}

func runEmoteUsageNormalizeBatch(ctx context.Context, normalizer *EmoteUsageNormalizer, sinceDuration time.Duration, batchSize int, log *slog.Logger) {
	if normalizer == nil || normalizer.store == nil {
		return
	}
	since := time.Now().UTC().Add(-sinceDuration)
	logins, err := normalizer.store.ListEmoteUsageNormalizeLogins(ctx, since, batchSize)
	if err != nil {
		log.Warn("emote usage normalize login list failed", "err", err)
		return
	}
	for _, login := range logins {
		result, err := normalizer.NormalizeChannelRange(ctx, login, since)
		if err != nil {
			log.Warn("emote usage normalization failed", "login", login, "err", err)
			continue
		}
		if result.Rows > 0 {
			log.Info("emote usage normalized", "login", login, "streams", result.Streams, "minutes", result.Minutes, "rows", result.Rows)
		}
	}
}

func (s *Store) ListEmoteSnapshotCandidates(ctx context.Context, limit int) ([]EmoteSnapshotCandidate, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	if limit <= 0 || limit > 500 {
		limit = 25
	}
	rows, err := s.db.Query(ctx, `
		WITH candidates AS (
			SELECT twitch_id::text, lower(login)::text AS login
			FROM channels
			WHERE twitch_id <> '' AND login <> ''
			UNION
			SELECT broadcaster_id::text AS twitch_id, lower(login)::text AS login
			FROM analytics_streams
			WHERE broadcaster_id <> '' AND broadcaster_id <> 'pending' AND login <> ''
			GROUP BY broadcaster_id, lower(login)
		)
		SELECT c.twitch_id, c.login
		FROM candidates c
		LEFT JOIN channel_emote_providers p ON p.twitch_id=c.twitch_id AND p.provider='seventv'
		WHERE COALESCE(p.next_snapshot_after, now()) <= now()
		ORDER BY COALESCE(p.last_snapshot_at, 'epoch'::timestamptz), c.login
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EmoteSnapshotCandidate{}
	for rows.Next() {
		var candidate EmoteSnapshotCandidate
		if err := rows.Scan(&candidate.TwitchID, &candidate.Login); err != nil {
			return nil, err
		}
		out = append(out, candidate)
	}
	return out, rows.Err()
}

func (s *Store) ListEmoteUsageNormalizeLogins(ctx context.Context, since time.Time, limit int) ([]string, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	if limit <= 0 || limit > 500 {
		limit = 25
	}
	rows, err := s.db.Query(ctx, `
		SELECT lower(s.login) AS login
		FROM analytics_minute_rollups r
		JOIN analytics_streams s ON s.stream_id=r.stream_id
		WHERE s.login <> '' AND r.minute_ts >= $1 AND r.emotes_json <> '{}'::jsonb
		GROUP BY lower(s.login)
		ORDER BY MAX(r.minute_ts) DESC
		LIMIT $2`, since.UTC(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	logins := []string{}
	for rows.Next() {
		var login string
		if err := rows.Scan(&login); err != nil {
			return nil, err
		}
		if login = normalizeLogin(login); login != "" {
			logins = append(logins, login)
		}
	}
	return logins, rows.Err()
}
