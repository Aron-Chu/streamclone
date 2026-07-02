package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type EmoteSnapshotProvider interface {
	SnapshotChannelEmotes(ctx context.Context, twitchID string) (EmoteProviderSnapshot, error)
}

type EmoteProviderSnapshot struct {
	Provider      string
	ProviderSetID string
	Items         []EmoteSnapshotItem
	FetchedAt     time.Time
	EffectiveAt   time.Time
	Complete      bool
	HTTPStatus    int
	Source        string
}

type SaveEmoteSnapshotInput struct {
	TwitchID      string
	Login         string
	Provider      string
	ProviderSetID string
	Items         []EmoteSnapshotItem
	FetchedAt     time.Time
	EffectiveAt   time.Time
	Complete      bool
	HTTPStatus    int
	Source        string
}

type SaveEmoteSnapshotResult struct {
	SnapshotID   string
	Created      bool
	Unchanged    bool
	SnapshotHash string
	Diff         EmoteSnapshotDiff
}

type EmoteSnapshotFailureInput struct {
	TwitchID          string
	Login             string
	Provider          string
	State             string
	Error             string
	HTTPStatus        int
	RateLimitedUntil  *time.Time
	NextSnapshotAfter *time.Time
}

type EmoteUsageNormalizer struct {
	store *Store
	log   *slog.Logger
}

type EmoteSnapshotPoller struct {
	store    *Store
	provider EmoteSnapshotProvider
	log      *slog.Logger
}

type EmoteUsageNormalizeResult struct {
	Streams int
	Minutes int
	Rows    int
}

type PortalChannelEmotesResponse struct {
	Login                 string               `json:"login"`
	Range                 string               `json:"range"`
	AsOf                  time.Time            `json:"asOf"`
	Coverage              PortalEmoteCoverage  `json:"coverage"`
	Freshness             PortalEmoteFreshness `json:"freshness"`
	IdentityResolutionPct float64              `json:"identityResolutionPct"`
	TotalEmoteUses        int64                `json:"totalEmoteUses"`
	EmotesPerMinute       float64              `json:"emotesPerMinute"`
	SevenTVSharePct       float64              `json:"sevenTvSharePct"`
	UniqueEmotes          int                  `json:"uniqueEmotes"`
	TopEmotes             []PortalChannelEmote `json:"topEmotes"`
	History               []PortalEmoteHistory `json:"history"`
	TopMoments            []PortalEmoteMoment  `json:"topMoments"`
	Partial               bool                 `json:"partial"`
	LowConfidence         bool                 `json:"lowConfidence"`
	Sources               []SourceStatus       `json:"sources"`
	UpdatedAt             int64                `json:"updatedAt"`
}

type PortalEmoteCoverage struct {
	ChatCoveragePct      float64 `json:"chatCoveragePct"`
	MinutesWithData      int     `json:"minutesWithData"`
	NormalizedMinutes    int     `json:"normalizedMinutes"`
	IdentityResolvedRows int     `json:"identityResolvedRows"`
	IdentityTotalRows    int     `json:"identityTotalRows"`
}

type PortalEmoteFreshness struct {
	LatestUsageAt        *time.Time `json:"latestUsageAt,omitempty"`
	LatestSnapshotAt     *time.Time `json:"latestSnapshotAt,omitempty"`
	ProviderStalenessSec int64      `json:"providerStalenessSec,omitempty"`
	UsageStalenessSec    int64      `json:"usageStalenessSec,omitempty"`
	ProviderState        string     `json:"providerState"`
	ProviderError        string     `json:"providerError,omitempty"`
}

type PortalChannelEmote struct {
	Provider           string     `json:"provider"`
	ProviderEmoteID    string     `json:"providerEmoteId"`
	Name               string     `json:"name"`
	ImageURL           string     `json:"imageUrl,omitempty"`
	UseCount           int64      `json:"useCount"`
	MinutesSeen        int        `json:"minutesSeen"`
	SharePct           float64    `json:"sharePct"`
	IdentityResolution string     `json:"identityResolution"`
	Confidence         float64    `json:"confidence"`
	FirstSeenAt        *time.Time `json:"firstSeenAt,omitempty"`
	LastSeenAt         *time.Time `json:"lastSeenAt,omitempty"`
}

type PortalEmoteHistory struct {
	Day             time.Time `json:"day"`
	UseCount        int64     `json:"useCount"`
	UniqueEmotes    int       `json:"uniqueEmotes"`
	SevenTVUseCount int64     `json:"sevenTvUseCount"`
}

type PortalEmoteMoment struct {
	StreamID        string    `json:"streamId"`
	StartedAt       time.Time `json:"startedAt"`
	OffsetSeconds   int       `json:"offsetSeconds"`
	Href            string    `json:"href"`
	UseCount        int       `json:"useCount"`
	TopEmoteName    string    `json:"topEmoteName"`
	Provider        string    `json:"provider"`
	ProviderEmoteID string    `json:"providerEmoteId"`
}

func NewEmoteUsageNormalizer(store *Store, log *slog.Logger) *EmoteUsageNormalizer {
	return &EmoteUsageNormalizer{store: store, log: log}
}

func NewEmoteSnapshotPoller(store *Store, provider EmoteSnapshotProvider, log *slog.Logger) *EmoteSnapshotPoller {
	return &EmoteSnapshotPoller{store: store, provider: provider, log: log}
}

func (p *EmoteSnapshotPoller) SnapshotChannel(ctx context.Context, twitchID, login string) (SaveEmoteSnapshotResult, error) {
	if p == nil || p.store == nil || p.provider == nil {
		return SaveEmoteSnapshotResult{}, errors.New("emote snapshot poller unavailable")
	}
	snap, err := p.provider.SnapshotChannelEmotes(ctx, twitchID)
	provider := normalizeProvider(snap.Provider)
	if provider == "" {
		provider = "seventv"
	}
	if err != nil {
		_ = p.store.RecordEmoteSnapshotFailure(ctx, EmoteSnapshotFailureInput{TwitchID: twitchID, Login: login, Provider: provider, State: "failed", Error: err.Error()})
		return SaveEmoteSnapshotResult{}, err
	}
	return p.store.SaveEmoteSnapshot(ctx, SaveEmoteSnapshotInput{
		TwitchID:      twitchID,
		Login:         login,
		Provider:      provider,
		ProviderSetID: snap.ProviderSetID,
		Items:         snap.Items,
		FetchedAt:     snap.FetchedAt,
		EffectiveAt:   snap.EffectiveAt,
		Complete:      snap.Complete,
		HTTPStatus:    snap.HTTPStatus,
		Source:        snap.Source,
	})
}

func (s *Store) SaveEmoteSnapshot(ctx context.Context, in SaveEmoteSnapshotInput) (SaveEmoteSnapshotResult, error) {
	if s == nil || s.db == nil {
		return SaveEmoteSnapshotResult{}, errors.New("store unavailable")
	}
	in.TwitchID = strings.TrimSpace(in.TwitchID)
	in.Login = normalizeLogin(in.Login)
	in.Provider = normalizeProvider(in.Provider)
	if in.TwitchID == "" || in.Provider == "" {
		return SaveEmoteSnapshotResult{}, errors.New("missing channel or provider")
	}
	if in.FetchedAt.IsZero() {
		in.FetchedAt = emoteHistoryNow()
	}
	if in.EffectiveAt.IsZero() {
		in.EffectiveAt = in.FetchedAt
	}
	if in.Source == "" {
		in.Source = "snapshot_poll"
	}
	if in.Login != "" {
		if err := s.ensureEmoteHistoryChannel(ctx, in.TwitchID, in.Login); err != nil {
			return SaveEmoteSnapshotResult{}, err
		}
	}
	items := NormalizeEmoteSnapshotItems(in.Provider, in.Items)
	hash := StableEmoteSnapshotHash(items)
	complete := in.Complete
	if !complete {
		_ = s.RecordEmoteSnapshotFailure(ctx, EmoteSnapshotFailureInput{TwitchID: in.TwitchID, Login: in.Login, Provider: in.Provider, State: "partial", Error: "incomplete snapshot", HTTPStatus: in.HTTPStatus})
		return SaveEmoteSnapshotResult{SnapshotHash: hash}, nil
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return SaveEmoteSnapshotResult{}, err
	}
	defer tx.Rollback(ctx)
	prevHash, _, err := latestEmoteSnapshotHashTx(ctx, tx, in.TwitchID, in.Provider)
	if err != nil {
		return SaveEmoteSnapshotResult{}, err
	}
	if !SnapshotShouldCreateHistory(prevHash, hash, true) {
		if err := updateEmoteProviderSnapshotStateTx(ctx, tx, in.TwitchID, in.Provider, "ready", len(items), hash, nil, nil, nil); err != nil {
			return SaveEmoteSnapshotResult{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return SaveEmoteSnapshotResult{}, err
		}
		return SaveEmoteSnapshotResult{Unchanged: true, SnapshotHash: hash}, nil
	}
	previous, err := latestCompleteSnapshotItemsTx(ctx, tx, in.TwitchID, in.Provider)
	if err != nil {
		return SaveEmoteSnapshotResult{}, err
	}
	seen, err := seenEmoteIdentityKeysTx(ctx, tx, in.TwitchID, in.Provider)
	if err != nil {
		return SaveEmoteSnapshotResult{}, err
	}
	diff := DiffEmoteSnapshots(previous, items, seen)
	snapshotID, err := insertEmoteSnapshotTx(ctx, tx, in, len(items), hash)
	if err != nil {
		return SaveEmoteSnapshotResult{}, err
	}
	if err := insertEmoteSnapshotItemsTx(ctx, tx, snapshotID, in.TwitchID, items); err != nil {
		return SaveEmoteSnapshotResult{}, err
	}
	if err := applyEmoteSnapshotDiffTx(ctx, tx, snapshotID, in, diff); err != nil {
		return SaveEmoteSnapshotResult{}, err
	}
	if err := updateEmoteProviderSnapshotStateTx(ctx, tx, in.TwitchID, in.Provider, "ready", len(items), hash, &snapshotID, nil, nil); err != nil {
		return SaveEmoteSnapshotResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SaveEmoteSnapshotResult{}, err
	}
	return SaveEmoteSnapshotResult{SnapshotID: snapshotID, Created: true, SnapshotHash: hash, Diff: diff}, nil
}

func (s *Store) RecordEmoteSnapshotFailure(ctx context.Context, in EmoteSnapshotFailureInput) error {
	if s == nil || s.db == nil {
		return nil
	}
	in.TwitchID = strings.TrimSpace(in.TwitchID)
	in.Login = normalizeLogin(in.Login)
	in.Provider = normalizeProvider(in.Provider)
	if in.TwitchID == "" || in.Provider == "" {
		return nil
	}
	if in.Login != "" {
		if err := s.ensureEmoteHistoryChannel(ctx, in.TwitchID, in.Login); err != nil {
			return err
		}
	}
	state := strings.TrimSpace(in.State)
	if state == "" {
		state = "failed"
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO channel_emote_providers (twitch_id, provider, state, count, last_error, updated_at, snapshot_state, snapshot_error, rate_limited_until, next_snapshot_after)
		VALUES ($1,$2,$3::varchar,0,$4,now(),$3::text,$4,$5,$6)
		ON CONFLICT (twitch_id, provider) DO UPDATE SET
			state=EXCLUDED.state,
			last_error=EXCLUDED.last_error,
			updated_at=now(),
			snapshot_state=EXCLUDED.snapshot_state,
			snapshot_error=EXCLUDED.snapshot_error,
			rate_limited_until=EXCLUDED.rate_limited_until,
			next_snapshot_after=EXCLUDED.next_snapshot_after`,
		in.TwitchID, in.Provider, state, strings.TrimSpace(in.Error), in.RateLimitedUntil, in.NextSnapshotAfter,
	)
	return err
}

func (s *Store) ensureEmoteHistoryChannel(ctx context.Context, twitchID, login string) error {
	if s == nil || s.db == nil || strings.TrimSpace(twitchID) == "" || normalizeLogin(login) == "" {
		return nil
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO channels (twitch_id, login, updated_at)
		VALUES ($1,$2,now())
		ON CONFLICT (twitch_id) DO UPDATE SET
			login=EXCLUDED.login,
			updated_at=now()`, strings.TrimSpace(twitchID), normalizeLogin(login))
	return err
}

func (n *EmoteUsageNormalizer) NormalizeChannelRange(ctx context.Context, login string, since time.Time) (EmoteUsageNormalizeResult, error) {
	if n == nil || n.store == nil {
		return EmoteUsageNormalizeResult{}, errors.New("emote usage normalizer unavailable")
	}
	return n.store.NormalizeEmoteUsageForChannel(ctx, login, since)
}

func (s *Store) NormalizeEmoteUsageForChannel(ctx context.Context, login string, since time.Time) (EmoteUsageNormalizeResult, error) {
	login = normalizeLogin(login)
	if s == nil || s.db == nil || login == "" {
		return EmoteUsageNormalizeResult{}, nil
	}
	if since.IsZero() {
		since = time.Now().UTC().Add(-30 * 24 * time.Hour)
	}
	rows, err := s.db.Query(ctx, `
		SELECT s.stream_id, COALESCE(s.broadcaster_id,''), s.login, r.minute_ts, r.emotes_json
		FROM analytics_minute_rollups r
		JOIN analytics_streams s ON s.stream_id = r.stream_id
		WHERE s.login=$1 AND r.minute_ts >= $2 AND r.emotes_json <> '{}'::jsonb
		ORDER BY s.stream_id, r.minute_ts`, login, since.UTC())
	if err != nil {
		return EmoteUsageNormalizeResult{}, err
	}
	defer rows.Close()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return EmoteUsageNormalizeResult{}, err
	}
	defer tx.Rollback(ctx)
	streams := map[string]struct{}{}
	result := EmoteUsageNormalizeResult{}
	for rows.Next() {
		var streamID, twitchID, streamLogin string
		var minute time.Time
		var raw []byte
		if err := rows.Scan(&streamID, &twitchID, &streamLogin, &minute, &raw); err != nil {
			return EmoteUsageNormalizeResult{}, err
		}
		var emotes map[string]int
		if len(raw) > 0 {
			if err := jsonUnmarshalEmoteCounts(raw, &emotes); err != nil {
				return EmoteUsageNormalizeResult{}, fmt.Errorf("decode emotes_json for stream %s minute %s: %w", streamID, minute.UTC().Format(time.RFC3339), err)
			}
		}
		if len(emotes) == 0 {
			continue
		}
		streams[streamID] = struct{}{}
		result.Minutes++
		for sourceKey, count := range emotes {
			if count <= 0 {
				continue
			}
			parsed := ParseEmoteRollupKey(sourceKey)
			resolution, localID, err := resolveEmoteUsageIdentityTx(ctx, tx, twitchID, parsed, minute)
			if err != nil {
				return EmoteUsageNormalizeResult{}, err
			}
			if err := upsertEmoteUsageMinuteTx(ctx, tx, streamID, minute, twitchID, streamLogin, sourceKey, count, resolution, localID); err != nil {
				return EmoteUsageNormalizeResult{}, err
			}
			result.Rows++
		}
	}
	if err := rows.Err(); err != nil {
		return EmoteUsageNormalizeResult{}, err
	}
	for streamID := range streams {
		if err := refreshEmoteUsageStreamRollupsTx(ctx, tx, streamID); err != nil {
			return EmoteUsageNormalizeResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return EmoteUsageNormalizeResult{}, err
	}
	result.Streams = len(streams)
	return result, nil
}

func (s *Store) PortalChannelEmotes(ctx context.Context, login string, rangeDuration time.Duration) (PortalChannelEmotesResponse, error) {
	login = normalizeLogin(login)
	now := time.Now().UTC()
	if rangeDuration <= 0 {
		rangeDuration = 30 * 24 * time.Hour
	}
	rangeLabel := formatEmoteRange(rangeDuration)
	out := PortalChannelEmotesResponse{
		Login:     login,
		Range:     rangeLabel,
		AsOf:      now,
		Sources:   []SourceStatus{{Source: "analytics_db", State: "ready"}},
		UpdatedAt: now.UnixMilli(),
	}
	if s == nil || s.db == nil || login == "" {
		out.Sources = []SourceStatus{{Source: "analytics_db", State: "unavailable"}}
		out.Partial = true
		out.LowConfidence = true
		return out, nil
	}
	since := now.Add(-rangeDuration)
	var trackedMinutes, normalizedMinutes, identityResolvedRows, identityTotalRows int
	var latestUsage *time.Time
	if err := s.db.QueryRow(ctx, `
		WITH tracked AS (
			SELECT COUNT(*)::int AS tracked_minutes
			FROM analytics_minute_rollups r JOIN analytics_streams s ON s.stream_id=r.stream_id
			WHERE s.login=$1 AND r.minute_ts >= $2
		), norm AS (
			SELECT COUNT(DISTINCT stream_id || ':' || minute_ts)::int AS normalized_minutes,
			       COUNT(*)::int AS identity_total_rows,
			       COUNT(*) FILTER (WHERE identity_resolution IN ('provider_id','alias_fallback'))::int AS identity_resolved_rows,
			       MAX(minute_ts) AS latest_usage
			FROM emote_usage_minute_rollups
			WHERE login=$1 AND minute_ts >= $2
		)
		SELECT tracked.tracked_minutes, COALESCE(norm.normalized_minutes,0), COALESCE(norm.identity_resolved_rows,0), COALESCE(norm.identity_total_rows,0), norm.latest_usage
		FROM tracked CROSS JOIN norm`, login, since).Scan(&trackedMinutes, &normalizedMinutes, &identityResolvedRows, &identityTotalRows, &latestUsage); err != nil {
		return out, err
	}
	coveragePct := 0.0
	if trackedMinutes > 0 {
		coveragePct = clampPct(float64(normalizedMinutes) / float64(trackedMinutes) * 100)
	}
	identityPct := 0.0
	if identityTotalRows > 0 {
		identityPct = clampPct(float64(identityResolvedRows) / float64(identityTotalRows) * 100)
	}
	out.Coverage = PortalEmoteCoverage{ChatCoveragePct: round2(coveragePct), MinutesWithData: trackedMinutes, NormalizedMinutes: normalizedMinutes, IdentityResolvedRows: identityResolvedRows, IdentityTotalRows: identityTotalRows}
	out.IdentityResolutionPct = round2(identityPct)
	out.Freshness = s.portalEmoteFreshness(ctx, login, since, latestUsage, now)
	top, totals, err := s.portalTopChannelEmotes(ctx, login, since, rangeDuration)
	if err != nil {
		return out, err
	}
	out.TopEmotes = top
	out.TotalEmoteUses = totals.totalUses
	out.UniqueEmotes = totals.uniqueEmotes
	out.SevenTVSharePct = round2(totals.sevenTVSharePct)
	minutes := math.Max(1, float64(normalizedMinutes))
	out.EmotesPerMinute = round2(float64(totals.totalUses) / minutes)
	history, err := s.portalEmoteHistory(ctx, login, since)
	if err != nil {
		return out, err
	}
	out.History = history
	moments, err := s.portalEmoteMoments(ctx, login, since)
	if err != nil {
		return out, err
	}
	out.TopMoments = moments
	out.Partial = coveragePct < 80 || identityPct < 80 || out.Freshness.ProviderState != "ready"
	out.LowConfidence = coveragePct < 50 || identityPct < 70 || totals.totalUses == 0
	if out.LowConfidence {
		out.Sources = append(out.Sources, SourceStatus{Source: "emote_history", State: "partial", Message: "Coverage or identity resolution is incomplete"})
	} else {
		out.Sources = append(out.Sources, SourceStatus{Source: "emote_history", State: "ready"})
	}
	return out, nil
}

type portalEmoteTotals struct {
	totalUses       int64
	uniqueEmotes    int
	sevenTVSharePct float64
}

func latestEmoteSnapshotHashTx(ctx context.Context, tx pgx.Tx, twitchID, provider string) (string, string, error) {
	var hash, snapshotID string
	err := tx.QueryRow(ctx, `
		SELECT snapshot_hash, id::text
		FROM channel_emote_set_snapshots
		WHERE twitch_id=$1 AND provider=$2 AND state='complete'
		ORDER BY fetched_at DESC, created_at DESC
		LIMIT 1`, twitchID, provider).Scan(&hash, &snapshotID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", nil
	}
	return hash, snapshotID, err
}

func latestCompleteSnapshotItemsTx(ctx context.Context, tx pgx.Tx, twitchID, provider string) ([]EmoteSnapshotItem, error) {
	rows, err := tx.Query(ctx, `
		SELECT i.provider, i.provider_emote_id, i.provider_set_id, i.alias, i.canonical_name, i.source_url, i.asset_hash, i.flags, i.animated, i.zero_width
		FROM channel_emote_set_snapshots s
		JOIN channel_emote_set_snapshot_items i ON i.snapshot_id=s.id
		WHERE s.id = (
			SELECT id FROM channel_emote_set_snapshots
			WHERE twitch_id=$1 AND provider=$2 AND state='complete'
			ORDER BY fetched_at DESC, created_at DESC
			LIMIT 1
		)
		ORDER BY i.sort_key`, twitchID, provider)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []EmoteSnapshotItem{}
	for rows.Next() {
		var item EmoteSnapshotItem
		if err := rows.Scan(&item.Provider, &item.ProviderEmoteID, &item.ProviderSetID, &item.Alias, &item.CanonicalName, &item.SourceURL, &item.AssetHash, &item.Flags, &item.Animated, &item.ZeroWidth); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return NormalizeEmoteSnapshotItems(provider, items), rows.Err()
}

func seenEmoteIdentityKeysTx(ctx context.Context, tx pgx.Tx, twitchID, provider string) (map[string]struct{}, error) {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT provider, provider_emote_id
		FROM channel_emote_membership_periods
		WHERE twitch_id=$1 AND provider=$2`, twitchID, provider)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]struct{}{}
	for rows.Next() {
		var p, id string
		if err := rows.Scan(&p, &id); err != nil {
			return nil, err
		}
		out[normalizeProvider(p)+":"+strings.TrimSpace(id)] = struct{}{}
	}
	return out, rows.Err()
}

func insertEmoteSnapshotTx(ctx context.Context, tx pgx.Tx, in SaveEmoteSnapshotInput, itemCount int, hash string) (string, error) {
	var snapshotID string
	err := tx.QueryRow(ctx, `
		INSERT INTO channel_emote_set_snapshots (twitch_id, login, provider, provider_set_id, fetched_at, effective_at, item_count, snapshot_hash, state, source, http_status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'complete',$9,$10)
		RETURNING id::text`,
		in.TwitchID, in.Login, in.Provider, strings.TrimSpace(in.ProviderSetID), in.FetchedAt, in.EffectiveAt, itemCount, hash, strings.TrimSpace(in.Source), nullableInt(in.HTTPStatus),
	).Scan(&snapshotID)
	return snapshotID, err
}

func insertEmoteSnapshotItemsTx(ctx context.Context, tx pgx.Tx, snapshotID, twitchID string, items []EmoteSnapshotItem) error {
	for _, item := range items {
		_, err := tx.Exec(ctx, `
			INSERT INTO channel_emote_set_snapshot_items (snapshot_id, twitch_id, provider, provider_emote_id, provider_set_id, alias, canonical_name, source_url, asset_hash, flags, animated, zero_width, sort_key)
			VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
			snapshotID, twitchID, item.Provider, item.ProviderEmoteID, item.ProviderSetID, item.Alias, item.CanonicalName, item.SourceURL, item.AssetHash, item.Flags, item.Animated, item.ZeroWidth, item.SortKey,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func applyEmoteSnapshotDiffTx(ctx context.Context, tx pgx.Tx, snapshotID string, in SaveEmoteSnapshotInput, diff EmoteSnapshotDiff) error {
	for _, item := range diff.Removed {
		_, err := tx.Exec(ctx, `
			UPDATE channel_emote_membership_periods
			SET valid_to=$4, last_seen_by_us=$4, closed_snapshot_id=$5::uuid, updated_at=now()
			WHERE twitch_id=$1 AND provider=$2 AND provider_emote_id=$3 AND valid_to IS NULL`, in.TwitchID, item.Provider, item.ProviderEmoteID, in.EffectiveAt, snapshotID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			UPDATE emote_alias_history
			SET valid_to=$4, last_seen_by_us=$4, closed_snapshot_id=$5::uuid, updated_at=now()
			WHERE twitch_id=$1 AND provider=$2 AND provider_emote_id=$3 AND valid_to IS NULL`, in.TwitchID, item.Provider, item.ProviderEmoteID, in.EffectiveAt, snapshotID)
		if err != nil {
			return err
		}
	}
	for _, change := range diff.AliasChanges {
		_, err := tx.Exec(ctx, `
			UPDATE emote_alias_history
			SET valid_to=$4, last_seen_by_us=$4, closed_snapshot_id=$5::uuid, updated_at=now()
			WHERE twitch_id=$1 AND provider=$2 AND provider_emote_id=$3 AND valid_to IS NULL`, in.TwitchID, change.Provider, change.ProviderEmoteID, in.EffectiveAt, snapshotID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			UPDATE channel_emote_membership_periods
			SET alias=$4, last_seen_by_us=$5, updated_at=now()
			WHERE twitch_id=$1 AND provider=$2 AND provider_emote_id=$3 AND valid_to IS NULL`, in.TwitchID, change.Provider, change.ProviderEmoteID, change.ToAlias, in.EffectiveAt)
		if err != nil {
			return err
		}
	}
	for _, item := range append(diff.Added, diff.Readded...) {
		eventKind := "add"
		for _, readded := range diff.Readded {
			if snapshotIdentityKey(readded) == snapshotIdentityKey(item) {
				eventKind = "readd"
				break
			}
		}
		localID, _ := localEmoteIDTx(ctx, tx, item.Provider, item.ProviderEmoteID)
		_, err := tx.Exec(ctx, `
			INSERT INTO channel_emote_membership_periods (twitch_id, login, provider, provider_emote_id, local_emote_id, provider_set_id, alias, valid_from, first_seen_by_us, last_seen_by_us, opened_snapshot_id, event_kind)
			VALUES ($1,$2,$3,$4,$5::uuid,$6,$7,$8,$8,$8,$9::uuid,$10)
			ON CONFLICT DO NOTHING`, in.TwitchID, in.Login, item.Provider, item.ProviderEmoteID, nullableString(localID), item.ProviderSetID, item.Alias, in.EffectiveAt, snapshotID, eventKind)
		if err != nil {
			return err
		}
	}
	for _, item := range append(append(diff.Added, diff.Readded...), aliasChangeItems(diff.AliasChanges, append(diff.Added, diff.Readded...))...) {
		_, err := tx.Exec(ctx, `
			INSERT INTO emote_alias_history (twitch_id, login, provider, provider_emote_id, alias, valid_from, first_seen_by_us, last_seen_by_us, opened_snapshot_id)
			VALUES ($1,$2,$3,$4,$5,$6,$6,$6,$7::uuid)
			ON CONFLICT DO NOTHING`, in.TwitchID, in.Login, item.Provider, item.ProviderEmoteID, item.Alias, in.EffectiveAt, snapshotID)
		if err != nil {
			return err
		}
	}
	if len(diff.Added) == 0 && len(diff.Readded) == 0 && len(diff.Removed) == 0 && len(diff.AliasChanges) == 0 {
		for _, item := range NormalizeEmoteSnapshotItems(in.Provider, in.Items) {
			localID, _ := localEmoteIDTx(ctx, tx, item.Provider, item.ProviderEmoteID)
			_, err := tx.Exec(ctx, `
				INSERT INTO channel_emote_membership_periods (twitch_id, login, provider, provider_emote_id, local_emote_id, provider_set_id, alias, valid_from, first_seen_by_us, last_seen_by_us, opened_snapshot_id, event_kind)
				VALUES ($1,$2,$3,$4,$5::uuid,$6,$7,$8,$8,$8,$9::uuid,'bootstrap')
				ON CONFLICT DO NOTHING`, in.TwitchID, in.Login, item.Provider, item.ProviderEmoteID, nullableString(localID), item.ProviderSetID, item.Alias, in.EffectiveAt, snapshotID)
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `
				INSERT INTO emote_alias_history (twitch_id, login, provider, provider_emote_id, alias, valid_from, first_seen_by_us, last_seen_by_us, opened_snapshot_id)
				VALUES ($1,$2,$3,$4,$5,$6,$6,$6,$7::uuid)
				ON CONFLICT DO NOTHING`, in.TwitchID, in.Login, item.Provider, item.ProviderEmoteID, item.Alias, in.EffectiveAt, snapshotID)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func updateEmoteProviderSnapshotStateTx(ctx context.Context, tx pgx.Tx, twitchID, provider, state string, count int, hash string, snapshotID *string, rateLimitedUntil, nextAfter *time.Time) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO channel_emote_providers (twitch_id, provider, state, count, updated_at, last_snapshot_id, last_snapshot_at, last_snapshot_hash, snapshot_state, rate_limited_until, next_snapshot_after)
		VALUES ($1,$2,$3::varchar,$4,now(),$5::uuid,now(),$6,$3::text,$7::timestamptz,$8::timestamptz)
		ON CONFLICT (twitch_id, provider) DO UPDATE SET
			state=EXCLUDED.state,
			count=EXCLUDED.count,
			updated_at=now(),
			last_snapshot_id=COALESCE(EXCLUDED.last_snapshot_id, channel_emote_providers.last_snapshot_id),
			last_snapshot_at=now(),
			last_snapshot_hash=EXCLUDED.last_snapshot_hash,
			snapshot_state=EXCLUDED.snapshot_state,
			snapshot_error=NULL,
			rate_limited_until=EXCLUDED.rate_limited_until,
			next_snapshot_after=EXCLUDED.next_snapshot_after`, twitchID, provider, state, count, snapshotID, hash, rateLimitedUntil, nextAfter)
	return err
}

func resolveEmoteUsageIdentityTx(ctx context.Context, tx pgx.Tx, twitchID string, parsed ParsedEmoteRollupKey, at time.Time) (EmoteIdentityResolution, string, error) {
	if parsed.Provider != "" && parsed.ID != "" {
		localID, _ := localEmoteIDTx(ctx, tx, parsed.Provider, parsed.ID)
		return ResolveEmoteIdentityAt(parsed, nil), localID, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT provider, provider_emote_id, alias
		FROM emote_alias_history
		WHERE twitch_id=$1 AND lower(alias)=lower($2) AND valid_from <= $3 AND (valid_to IS NULL OR valid_to > $3)
		  AND ($4 = '' OR provider = $4)`, twitchID, parsed.Name, at, parsed.Provider)
	if err != nil {
		return EmoteIdentityResolution{}, "", err
	}
	defer rows.Close()
	candidates := []EmoteIdentityCandidate{}
	for rows.Next() {
		var candidate EmoteIdentityCandidate
		if err := rows.Scan(&candidate.Provider, &candidate.ProviderEmoteID, &candidate.Name); err != nil {
			return EmoteIdentityResolution{}, "", err
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return EmoteIdentityResolution{}, "", err
	}
	resolution := ResolveEmoteIdentityAt(parsed, candidates)
	localID := ""
	if resolution.Provider != "" && resolution.ProviderEmoteID != "" {
		localID, _ = localEmoteIDTx(ctx, tx, resolution.Provider, resolution.ProviderEmoteID)
	}
	return resolution, localID, nil
}

func upsertEmoteUsageMinuteTx(ctx context.Context, tx pgx.Tx, streamID string, minute time.Time, twitchID, login, sourceKey string, count int, resolution EmoteIdentityResolution, localID string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO emote_usage_minute_rollups (stream_id, minute_ts, twitch_id, login, provider, provider_emote_id, emote_name, local_emote_id, use_count, identity_resolution, confidence, source_key, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8::uuid,$9,$10,$11,$12,now())
		ON CONFLICT (stream_id, minute_ts, source_key) DO UPDATE SET
			twitch_id=EXCLUDED.twitch_id,
			login=EXCLUDED.login,
			provider=EXCLUDED.provider,
			provider_emote_id=EXCLUDED.provider_emote_id,
			emote_name=EXCLUDED.emote_name,
			local_emote_id=EXCLUDED.local_emote_id,
			use_count=EXCLUDED.use_count,
			identity_resolution=EXCLUDED.identity_resolution,
			confidence=EXCLUDED.confidence,
			updated_at=now()`, streamID, minute, twitchID, login, resolution.Provider, resolution.ProviderEmoteID, resolution.Name, nullableString(localID), count, resolution.Resolution, resolution.Confidence, sourceKey)
	return err
}

func refreshEmoteUsageStreamRollupsTx(ctx context.Context, tx pgx.Tx, streamID string) error {
	_, err := tx.Exec(ctx, `DELETE FROM emote_usage_stream_rollups WHERE stream_id=$1`, streamID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO emote_usage_stream_rollups (stream_id, twitch_id, login, provider, provider_emote_id, emote_name, local_emote_id, use_count, minutes_seen, first_minute_ts, last_minute_ts, identity_resolution, confidence, updated_at)
		SELECT stream_id, twitch_id, login, provider, provider_emote_id, emote_name, (array_agg(local_emote_id) FILTER (WHERE local_emote_id IS NOT NULL))[1], SUM(use_count)::bigint, COUNT(*)::int, MIN(minute_ts), MAX(minute_ts), identity_resolution, AVG(confidence)::numeric(5,4), now()
		FROM emote_usage_minute_rollups
		WHERE stream_id=$1
		GROUP BY stream_id, twitch_id, login, provider, provider_emote_id, emote_name, identity_resolution`, streamID)
	return err
}

func localEmoteIDTx(ctx context.Context, tx pgx.Tx, provider, providerEmoteID string) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `SELECT id::text FROM emotes WHERE provider=$1 AND provider_emote_id=$2 LIMIT 1`, normalizeProvider(provider), strings.TrimSpace(providerEmoteID)).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return id, err
}

func (s *Store) portalTopChannelEmotes(ctx context.Context, login string, since time.Time, _ time.Duration) ([]PortalChannelEmote, portalEmoteTotals, error) {
	rows, err := s.db.Query(ctx, `
		WITH grouped AS (
			SELECT provider, provider_emote_id, emote_name, identity_resolution,
			       SUM(use_count)::bigint AS use_count,
			       COUNT(DISTINCT stream_id || ':' || minute_ts)::int AS minutes_seen,
			       AVG(confidence)::float8 AS confidence,
			       MIN(minute_ts) AS first_seen,
			       MAX(minute_ts) AS last_seen
			FROM emote_usage_minute_rollups
			WHERE login=$1 AND minute_ts >= $2
			GROUP BY provider, provider_emote_id, emote_name, identity_resolution
		), totals AS (
			SELECT COALESCE(SUM(use_count),0)::bigint AS total_uses,
			       COUNT(*)::int AS unique_emotes,
			       COALESCE(SUM(use_count) FILTER (WHERE provider='seventv'),0)::bigint AS seventv_uses
			FROM grouped
		)
		SELECT g.provider, g.provider_emote_id, g.emote_name, g.identity_resolution, g.use_count, g.minutes_seen, g.confidence, g.first_seen, g.last_seen,
		       t.total_uses, t.unique_emotes, t.seventv_uses
		FROM grouped g CROSS JOIN totals t
		ORDER BY g.use_count DESC, g.confidence DESC, g.provider, g.provider_emote_id, g.emote_name
		LIMIT 25`, login, since)
	if err != nil {
		return nil, portalEmoteTotals{}, err
	}
	defer rows.Close()
	out := []PortalChannelEmote{}
	totals := portalEmoteTotals{}
	for rows.Next() {
		var emote PortalChannelEmote
		var firstSeen, lastSeen time.Time
		var seventvUses int64
		if err := rows.Scan(&emote.Provider, &emote.ProviderEmoteID, &emote.Name, &emote.IdentityResolution, &emote.UseCount, &emote.MinutesSeen, &emote.Confidence, &firstSeen, &lastSeen, &totals.totalUses, &totals.uniqueEmotes, &seventvUses); err != nil {
			return nil, totals, err
		}
		emote.Confidence = round2(emote.Confidence * 100)
		if totals.totalUses > 0 {
			emote.SharePct = round2(float64(emote.UseCount) / float64(totals.totalUses) * 100)
			totals.sevenTVSharePct = float64(seventvUses) / float64(totals.totalUses) * 100
		}
		emote.FirstSeenAt = &firstSeen
		emote.LastSeenAt = &lastSeen
		out = append(out, emote)
	}
	return out, totals, rows.Err()
}

func (s *Store) portalEmoteHistory(ctx context.Context, login string, since time.Time) ([]PortalEmoteHistory, error) {
	rows, err := s.db.Query(ctx, `
		SELECT date_trunc('day', minute_ts)::date AS day,
		       COALESCE(SUM(use_count),0)::bigint AS use_count,
		       COUNT(DISTINCT provider || ':' || provider_emote_id || ':' || emote_name)::int AS unique_emotes,
		       COALESCE(SUM(use_count) FILTER (WHERE provider='seventv'),0)::bigint AS seventv_uses
		FROM emote_usage_minute_rollups
		WHERE login=$1 AND minute_ts >= $2
		GROUP BY day
		ORDER BY day`, login, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PortalEmoteHistory{}
	for rows.Next() {
		var item PortalEmoteHistory
		if err := rows.Scan(&item.Day, &item.UseCount, &item.UniqueEmotes, &item.SevenTVUseCount); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) portalEmoteMoments(ctx context.Context, login string, since time.Time) ([]PortalEmoteMoment, error) {
	rows, err := s.db.Query(ctx, `
		WITH minute_totals AS (
			SELECT u.stream_id, u.minute_ts, SUM(u.use_count)::int AS uses
			FROM emote_usage_minute_rollups u
			WHERE u.login=$1 AND u.minute_ts >= $2 AND u.identity_resolution IN ('provider_id','alias_fallback')
			GROUP BY u.stream_id, u.minute_ts
		), top_emote AS (
			SELECT DISTINCT ON (u.stream_id, u.minute_ts)
			       u.stream_id, u.minute_ts, u.provider, u.provider_emote_id, u.emote_name, u.use_count
			FROM emote_usage_minute_rollups u
			WHERE u.login=$1 AND u.minute_ts >= $2
			ORDER BY u.stream_id, u.minute_ts, u.use_count DESC, u.provider, u.provider_emote_id
		)
		SELECT m.stream_id, s.started_at, GREATEST(0, EXTRACT(EPOCH FROM (m.minute_ts - s.started_at))::int), m.uses, t.emote_name, t.provider, t.provider_emote_id
		FROM minute_totals m
		JOIN analytics_streams s ON s.stream_id=m.stream_id
		LEFT JOIN top_emote t ON t.stream_id=m.stream_id AND t.minute_ts=m.minute_ts
		ORDER BY m.uses DESC, m.minute_ts DESC
		LIMIT 10`, login, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PortalEmoteMoment{}
	for rows.Next() {
		var item PortalEmoteMoment
		if err := rows.Scan(&item.StreamID, &item.StartedAt, &item.OffsetSeconds, &item.UseCount, &item.TopEmoteName, &item.Provider, &item.ProviderEmoteID); err != nil {
			return nil, err
		}
		item.Href = fmt.Sprintf("/analytics/%s/s/%s?t=%d#emotes", login, item.StreamID, item.OffsetSeconds)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) portalEmoteFreshness(ctx context.Context, login string, since time.Time, latestUsage *time.Time, now time.Time) PortalEmoteFreshness {
	out := PortalEmoteFreshness{LatestUsageAt: latestUsage, ProviderState: "unknown"}
	if latestUsage != nil {
		out.UsageStalenessSec = int64(now.Sub(*latestUsage).Seconds())
	}
	var snapshotAt *time.Time
	var state, errText string
	err := s.db.QueryRow(ctx, `
		SELECT p.last_snapshot_at, COALESCE(p.snapshot_state,''), COALESCE(p.snapshot_error,'')
		FROM channel_emote_providers p
		JOIN channels c ON c.twitch_id=p.twitch_id
		WHERE c.login=$1
		ORDER BY p.last_snapshot_at DESC NULLS LAST, p.updated_at DESC
		LIMIT 1`, login).Scan(&snapshotAt, &state, &errText)
	if err == nil {
		out.LatestSnapshotAt = snapshotAt
		out.ProviderState = firstNonEmptyEmoteValue(state, "unknown")
		out.ProviderError = errText
		if snapshotAt != nil {
			out.ProviderStalenessSec = int64(now.Sub(*snapshotAt).Seconds())
		}
	}
	if latestUsage == nil && out.LatestSnapshotAt == nil && since.Before(now) {
		out.ProviderState = firstNonEmptyEmoteValue(out.ProviderState, "unknown")
	}
	return out
}

func aliasChangeItems(changes []EmoteAliasChange, added []EmoteSnapshotItem) []EmoteSnapshotItem {
	addedByID := snapshotIdentityMap(added)
	out := []EmoteSnapshotItem{}
	for _, change := range changes {
		key := normalizeProvider(change.Provider) + ":" + strings.TrimSpace(change.ProviderEmoteID)
		if _, ok := addedByID[key]; ok {
			continue
		}
		out = append(out, EmoteSnapshotItem{Provider: normalizeProvider(change.Provider), ProviderEmoteID: change.ProviderEmoteID, Alias: change.ToAlias})
	}
	return out
}

func jsonUnmarshalEmoteCounts(raw []byte, out *map[string]int) error {
	if out == nil {
		return nil
	}
	var values map[string]int
	if err := json.Unmarshal(raw, &values); err != nil {
		return err
	}
	*out = values
	return nil
}

func nullableString(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func nullableInt(value int) any {
	if value == 0 {
		return nil
	}
	return value
}

func formatEmoteRange(d time.Duration) string {
	hours := int(d.Hours())
	if hours > 0 && hours%24 == 0 {
		return fmt.Sprintf("%dd", hours/24)
	}
	return d.String()
}

func parsePortalEmoteRange(raw string) time.Duration {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return 30 * 24 * time.Hour
	}
	if strings.HasSuffix(raw, "d") {
		var days int
		if _, err := fmt.Sscanf(raw, "%dd", &days); err == nil && days > 0 && days <= 365 {
			return time.Duration(days) * 24 * time.Hour
		}
	}
	if d, err := time.ParseDuration(raw); err == nil && d > 0 && d <= 365*24*time.Hour {
		return d
	}
	return 30 * 24 * time.Hour
}

func sortPortalEmotes(items []PortalChannelEmote) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].UseCount != items[j].UseCount {
			return items[i].UseCount > items[j].UseCount
		}
		if items[i].Confidence != items[j].Confidence {
			return items[i].Confidence > items[j].Confidence
		}
		return items[i].Provider+items[i].ProviderEmoteID < items[j].Provider+items[j].ProviderEmoteID
	})
}
