package analytics

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	Top500ChannelSourceOperatorSeed = "operator_seed"
	Top500ChannelSourceConfigured   = "configured"

	Top500SnapshotSourceHelixStreams = "helix_streams"
	Top500SnapshotSourceHelixUsers   = "helix_users"
	Top500SnapshotSourceCache        = "cache"
	Top500SnapshotSourceMetadata     = "top500_metadata"
	Top500SnapshotSourceConfigured   = "configured"

	Top500CoverageSourceMetadata  = "top500_metadata"
	Top500CoverageSourceTier0     = "tier0"
	Top500CoverageSourceCollector = "collector"
	Top500CoverageSourceCache     = "cache"
	Top500CoverageSourceHelix     = "helix"
)

type Top500Channel struct {
	ChannelID      string
	Login          string
	DisplayName    string
	Rank           int
	Source         string
	SourceVersion  string
	SeededBy       string
	EffectiveAt    time.Time
	SourceMetadata map[string]any
	Enabled        bool
	LastSeenAt     *time.Time
	LastSampledAt  *time.Time
	LastLiveAt     *time.Time
}

type Top500LiveSnapshot struct {
	ChannelID    string
	Login        string
	StreamID     *string
	IsLive       bool
	Title        string
	CategoryID   string
	CategoryName string
	StartedAt    *time.Time
	ViewerCount  *int
	Language     string
	Tags         []string
	SampleTickAt time.Time
	SampledAt    time.Time
	Source       string
	FailureCode  string
}

type Top500Current struct {
	ChannelID      string
	Login          string
	DisplayName    string
	Rank           int
	CoverageSource string
	IsLive         bool
	StreamID       *string
	Title          string
	CategoryID     string
	CategoryName   string
	StartedAt      *time.Time
	ViewerCount    *int
	Language       string
	Tags           []string
	SampledAt      time.Time
	StaleAfter     time.Time
	LastSuccessAt  *time.Time
	LastErrorCode  string
	UpdatedAt      time.Time
}

func (c Top500Current) FreshnessSeconds(now time.Time) *int {
	if c.SampledAt.IsZero() {
		return nil
	}
	seconds := int(now.Sub(c.SampledAt).Seconds())
	if seconds < 0 {
		seconds = 0
	}
	return &seconds
}

func (s *Store) UpsertTop500Channel(ctx context.Context, entry Top500Channel) error {
	if err := validateTop500Channel(entry); err != nil {
		return err
	}
	metadata, err := marshalTop500Object(entry.SourceMetadata)
	if err != nil {
		return err
	}
	effectiveAt := entry.EffectiveAt
	if effectiveAt.IsZero() {
		effectiveAt = time.Now().UTC()
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO top500_channels (
			channel_id, login, display_name, rank, source, source_version, seeded_by,
			effective_at, source_metadata, enabled, last_seen_at, last_sampled_at, last_live_at
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11,$12,$13)
		ON CONFLICT (channel_id) DO UPDATE SET
			login=EXCLUDED.login,
			display_name=EXCLUDED.display_name,
			rank=EXCLUDED.rank,
			source=EXCLUDED.source,
			source_version=EXCLUDED.source_version,
			seeded_by=EXCLUDED.seeded_by,
			effective_at=EXCLUDED.effective_at,
			source_metadata=EXCLUDED.source_metadata,
			enabled=EXCLUDED.enabled,
			last_seen_at=EXCLUDED.last_seen_at,
			last_sampled_at=EXCLUDED.last_sampled_at,
			last_live_at=EXCLUDED.last_live_at,
			updated_at=now()`,
		strings.TrimSpace(entry.ChannelID), normalizeLogin(entry.Login), strings.TrimSpace(entry.DisplayName), entry.Rank,
		entry.Source, strings.TrimSpace(entry.SourceVersion), strings.TrimSpace(entry.SeededBy), effectiveAt,
		string(metadata), entry.Enabled, entry.LastSeenAt, entry.LastSampledAt, entry.LastLiveAt)
	return err
}

func (s *Store) UpsertTop500Channels(ctx context.Context, entries []Top500Channel) error {
	if len(entries) == 0 {
		return nil
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	store := &Store{db: s.db}
	for _, entry := range entries {
		if err := store.upsertTop500ChannelTx(ctx, tx, entry); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) upsertTop500ChannelTx(ctx context.Context, tx pgx.Tx, entry Top500Channel) error {
	if err := validateTop500Channel(entry); err != nil {
		return err
	}
	metadata, err := marshalTop500Object(entry.SourceMetadata)
	if err != nil {
		return err
	}
	effectiveAt := entry.EffectiveAt
	if effectiveAt.IsZero() {
		effectiveAt = time.Now().UTC()
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO top500_channels (
			channel_id, login, display_name, rank, source, source_version, seeded_by,
			effective_at, source_metadata, enabled, last_seen_at, last_sampled_at, last_live_at
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11,$12,$13)
		ON CONFLICT (channel_id) DO UPDATE SET
			login=EXCLUDED.login,
			display_name=EXCLUDED.display_name,
			rank=EXCLUDED.rank,
			source=EXCLUDED.source,
			source_version=EXCLUDED.source_version,
			seeded_by=EXCLUDED.seeded_by,
			effective_at=EXCLUDED.effective_at,
			source_metadata=EXCLUDED.source_metadata,
			enabled=EXCLUDED.enabled,
			last_seen_at=EXCLUDED.last_seen_at,
			last_sampled_at=EXCLUDED.last_sampled_at,
			last_live_at=EXCLUDED.last_live_at,
			updated_at=now()`,
		strings.TrimSpace(entry.ChannelID), normalizeLogin(entry.Login), strings.TrimSpace(entry.DisplayName), entry.Rank,
		entry.Source, strings.TrimSpace(entry.SourceVersion), strings.TrimSpace(entry.SeededBy), effectiveAt,
		string(metadata), entry.Enabled, entry.LastSeenAt, entry.LastSampledAt, entry.LastLiveAt)
	return err
}

func (s *Store) UpsertTop500LiveSnapshot(ctx context.Context, snapshot Top500LiveSnapshot) error {
	if err := validateTop500LiveSnapshot(snapshot); err != nil {
		return err
	}
	tags, err := marshalTop500Tags(snapshot.Tags)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO top500_live_snapshots (
			channel_id, login, stream_id, is_live, title, category_id, category_name,
			started_at, viewer_count, language, tags, sample_tick_at, sampled_at, source, failure_code
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb,$12,$13,$14,$15)
		ON CONFLICT (channel_id, sample_tick_at) DO UPDATE SET
			login=EXCLUDED.login,
			stream_id=EXCLUDED.stream_id,
			is_live=EXCLUDED.is_live,
			title=EXCLUDED.title,
			category_id=EXCLUDED.category_id,
			category_name=EXCLUDED.category_name,
			started_at=EXCLUDED.started_at,
			viewer_count=EXCLUDED.viewer_count,
			language=EXCLUDED.language,
			tags=EXCLUDED.tags,
			sampled_at=EXCLUDED.sampled_at,
			source=EXCLUDED.source,
			failure_code=EXCLUDED.failure_code,
			ingested_at=now()`,
		strings.TrimSpace(snapshot.ChannelID), normalizeLogin(snapshot.Login), snapshot.StreamID, snapshot.IsLive,
		strings.TrimSpace(snapshot.Title), strings.TrimSpace(snapshot.CategoryID), strings.TrimSpace(snapshot.CategoryName),
		snapshot.StartedAt, snapshot.ViewerCount, strings.TrimSpace(snapshot.Language), string(tags),
		snapshot.SampleTickAt, snapshot.SampledAt, snapshot.Source, strings.TrimSpace(snapshot.FailureCode))
	return err
}

func (s *Store) UpsertTop500LiveSnapshots(ctx context.Context, snapshots []Top500LiveSnapshot) error {
	if len(snapshots) == 0 {
		return nil
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, snapshot := range snapshots {
		if err := upsertTop500LiveSnapshotTx(ctx, tx, snapshot); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func upsertTop500LiveSnapshotTx(ctx context.Context, tx pgx.Tx, snapshot Top500LiveSnapshot) error {
	if err := validateTop500LiveSnapshot(snapshot); err != nil {
		return err
	}
	tags, err := marshalTop500Tags(snapshot.Tags)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO top500_live_snapshots (
			channel_id, login, stream_id, is_live, title, category_id, category_name,
			started_at, viewer_count, language, tags, sample_tick_at, sampled_at, source, failure_code
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb,$12,$13,$14,$15)
		ON CONFLICT (channel_id, sample_tick_at) DO UPDATE SET
			login=EXCLUDED.login,
			stream_id=EXCLUDED.stream_id,
			is_live=EXCLUDED.is_live,
			title=EXCLUDED.title,
			category_id=EXCLUDED.category_id,
			category_name=EXCLUDED.category_name,
			started_at=EXCLUDED.started_at,
			viewer_count=EXCLUDED.viewer_count,
			language=EXCLUDED.language,
			tags=EXCLUDED.tags,
			sampled_at=EXCLUDED.sampled_at,
			source=EXCLUDED.source,
			failure_code=EXCLUDED.failure_code,
			ingested_at=now()`,
		strings.TrimSpace(snapshot.ChannelID), normalizeLogin(snapshot.Login), snapshot.StreamID, snapshot.IsLive,
		strings.TrimSpace(snapshot.Title), strings.TrimSpace(snapshot.CategoryID), strings.TrimSpace(snapshot.CategoryName),
		snapshot.StartedAt, snapshot.ViewerCount, strings.TrimSpace(snapshot.Language), string(tags),
		snapshot.SampleTickAt, snapshot.SampledAt, snapshot.Source, strings.TrimSpace(snapshot.FailureCode))
	return err
}

func (s *Store) UpsertTop500Current(ctx context.Context, current Top500Current) error {
	if err := validateTop500Current(current); err != nil {
		return err
	}
	tags, err := marshalTop500Tags(current.Tags)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO top500_current (
			channel_id, login, display_name, rank, coverage_source, is_live, stream_id,
			title, category_id, category_name, started_at, viewer_count, language, tags,
			sampled_at, stale_after, last_success_at, last_error_code
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14::jsonb,$15,$16,$17,$18)
		ON CONFLICT (channel_id) DO UPDATE SET
			login=EXCLUDED.login,
			display_name=EXCLUDED.display_name,
			rank=EXCLUDED.rank,
			coverage_source=EXCLUDED.coverage_source,
			is_live=EXCLUDED.is_live,
			stream_id=EXCLUDED.stream_id,
			title=EXCLUDED.title,
			category_id=EXCLUDED.category_id,
			category_name=EXCLUDED.category_name,
			started_at=EXCLUDED.started_at,
			viewer_count=EXCLUDED.viewer_count,
			language=EXCLUDED.language,
			tags=EXCLUDED.tags,
			sampled_at=EXCLUDED.sampled_at,
			stale_after=EXCLUDED.stale_after,
			last_success_at=EXCLUDED.last_success_at,
			last_error_code=EXCLUDED.last_error_code,
			updated_at=now()`,
		strings.TrimSpace(current.ChannelID), normalizeLogin(current.Login), strings.TrimSpace(current.DisplayName), current.Rank,
		current.CoverageSource, current.IsLive, current.StreamID, strings.TrimSpace(current.Title),
		strings.TrimSpace(current.CategoryID), strings.TrimSpace(current.CategoryName), current.StartedAt,
		current.ViewerCount, strings.TrimSpace(current.Language), string(tags), current.SampledAt,
		current.StaleAfter, current.LastSuccessAt, strings.TrimSpace(current.LastErrorCode))
	return err
}

func (s *Store) GetTop500CurrentByLogin(ctx context.Context, login string) (*Top500Current, error) {
	return scanTop500Current(s.db.QueryRow(ctx, top500CurrentSelectSQL+` WHERE login=$1`, normalizeLogin(login)))
}

func (s *Store) GetTop500CurrentByChannelID(ctx context.Context, channelID string) (*Top500Current, error) {
	return scanTop500Current(s.db.QueryRow(ctx, top500CurrentSelectSQL+` WHERE channel_id=$1`, strings.TrimSpace(channelID)))
}

const top500CurrentSelectSQL = `
	SELECT channel_id, login, display_name, rank, coverage_source, is_live, stream_id,
		title, category_id, category_name, started_at, viewer_count, language, tags,
		sampled_at, stale_after, last_success_at, last_error_code, updated_at
	FROM top500_current`

func scanTop500Current(row pgx.Row) (*Top500Current, error) {
	var current Top500Current
	var streamID sql.NullString
	var startedAt sql.NullTime
	var viewerCount sql.NullInt64
	var lastSuccessAt sql.NullTime
	var tagsJSON []byte
	err := row.Scan(
		&current.ChannelID, &current.Login, &current.DisplayName, &current.Rank, &current.CoverageSource,
		&current.IsLive, &streamID, &current.Title, &current.CategoryID, &current.CategoryName,
		&startedAt, &viewerCount, &current.Language, &tagsJSON, &current.SampledAt, &current.StaleAfter,
		&lastSuccessAt, &current.LastErrorCode, &current.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if streamID.Valid {
		current.StreamID = &streamID.String
	}
	if startedAt.Valid {
		current.StartedAt = &startedAt.Time
	}
	if viewerCount.Valid {
		value := int(viewerCount.Int64)
		current.ViewerCount = &value
	}
	if lastSuccessAt.Valid {
		current.LastSuccessAt = &lastSuccessAt.Time
	}
	if len(tagsJSON) > 0 {
		if err := json.Unmarshal(tagsJSON, &current.Tags); err != nil {
			return nil, err
		}
	}
	return &current, nil
}

func validateTop500Channel(entry Top500Channel) error {
	if strings.TrimSpace(entry.ChannelID) == "" {
		return errors.New("top500 channel_id required")
	}
	if normalizeLogin(entry.Login) == "" {
		return errors.New("top500 login required")
	}
	if entry.Rank <= 0 {
		return errors.New("top500 rank must be positive")
	}
	if !allowedTop500ChannelSource(entry.Source) {
		return fmt.Errorf("unsupported top500 channel source %q", entry.Source)
	}
	return nil
}

func validateTop500LiveSnapshot(snapshot Top500LiveSnapshot) error {
	if strings.TrimSpace(snapshot.ChannelID) == "" {
		return errors.New("top500 snapshot channel_id required")
	}
	if normalizeLogin(snapshot.Login) == "" {
		return errors.New("top500 snapshot login required")
	}
	if snapshot.SampleTickAt.IsZero() {
		return errors.New("top500 snapshot sample_tick_at required")
	}
	if snapshot.SampledAt.IsZero() {
		return errors.New("top500 snapshot sampled_at required")
	}
	if !allowedTop500SnapshotSource(snapshot.Source) {
		return fmt.Errorf("unsupported top500 snapshot source %q", snapshot.Source)
	}
	return nil
}

func validateTop500Current(current Top500Current) error {
	if strings.TrimSpace(current.ChannelID) == "" {
		return errors.New("top500 current channel_id required")
	}
	if normalizeLogin(current.Login) == "" {
		return errors.New("top500 current login required")
	}
	if current.Rank <= 0 {
		return errors.New("top500 current rank must be positive")
	}
	if current.SampledAt.IsZero() {
		return errors.New("top500 current sampled_at required")
	}
	if current.StaleAfter.IsZero() {
		return errors.New("top500 current stale_after required")
	}
	if !allowedTop500CoverageSource(current.CoverageSource) {
		return fmt.Errorf("unsupported top500 coverage source %q", current.CoverageSource)
	}
	return nil
}

func allowedTop500ChannelSource(source string) bool {
	switch source {
	case Top500ChannelSourceOperatorSeed, Top500ChannelSourceConfigured:
		return true
	default:
		return false
	}
}

func allowedTop500SnapshotSource(source string) bool {
	switch source {
	case Top500SnapshotSourceHelixStreams, Top500SnapshotSourceHelixUsers, Top500SnapshotSourceCache,
		Top500SnapshotSourceMetadata, Top500SnapshotSourceConfigured:
		return true
	default:
		return false
	}
}

func allowedTop500CoverageSource(source string) bool {
	switch source {
	case Top500CoverageSourceMetadata, Top500CoverageSourceTier0, Top500CoverageSourceCollector,
		Top500CoverageSourceCache, Top500CoverageSourceHelix:
		return true
	default:
		return false
	}
}

func marshalTop500Object(value map[string]any) ([]byte, error) {
	if value == nil {
		return []byte(`{}`), nil
	}
	return json.Marshal(value)
}

func marshalTop500Tags(tags []string) ([]byte, error) {
	if tags == nil {
		return []byte(`[]`), nil
	}
	return json.Marshal(tags)
}
