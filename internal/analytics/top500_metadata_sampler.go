package analytics

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"streamclone/internal/metrics"
)

const (
	DefaultTop500MetadataTopN            = 100
	MaxTop500MetadataTopN                = 1000
	DefaultTop500MetadataBatchSize       = 100
	MaxTop500MetadataBatchSize           = 100
	DefaultTop500MetadataLiveInterval    = 60 * time.Second
	DefaultTop500MetadataOfflineInterval = 10 * time.Minute
	DefaultTop500MetadataStaleAfter      = 15 * time.Minute

	Top500SamplerClassHelixRateLimited    = "helix_rate_limited"
	Top500SamplerClassHelixAuthMissing    = "helix_auth_missing"
	Top500SamplerClassHelixTransientError = "helix_transient_error"
	Top500SamplerClassHelixNotFound       = "helix_not_found"
	Top500SamplerClassProviderUnavailable = "provider_unavailable"
	Top500SamplerClassSamplerDisabled     = "sampler_disabled"
	Top500SamplerClassWriteDisabled       = "write_disabled"
	Top500SamplerClassDryRun              = "dry_run"
	Top500SamplerClassLockUnavailable     = "lock_unavailable"

	top500MetadataSamplerAdvisoryLockKey int64 = 500100500
)

var (
	ErrTop500ProviderRateLimited   = errors.New("top500 metadata provider rate limited")
	ErrTop500ProviderAuthMissing   = errors.New("top500 metadata provider auth missing")
	ErrTop500ProviderTransient     = errors.New("top500 metadata provider transient error")
	ErrTop500ProviderNotFound      = errors.New("top500 metadata provider not found")
	ErrTop500ProviderUnavailable   = errors.New("top500 metadata provider unavailable")
	ErrTop500MetadataStoreRequired = errors.New("top500 metadata store required")
)

type Top500MetadataProvider interface {
	FetchStreams(ctx context.Context, channels []Top500Channel) ([]Top500StreamMetadata, error)
	FetchUsers(ctx context.Context, channels []Top500Channel) ([]Top500UserMetadata, error)
}

type Top500MetadataStore interface {
	ListEnabledTop500Channels(ctx context.Context, limit int) ([]Top500Channel, error)
	GetTop500CurrentByChannelID(ctx context.Context, channelID string) (*Top500Current, error)
	WriteTop500MetadataSamples(ctx context.Context, samples []Top500MetadataSample) error
}

type Top500MetadataSamplerLocker interface {
	TryTop500MetadataSamplerLock(ctx context.Context) (Top500MetadataSamplerLock, bool, error)
}

type Top500MetadataSamplerLock interface {
	Release(ctx context.Context) error
}

type Top500SamplerConfig struct {
	Enabled         bool
	DryRun          bool
	WriteEnabled    bool
	TopN            int
	BatchSize       int
	LiveInterval    time.Duration
	OfflineInterval time.Duration
}

type Top500StreamMetadata struct {
	ChannelID    string
	Login        string
	StreamID     string
	Title        string
	CategoryID   string
	CategoryName string
	StartedAt    *time.Time
	ViewerCount  *int
	Language     string
	Tags         []string
	SampledAt    time.Time
}

type Top500UserMetadata struct {
	ChannelID   string
	Login       string
	DisplayName string
}

type Top500MetadataSample struct {
	Channel  Top500Channel
	Snapshot Top500LiveSnapshot
	Current  Top500Current
}

type Top500SamplerTickResult struct {
	Classifications []string
	Planned         []Top500Channel
	SkippedNotDue   []Top500Channel
	StreamsFetched  int
	UsersFetched    int
	WritesAttempted int
	LockAcquired    bool
	LockReleased    bool
}

type Top500MetadataSampler struct {
	cfg        Top500SamplerConfig
	store      Top500MetadataStore
	provider   Top500MetadataProvider
	locker     Top500MetadataSamplerLocker
	planCursor int
}

func NewTop500MetadataSampler(cfg Top500SamplerConfig, store Top500MetadataStore, provider Top500MetadataProvider, locker Top500MetadataSamplerLocker) *Top500MetadataSampler {
	return &Top500MetadataSampler{
		cfg:      normalizeTop500SamplerConfig(cfg),
		store:    store,
		provider: provider,
		locker:   locker,
	}
}

func normalizeTop500SamplerConfig(cfg Top500SamplerConfig) Top500SamplerConfig {
	if cfg.TopN <= 0 || cfg.TopN > MaxTop500MetadataTopN {
		cfg.TopN = DefaultTop500MetadataTopN
	}
	if cfg.BatchSize <= 0 || cfg.BatchSize > MaxTop500MetadataBatchSize {
		cfg.BatchSize = DefaultTop500MetadataBatchSize
	}
	if cfg.LiveInterval <= 0 {
		cfg.LiveInterval = DefaultTop500MetadataLiveInterval
	}
	if cfg.OfflineInterval <= 0 {
		cfg.OfflineInterval = DefaultTop500MetadataOfflineInterval
	}
	return cfg
}

func (s *Top500MetadataSampler) RunTick(ctx context.Context, now time.Time) (result Top500SamplerTickResult, err error) {
	if s == nil {
		result.addClass(Top500SamplerClassSamplerDisabled)
		recordTop500SamplerConfigMetrics(Top500SamplerConfig{})
		recordTop500SamplerRollback(Top500SamplerClassSamplerDisabled, top500SamplerMode(Top500SamplerConfig{}))
		return result, nil
	}
	cfg := normalizeTop500SamplerConfig(s.cfg)
	mode := top500SamplerMode(cfg)
	recordTop500SamplerConfigMetrics(cfg)
	resetTop500SamplerRollbackState(mode)
	if !cfg.Enabled {
		result.addClass(Top500SamplerClassSamplerDisabled)
		recordTop500SamplerRollback(Top500SamplerClassSamplerDisabled, mode)
		return result, nil
	}
	if !cfg.DryRun && cfg.WriteEnabled && s.locker == nil {
		result.addClass(Top500SamplerClassLockUnavailable)
		recordTop500SamplerLockUnavailable("missing_locker", mode)
		return result, nil
	}

	if s.locker != nil {
		lock, acquired, err := s.locker.TryTop500MetadataSamplerLock(ctx)
		if err != nil {
			result.addClass(Top500SamplerClassLockUnavailable)
			recordTop500SamplerLockUnavailable("lock_error", mode)
			return result, err
		}
		if !acquired {
			result.addClass(Top500SamplerClassLockUnavailable)
			recordTop500SamplerLockUnavailable(Top500SamplerClassLockUnavailable, mode)
			return result, nil
		}
		result.LockAcquired = true
		defer func() {
			if lock != nil {
				_ = lock.Release(ctx)
				result.LockReleased = true
			}
		}()
	}

	plan, err := s.PlanTick(ctx, now)
	if err != nil {
		return result, err
	}
	result.Planned = plan.Planned
	result.SkippedNotDue = plan.SkippedNotDue
	metrics.Top500MetadataRosterSize.Set(float64(len(result.Planned) + len(result.SkippedNotDue)))
	metrics.Top500MetadataChannelsPlannedTotal.WithLabelValues("planned", mode).Add(float64(len(result.Planned)))
	if len(result.Planned) == 0 {
		result.addWriteModeClass(cfg)
		return result, nil
	}
	if s.provider == nil {
		result.addClass(Top500SamplerClassProviderUnavailable)
		recordTop500SamplerRollback(Top500SamplerClassProviderUnavailable, mode)
		return result, nil
	}

	streams, err := s.provider.FetchStreams(ctx, result.Planned)
	if err != nil {
		classification := classifyTop500ProviderError(err)
		result.addClass(classification)
		recordTop500ProviderError("fetch_streams", classification, mode)
		return result, nil
	}
	result.StreamsFetched = len(streams)
	metrics.Top500MetadataProviderCallsTotal.WithLabelValues("fetch_streams", "success", "helix").Inc()

	users, err := s.provider.FetchUsers(ctx, result.Planned)
	if err != nil {
		classification := classifyTop500ProviderError(err)
		result.addClass(classification)
		recordTop500ProviderError("fetch_users", classification, mode)
		return result, nil
	}
	result.UsersFetched = len(users)
	metrics.Top500MetadataProviderCallsTotal.WithLabelValues("fetch_users", "success", "helix").Inc()
	metrics.Top500MetadataChannelsSampledTotal.WithLabelValues("success", mode).Add(float64(len(result.Planned)))
	if cfg.DryRun || !cfg.WriteEnabled {
		result.addWriteModeClass(cfg)
		return result, nil
	}

	samples := buildTop500MetadataSamples(result.Planned, streams, users, now)
	result.WritesAttempted = len(samples)
	writeStarted := time.Now()
	if err := s.store.WriteTop500MetadataSamples(ctx, samples); err != nil {
		recordTop500SamplerWrite("error", mode, len(samples), time.Since(writeStarted))
		metrics.Top500MetadataSnapshotWritesTotal.WithLabelValues("error", mode).Add(float64(len(samples)))
		metrics.Top500MetadataCurrentUpsertsTotal.WithLabelValues("error", mode).Add(float64(len(samples)))
		recordTop500SamplerRollback("store_error", mode)
		return result, err
	}
	recordTop500SamplerWrite("success", mode, len(samples), time.Since(writeStarted))
	metrics.Top500MetadataFreshnessSeconds.Set(top500MetadataMaxFreshnessSeconds(samples, now))
	metrics.Top500MetadataSnapshotWritesTotal.WithLabelValues("success", mode).Add(float64(len(samples)))
	metrics.Top500MetadataCurrentUpsertsTotal.WithLabelValues("success", mode).Add(float64(len(samples)))
	result.addWriteModeClass(cfg)
	return result, nil
}

func (s *Top500MetadataSampler) PlanTick(ctx context.Context, now time.Time) (Top500SamplerTickResult, error) {
	result := Top500SamplerTickResult{}
	if s == nil || !normalizeTop500SamplerConfig(s.cfg).Enabled {
		result.addClass(Top500SamplerClassSamplerDisabled)
		return result, nil
	}
	if s.store == nil {
		return result, ErrTop500MetadataStoreRequired
	}
	cfg := normalizeTop500SamplerConfig(s.cfg)
	channels, err := s.store.ListEnabledTop500Channels(ctx, cfg.TopN)
	if err != nil {
		return result, err
	}
	channels = filterTop500SamplerRoster(channels, cfg.TopN)
	if len(channels) > 1 && s.planCursor > 0 {
		offset := s.planCursor % len(channels)
		channels = append(channels[offset:], channels[:offset]...)
	}
	for _, channel := range channels {
		current, err := s.store.GetTop500CurrentByChannelID(ctx, channel.ChannelID)
		if err != nil {
			return result, err
		}
		if top500ChannelDue(channel, current, now, cfg) {
			result.Planned = append(result.Planned, channel)
			if len(result.Planned) >= cfg.BatchSize {
				break
			}
			continue
		}
		result.SkippedNotDue = append(result.SkippedNotDue, channel)
	}
	if len(channels) > 0 {
		s.planCursor = (s.planCursor + cfg.BatchSize) % len(channels)
	}
	return result, nil
}

func filterTop500SamplerRoster(channels []Top500Channel, topN int) []Top500Channel {
	if topN <= 0 || topN > MaxTop500MetadataTopN {
		topN = DefaultTop500MetadataTopN
	}
	filtered := make([]Top500Channel, 0, len(channels))
	for _, channel := range channels {
		if !channel.Enabled || !allowedTop500ChannelSource(channel.Source) || channel.Rank <= 0 {
			continue
		}
		filtered = append(filtered, channel)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].Rank == filtered[j].Rank {
			return strings.Compare(filtered[i].Login, filtered[j].Login) < 0
		}
		return filtered[i].Rank < filtered[j].Rank
	})
	if len(filtered) > topN {
		filtered = filtered[:topN]
	}
	return filtered
}

func top500ChannelDue(_ Top500Channel, current *Top500Current, now time.Time, cfg Top500SamplerConfig) bool {
	if current == nil || current.SampledAt.IsZero() {
		return true
	}
	interval := cfg.OfflineInterval
	if current.IsLive {
		interval = cfg.LiveInterval
	}
	if interval <= 0 {
		return true
	}
	return !current.SampledAt.Add(interval).After(now)
}

func classifyTop500ProviderError(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrTop500ProviderRateLimited):
		return Top500SamplerClassHelixRateLimited
	case errors.Is(err, ErrTop500ProviderAuthMissing):
		return Top500SamplerClassHelixAuthMissing
	case errors.Is(err, ErrTop500ProviderTransient):
		return Top500SamplerClassHelixTransientError
	case errors.Is(err, ErrTop500ProviderNotFound):
		return Top500SamplerClassHelixNotFound
	case errors.Is(err, ErrTop500ProviderUnavailable):
		return Top500SamplerClassProviderUnavailable
	default:
		return Top500SamplerClassProviderUnavailable
	}
}

func buildTop500MetadataSamples(channels []Top500Channel, streams []Top500StreamMetadata, users []Top500UserMetadata, sampleTickAt time.Time) []Top500MetadataSample {
	if sampleTickAt.IsZero() {
		sampleTickAt = time.Now().UTC()
	}
	streamByChannel := make(map[string]Top500StreamMetadata, len(streams))
	for _, stream := range streams {
		channelID := strings.TrimSpace(stream.ChannelID)
		if channelID == "" {
			continue
		}
		streamByChannel[channelID] = stream
	}
	userByChannel := make(map[string]Top500UserMetadata, len(users))
	for _, user := range users {
		channelID := strings.TrimSpace(user.ChannelID)
		if channelID == "" {
			continue
		}
		userByChannel[channelID] = user
	}

	samples := make([]Top500MetadataSample, 0, len(channels))
	for _, channel := range channels {
		channelID := strings.TrimSpace(channel.ChannelID)
		if channelID == "" {
			continue
		}
		user := userByChannel[channelID]
		login := firstNonEmpty(user.Login, channel.Login)
		displayName := firstNonEmpty(user.DisplayName, channel.DisplayName, login)
		sampledAt := sampleTickAt
		snapshot := Top500LiveSnapshot{
			ChannelID:    channelID,
			Login:        login,
			IsLive:       false,
			SampleTickAt: sampleTickAt,
			SampledAt:    sampledAt,
			Source:       Top500SnapshotSourceHelixUsers,
		}
		current := Top500Current{
			ChannelID:      channelID,
			Login:          login,
			DisplayName:    displayName,
			Rank:           channel.Rank,
			CoverageSource: Top500CoverageSourceMetadata,
			IsLive:         false,
			SampledAt:      sampledAt,
			StaleAfter:     sampledAt.Add(DefaultTop500MetadataStaleAfter),
			LastSuccessAt:  &sampledAt,
		}

		if stream, ok := streamByChannel[channelID]; ok {
			if !stream.SampledAt.IsZero() {
				sampledAt = stream.SampledAt
			}
			login = firstNonEmpty(stream.Login, login)
			displayName = firstNonEmpty(user.DisplayName, channel.DisplayName, login)
			streamID := strings.TrimSpace(stream.StreamID)
			snapshot = Top500LiveSnapshot{
				ChannelID:    channelID,
				Login:        login,
				IsLive:       true,
				Title:        stream.Title,
				CategoryID:   stream.CategoryID,
				CategoryName: stream.CategoryName,
				StartedAt:    stream.StartedAt,
				ViewerCount:  stream.ViewerCount,
				Language:     stream.Language,
				Tags:         stream.Tags,
				SampleTickAt: sampleTickAt,
				SampledAt:    sampledAt,
				Source:       Top500SnapshotSourceHelixStreams,
			}
			current = Top500Current{
				ChannelID:      channelID,
				Login:          login,
				DisplayName:    displayName,
				Rank:           channel.Rank,
				CoverageSource: Top500CoverageSourceMetadata,
				IsLive:         true,
				Title:          stream.Title,
				CategoryID:     stream.CategoryID,
				CategoryName:   stream.CategoryName,
				StartedAt:      stream.StartedAt,
				ViewerCount:    stream.ViewerCount,
				Language:       stream.Language,
				Tags:           stream.Tags,
				SampledAt:      sampledAt,
				StaleAfter:     sampledAt.Add(DefaultTop500MetadataStaleAfter),
				LastSuccessAt:  &sampledAt,
			}
			if streamID != "" {
				snapshot.StreamID = &streamID
				current.StreamID = &streamID
			}
		}

		samples = append(samples, Top500MetadataSample{Channel: channel, Snapshot: snapshot, Current: current})
	}
	return samples
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func (r *Top500SamplerTickResult) addWriteModeClass(cfg Top500SamplerConfig) {
	if cfg.DryRun {
		r.addClass(Top500SamplerClassDryRun)
		recordTop500SamplerRollback(Top500SamplerClassDryRun, top500SamplerMode(cfg))
		return
	}
	if !cfg.WriteEnabled {
		r.addClass(Top500SamplerClassWriteDisabled)
		recordTop500SamplerRollback(Top500SamplerClassWriteDisabled, top500SamplerMode(cfg))
	}
}

func recordTop500SamplerConfigMetrics(cfg Top500SamplerConfig) {
	metrics.Top500MetadataSamplerEnabled.Set(boolFloat(cfg.Enabled))
	metrics.Top500MetadataDryRun.Set(boolFloat(cfg.DryRun))
	metrics.Top500MetadataWriteEnabled.Set(boolFloat(cfg.WriteEnabled))
	metrics.Top500MetadataTopNConfigured.Set(float64(normalizeTop500SamplerConfig(cfg).TopN))
}

func recordTop500ProviderError(operation, classification, mode string) {
	metrics.Top500MetadataProviderCallsTotal.WithLabelValues(operation, "error", "helix").Inc()
	metrics.Top500MetadataProviderErrorsTotal.WithLabelValues(operation, classification, "helix").Inc()
	if classification == Top500SamplerClassHelixRateLimited {
		metrics.Top500MetadataProviderRateLimitsTotal.WithLabelValues(operation, "helix").Inc()
	}
	recordTop500SamplerRollback(classification, mode)
}

func recordTop500SamplerWrite(result, mode string, batchSize int, duration time.Duration) {
	metrics.Top500MetadataWriteBatchSize.WithLabelValues(result, mode, "write_samples").Set(float64(batchSize))
	metrics.Top500MetadataWriteLatencySeconds.WithLabelValues(result, mode, "write_samples").Set(duration.Seconds())
}

func recordTop500SamplerLockUnavailable(reason, mode string) {
	metrics.Top500MetadataLockUnavailableTotal.WithLabelValues(reason, mode).Inc()
	recordTop500SamplerRollback(reason, mode)
}

func recordTop500SamplerRollback(reason, mode string) {
	metrics.Top500MetadataSamplesDegradedTotal.WithLabelValues(reason, mode).Inc()
	metrics.Top500MetadataRollbackState.WithLabelValues(reason, mode).Set(1)
}

func resetTop500SamplerRollbackState(mode string) {
	for _, reason := range []string{
		Top500SamplerClassSamplerDisabled,
		Top500SamplerClassWriteDisabled,
		Top500SamplerClassDryRun,
		Top500SamplerClassLockUnavailable,
		Top500SamplerClassHelixRateLimited,
		Top500SamplerClassHelixAuthMissing,
		Top500SamplerClassHelixTransientError,
		Top500SamplerClassHelixNotFound,
		Top500SamplerClassProviderUnavailable,
		"missing_locker",
		"lock_error",
		"store_error",
	} {
		metrics.Top500MetadataRollbackState.WithLabelValues(reason, mode).Set(0)
	}
}

func top500MetadataMaxFreshnessSeconds(samples []Top500MetadataSample, now time.Time) float64 {
	var max float64
	for _, sample := range samples {
		if sample.Current.SampledAt.IsZero() {
			continue
		}
		seconds := now.Sub(sample.Current.SampledAt).Seconds()
		if seconds < 0 {
			seconds = 0
		}
		if seconds > max {
			max = seconds
		}
	}
	return max
}

func top500SamplerMode(cfg Top500SamplerConfig) string {
	switch {
	case !cfg.Enabled:
		return "disabled"
	case cfg.DryRun:
		return "dry_run"
	case !cfg.WriteEnabled:
		return "write_disabled"
	default:
		return "write_enabled"
	}
}

func boolFloat(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

func (r *Top500SamplerTickResult) addClass(classification string) {
	if classification == "" {
		return
	}
	for _, existing := range r.Classifications {
		if existing == classification {
			return
		}
	}
	r.Classifications = append(r.Classifications, classification)
}

func (s *Store) ListEnabledTop500Channels(ctx context.Context, limit int) ([]Top500Channel, error) {
	if limit <= 0 || limit > MaxTop500MetadataTopN {
		limit = DefaultTop500MetadataTopN
	}
	rows, err := s.db.Query(ctx, `
		SELECT channel_id, login, display_name, rank, source, source_version, seeded_by,
			effective_at, source_metadata, enabled, last_seen_at, last_sampled_at, last_live_at
		FROM top500_channels
		WHERE enabled = true AND source IN ('operator_seed', 'configured')
		ORDER BY rank ASC, login ASC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	channels := []Top500Channel{}
	for rows.Next() {
		channel, err := scanTop500Channel(rows)
		if err != nil {
			return nil, err
		}
		channels = append(channels, channel)
	}
	return channels, rows.Err()
}

type top500ChannelScanner interface {
	Scan(dest ...any) error
}

func scanTop500Channel(row top500ChannelScanner) (Top500Channel, error) {
	var channel Top500Channel
	var sourceMetadata []byte
	var lastSeenAt sql.NullTime
	var lastSampledAt sql.NullTime
	var lastLiveAt sql.NullTime
	if err := row.Scan(
		&channel.ChannelID, &channel.Login, &channel.DisplayName, &channel.Rank, &channel.Source,
		&channel.SourceVersion, &channel.SeededBy, &channel.EffectiveAt, &sourceMetadata, &channel.Enabled,
		&lastSeenAt, &lastSampledAt, &lastLiveAt,
	); err != nil {
		return Top500Channel{}, err
	}
	if len(sourceMetadata) > 0 {
		if err := json.Unmarshal(sourceMetadata, &channel.SourceMetadata); err != nil {
			return Top500Channel{}, err
		}
	}
	if lastSeenAt.Valid {
		channel.LastSeenAt = &lastSeenAt.Time
	}
	if lastSampledAt.Valid {
		channel.LastSampledAt = &lastSampledAt.Time
	}
	if lastLiveAt.Valid {
		channel.LastLiveAt = &lastLiveAt.Time
	}
	return channel, nil
}

func (s *Store) TryTop500MetadataSamplerLock(ctx context.Context) (Top500MetadataSamplerLock, bool, error) {
	if s == nil || s.db == nil {
		return nil, false, nil
	}
	conn, err := s.db.Acquire(ctx)
	if err != nil {
		return nil, false, err
	}
	var acquired bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, top500MetadataSamplerAdvisoryLockKey).Scan(&acquired); err != nil {
		conn.Release()
		return nil, false, err
	}
	if !acquired {
		conn.Release()
		return nil, false, nil
	}
	return &top500MetadataPostgresLock{conn: conn}, true, nil
}

type top500MetadataPostgresLock struct {
	conn *pgxpool.Conn
}

func (l *top500MetadataPostgresLock) Release(ctx context.Context) error {
	if l == nil || l.conn == nil {
		return nil
	}
	defer l.conn.Release()
	var released bool
	return l.conn.QueryRow(ctx, `SELECT pg_advisory_unlock($1)`, top500MetadataSamplerAdvisoryLockKey).Scan(&released)
}
