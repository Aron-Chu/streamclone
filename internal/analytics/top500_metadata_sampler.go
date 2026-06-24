package analytics

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
)

const (
	DefaultTop500MetadataTopN            = 100
	DefaultTop500MetadataBatchSize       = 100
	DefaultTop500MetadataLiveInterval    = 60 * time.Second
	DefaultTop500MetadataOfflineInterval = 10 * time.Minute

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
	cfg      Top500SamplerConfig
	store    Top500MetadataStore
	provider Top500MetadataProvider
	locker   Top500MetadataSamplerLocker
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
	if cfg.TopN <= 0 || cfg.TopN > DefaultTop500MetadataTopN {
		cfg.TopN = DefaultTop500MetadataTopN
	}
	if cfg.BatchSize <= 0 || cfg.BatchSize > DefaultTop500MetadataBatchSize {
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
		return result, nil
	}
	cfg := normalizeTop500SamplerConfig(s.cfg)
	if !cfg.Enabled {
		result.addClass(Top500SamplerClassSamplerDisabled)
		return result, nil
	}

	if s.locker != nil {
		lock, acquired, err := s.locker.TryTop500MetadataSamplerLock(ctx)
		if err != nil {
			result.addClass(Top500SamplerClassLockUnavailable)
			return result, err
		}
		if !acquired {
			result.addClass(Top500SamplerClassLockUnavailable)
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
	if len(result.Planned) == 0 {
		result.addWriteModeClass(cfg)
		return result, nil
	}
	if s.provider == nil {
		result.addClass(Top500SamplerClassProviderUnavailable)
		return result, nil
	}

	streams, err := s.provider.FetchStreams(ctx, result.Planned)
	if err != nil {
		result.addClass(classifyTop500ProviderError(err))
		return result, nil
	}
	result.StreamsFetched = len(streams)

	users, err := s.provider.FetchUsers(ctx, result.Planned)
	if err != nil {
		result.addClass(classifyTop500ProviderError(err))
		return result, nil
	}
	result.UsersFetched = len(users)
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
	return result, nil
}

func filterTop500SamplerRoster(channels []Top500Channel, topN int) []Top500Channel {
	if topN <= 0 || topN > DefaultTop500MetadataTopN {
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

func (r *Top500SamplerTickResult) addWriteModeClass(cfg Top500SamplerConfig) {
	if cfg.DryRun {
		r.addClass(Top500SamplerClassDryRun)
		return
	}
	r.addClass(Top500SamplerClassWriteDisabled)
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
	if limit <= 0 || limit > DefaultTop500MetadataTopN {
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
	var acquired bool
	if err := s.db.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, top500MetadataSamplerAdvisoryLockKey).Scan(&acquired); err != nil {
		return nil, false, err
	}
	if !acquired {
		return nil, false, nil
	}
	return top500MetadataPostgresLock{store: s}, true, nil
}

type top500MetadataPostgresLock struct {
	store *Store
}

func (l top500MetadataPostgresLock) Release(ctx context.Context) error {
	var released bool
	return l.store.db.QueryRow(ctx, `SELECT pg_advisory_unlock($1)`, top500MetadataSamplerAdvisoryLockKey).Scan(&released)
}
