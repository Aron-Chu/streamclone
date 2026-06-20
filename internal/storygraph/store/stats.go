package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"streamclone/internal/archive"
)

// DirectorySample is one row from a directory sampler run.
type DirectorySample struct {
	TwitchLogin string
	TwitchID    string
	DisplayName string
	Category    string
	Viewers     int
	Rank        int
	IsLive      bool
	SampleRunID string
	SampledAt   time.Time
}

// ViewerPoint is a viewer count at a sample time for sparklines.
type ViewerPoint struct {
	Viewers   int       `json:"viewers"`
	Rank      int       `json:"rank"`
	SampledAt time.Time `json:"sampledAt"`
}

// RisingStreamer is a ranked rising streamer row.
type RisingStreamer struct {
	Login          string        `json:"login"`
	DisplayName    string        `json:"displayName"`
	Category       string        `json:"category,omitempty"`
	AvatarURL      string        `json:"avatarUrl,omitempty"`
	Window         string        `json:"window"`
	ViewersNow     int           `json:"viewersNow"`
	ViewersPrev    int           `json:"viewersPrev"`
	ViewerDeltaPct float64       `json:"viewerDeltaPct"`
	RankNow        int           `json:"rankNow"`
	RankPrev       int           `json:"rankPrev"`
	RankDelta      int           `json:"rankDelta"`
	NewEntrant     bool          `json:"newEntrant"`
	ClipVelocity   float64       `json:"clipVelocity"`
	RisingScore    float64       `json:"risingScore"`
	ComputedAt     time.Time     `json:"computedAt"`
	ViewerSeries   []ViewerPoint `json:"viewerSeries,omitempty"`
	TopStoryID     *int64        `json:"topStoryId,omitempty"`
	TopStoryTitle  string        `json:"topStoryTitle,omitempty"`
}

// StreamerStatProfile is the per-streamer stats drilldown payload.
type StreamerStatProfile struct {
	Login         string          `json:"login"`
	DisplayName   string          `json:"displayName,omitempty"`
	Category      string          `json:"category,omitempty"`
	AvatarURL     string          `json:"avatarUrl,omitempty"`
	ViewersNow    int             `json:"viewersNow"`
	RankNow       int             `json:"rankNow"`
	IsLive        bool            `json:"isLive"`
	StatsSource   string          `json:"statsSource,omitempty"`
	LastSampleAt  *time.Time      `json:"lastSampleAt,omitempty"`
	ViewerSeries  []ViewerPoint   `json:"viewerSeries"`
	Rising        *RisingStreamer `json:"rising,omitempty"`
	FollowersNow  *int64          `json:"followersNow,omitempty"`
	FollowerDelta *int64          `json:"followerDelta,omitempty"`
	RecentStories []StoryCard     `json:"recentStories"`
}

// DailyEdition is the daily Pulse Wire dashboard summary.
type DailyEdition struct {
	Date         string           `json:"date"`
	TotalLive    int              `json:"totalLive"`
	TopGainers   []RisingStreamer `json:"topGainers"`
	TopDroppers  []RisingStreamer `json:"topDroppers"`
	NewEntrants  []RisingStreamer `json:"newEntrants"`
	BansOfTheDay []StoryCard      `json:"bansOfTheDay"`
	TopStories   []StoryCard      `json:"topStories"`
}

// RisingRow is input for upserting computed rising scores.
type RisingRow struct {
	TwitchLogin    string
	Window         string
	ViewersNow     int
	ViewersPrev    int
	ViewerDeltaPct float64
	RankNow        int
	RankPrev       int
	RankDelta      int
	NewEntrant     bool
	ClipVelocity   float64
	RisingScore    float64
	ComputedAt     time.Time
}

func (s *Store) InsertDirectorySamples(ctx context.Context, samples []DirectorySample) error {
	if len(samples) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, sample := range samples {
		batch.Queue(`
			INSERT INTO directory_samples
				(twitch_login, twitch_id, display_name, category, viewers, rank, is_live, sample_run_id, sampled_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			strings.ToLower(strings.TrimSpace(sample.TwitchLogin)),
			strings.TrimSpace(sample.TwitchID),
			strings.TrimSpace(sample.DisplayName),
			strings.TrimSpace(sample.Category),
			sample.Viewers,
			sample.Rank,
			sample.IsLive,
			sample.SampleRunID,
			sample.SampledAt,
		)
	}
	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range samples {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return br.Close()
}

func (s *Store) ViewerSeriesForLogin(ctx context.Context, login string, since time.Time, limit int) ([]ViewerPoint, error) {
	login = strings.ToLower(strings.TrimSpace(login))
	if login == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 48
	}
	rows, err := s.pool.Query(ctx, `
		SELECT viewers, rank, sampled_at
		FROM directory_samples
		WHERE twitch_login = $1 AND sampled_at >= $2
		ORDER BY sampled_at ASC
		LIMIT $3`, login, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ViewerPoint
	for rows.Next() {
		var pt ViewerPoint
		if err := rows.Scan(&pt.Viewers, &pt.Rank, &pt.SampledAt); err != nil {
			return nil, err
		}
		out = append(out, pt)
	}
	return out, rows.Err()
}

func (s *Store) RisingCandidates(ctx context.Context, window, category string, limit int) ([]RisingStreamer, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	window = strings.ToLower(strings.TrimSpace(window))
	if window == "" {
		window = "today"
	}
	args := []any{window, limit}
	categoryFilter := ""
	if cat := strings.TrimSpace(category); cat != "" {
		categoryFilter = `
			AND EXISTS (
				SELECT 1 FROM directory_samples ds
				WHERE ds.twitch_login = sr.twitch_login
				  AND ds.sampled_at = (
				    SELECT MAX(sampled_at) FROM directory_samples WHERE twitch_login = sr.twitch_login
				  )
				  AND LOWER(ds.category) = LOWER($3)
			)`
		args = append(args, cat)
	}
	query := fmt.Sprintf(`
		SELECT sr.twitch_login, sr."window", sr.viewers_now, sr.viewers_prev, sr.viewer_delta_pct,
		       sr.rank_now, sr.rank_prev, sr.rank_delta, sr.new_entrant, sr.clip_velocity,
		       sr.rising_score, sr.computed_at,
		       COALESCE(ds.display_name, sr.twitch_login), COALESCE(ds.category, '')
		FROM streamer_rising sr
		LEFT JOIN LATERAL (
			SELECT display_name, category
			FROM directory_samples
			WHERE twitch_login = sr.twitch_login
			ORDER BY sampled_at DESC
			LIMIT 1
		) ds ON true
		WHERE sr."window" = $1%s
		ORDER BY sr.rising_score DESC
		LIMIT $2`, categoryFilter)
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RisingStreamer
	for rows.Next() {
		var row RisingStreamer
		if err := rows.Scan(
			&row.Login, &row.Window, &row.ViewersNow, &row.ViewersPrev, &row.ViewerDeltaPct,
			&row.RankNow, &row.RankPrev, &row.RankDelta, &row.NewEntrant, &row.ClipVelocity,
			&row.RisingScore, &row.ComputedAt, &row.DisplayName, &row.Category,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) UpsertRisingRows(ctx context.Context, rows []RisingRow) error {
	if len(rows) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, row := range rows {
		batch.Queue(`
			INSERT INTO streamer_rising
				(twitch_login, "window", viewers_now, viewers_prev, viewer_delta_pct, rank_now, rank_prev,
				 rank_delta, new_entrant, clip_velocity, rising_score, computed_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
			ON CONFLICT (twitch_login, "window") DO UPDATE SET
				viewers_now = EXCLUDED.viewers_now,
				viewers_prev = EXCLUDED.viewers_prev,
				viewer_delta_pct = EXCLUDED.viewer_delta_pct,
				rank_now = EXCLUDED.rank_now,
				rank_prev = EXCLUDED.rank_prev,
				rank_delta = EXCLUDED.rank_delta,
				new_entrant = EXCLUDED.new_entrant,
				clip_velocity = EXCLUDED.clip_velocity,
				rising_score = EXCLUDED.rising_score,
				computed_at = EXCLUDED.computed_at`,
			strings.ToLower(strings.TrimSpace(row.TwitchLogin)),
			row.Window,
			row.ViewersNow,
			row.ViewersPrev,
			row.ViewerDeltaPct,
			row.RankNow,
			row.RankPrev,
			row.RankDelta,
			row.NewEntrant,
			row.ClipVelocity,
			row.RisingScore,
			row.ComputedAt,
		)
	}
	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range rows {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return br.Close()
}

func (s *Store) DeleteExpiredDirectorySamples(ctx context.Context, retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		retentionDays = 30
	}
	if s.archiveProtectRetention {
		var missingDirectory int64
		err := s.pool.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM directory_samples ds
			WHERE ds.sampled_at < now() - ($1::int * interval '1 day')
			  AND NOT EXISTS (
				SELECT 1
				FROM archive_exports ae
				WHERE ae.artifact_type = $2
				  AND ae.natural_key = ds.sample_run_id || ':' || ds.twitch_login
				  AND ae.export_status = 'confirmed'
			  )`,
			retentionDays, archive.ArtifactDirectorySample,
		).Scan(&missingDirectory)
		if err != nil {
			return 0, err
		}
		if err := archive.BlockIfMissing(archive.ArtifactDirectorySample, missingDirectory); err != nil {
			return 0, err
		}
		var missingFollowers int64
		err = s.pool.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM streamer_follower_snapshots fs
			WHERE fs.sampled_at < now() - ($1::int * interval '1 day')
			  AND NOT EXISTS (
				SELECT 1
				FROM archive_exports ae
				WHERE ae.artifact_type = $2
				  AND ae.natural_key = fs.twitch_login || ':' || to_char(fs.sampled_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
				  AND ae.export_status = 'confirmed'
			  )`,
			retentionDays, archive.ArtifactFollowerSample,
		).Scan(&missingFollowers)
		if err != nil {
			return 0, err
		}
		if err := archive.BlockIfMissing(archive.ArtifactFollowerSample, missingFollowers); err != nil {
			return 0, err
		}
	}
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM directory_samples
		WHERE sampled_at < now() - ($1::int * interval '1 day')`, retentionDays)
	if err != nil {
		return 0, err
	}
	_, _ = s.pool.Exec(ctx, `
		DELETE FROM streamer_follower_snapshots
		WHERE sampled_at < now() - ($1::int * interval '1 day')`, retentionDays)
	return tag.RowsAffected(), nil
}

func (s *Store) DirectorySeedLogins(ctx context.Context, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.pool.Query(ctx, `
		SELECT twitch_login
		FROM directory_samples
		WHERE sample_run_id = (SELECT sample_run_id FROM directory_samples ORDER BY sampled_at DESC LIMIT 1)
		ORDER BY rank ASC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var login string
		if err := rows.Scan(&login); err != nil {
			return nil, err
		}
		out = append(out, login)
	}
	return out, rows.Err()
}

func (s *Store) LatestSamplesForRun(ctx context.Context, runID string) ([]DirectorySample, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT twitch_login, twitch_id, display_name, category, viewers, rank, is_live, sample_run_id, sampled_at
		FROM directory_samples
		WHERE sample_run_id = $1
		ORDER BY rank ASC`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DirectorySample
	for rows.Next() {
		var sample DirectorySample
		if err := rows.Scan(
			&sample.TwitchLogin, &sample.TwitchID, &sample.DisplayName, &sample.Category,
			&sample.Viewers, &sample.Rank, &sample.IsLive, &sample.SampleRunID, &sample.SampledAt,
		); err != nil {
			return nil, err
		}
		out = append(out, sample)
	}
	return out, rows.Err()
}

func (s *Store) FirstSampleSince(ctx context.Context, login string, since time.Time) (*DirectorySample, error) {
	login = strings.ToLower(strings.TrimSpace(login))
	var sample DirectorySample
	err := s.pool.QueryRow(ctx, `
		SELECT twitch_login, twitch_id, display_name, category, viewers, rank, is_live, sample_run_id, sampled_at
		FROM directory_samples
		WHERE twitch_login = $1 AND sampled_at >= $2
		ORDER BY sampled_at ASC
		LIMIT 1`, login, since).Scan(
		&sample.TwitchLogin, &sample.TwitchID, &sample.DisplayName, &sample.Category,
		&sample.Viewers, &sample.Rank, &sample.IsLive, &sample.SampleRunID, &sample.SampledAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &sample, nil
}

func (s *Store) LatestSampleForLogin(ctx context.Context, login string) (*DirectorySample, error) {
	login = strings.ToLower(strings.TrimSpace(login))
	var sample DirectorySample
	err := s.pool.QueryRow(ctx, `
		SELECT twitch_login, twitch_id, display_name, category, viewers, rank, is_live, sample_run_id, sampled_at
		FROM directory_samples
		WHERE twitch_login = $1
		ORDER BY sampled_at DESC
		LIMIT 1`, login).Scan(
		&sample.TwitchLogin, &sample.TwitchID, &sample.DisplayName, &sample.Category,
		&sample.Viewers, &sample.Rank, &sample.IsLive, &sample.SampleRunID, &sample.SampledAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &sample, nil
}

func (s *Store) ClipVelocityForLogin(ctx context.Context, login string, since time.Time) (float64, error) {
	login = strings.ToLower(strings.TrimSpace(login))
	ent, err := s.EntityByLogin(ctx, login)
	if err != nil || ent == nil {
		return 0, err
	}
	var count int
	err = s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM story_evidence se
		JOIN story_clusters sc ON sc.id = se.cluster_id
		WHERE sc.entity_id = $1
		  AND se.source_type = 'twitch_clip'
		  AND COALESCE(se.occurred_at, se.id::text::timestamptz) >= $2`, ent.ID, since).Scan(&count)
	if err != nil {
		return 0, err
	}
	hours := time.Since(since).Hours()
	if hours < 1 {
		hours = 1
	}
	return float64(count) / hours, nil
}

func (s *Store) CountLiveInLatestRun(ctx context.Context) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM directory_samples
		WHERE sample_run_id = (SELECT sample_run_id FROM directory_samples ORDER BY sampled_at DESC LIMIT 1)
		  AND is_live = true`).Scan(&count)
	return count, err
}

func (s *Store) SumViewersInLatestRun(ctx context.Context) (int, error) {
	var total int
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(viewers), 0)
		FROM directory_samples
		WHERE sample_run_id = (SELECT sample_run_id FROM directory_samples ORDER BY sampled_at DESC LIMIT 1)`).Scan(&total)
	return total, err
}

func (s *Store) TopStoryForLogin(ctx context.Context, login string, since time.Time) (*int64, string, error) {
	ent, err := s.EntityByLogin(ctx, login)
	if err != nil || ent == nil {
		return nil, "", err
	}
	var id int64
	var title string
	err = s.pool.QueryRow(ctx, `
		SELECT sc.id, COALESCE(sc.title, '')
		FROM story_clusters sc
		LEFT JOIN story_scores ss ON ss.cluster_id = sc.id
		WHERE sc.entity_id = $1
		  AND sc.updated_at >= $2
		  AND sc.state IN ('published', 'developing', 'unverified', 'settled')
		ORDER BY COALESCE(ss.trend, 0) DESC, sc.updated_at DESC
		LIMIT 1`, ent.ID, since).Scan(&id, &title)
	if err == pgx.ErrNoRows {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	return &id, title, nil
}

func (s *Store) DailyEdition(ctx context.Context, day time.Time, storyLimit int) (*DailyEdition, error) {
	if storyLimit <= 0 {
		storyLimit = 10
	}
	day = day.UTC()
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	totalLive, _ := s.CountLiveInLatestRun(ctx)

	gainers, _ := s.RisingCandidates(ctx, "today", "", 5)
	droppers, err := s.pool.Query(ctx, `
		SELECT sr.twitch_login, sr."window", sr.viewers_now, sr.viewers_prev, sr.viewer_delta_pct,
		       sr.rank_now, sr.rank_prev, sr.rank_delta, sr.new_entrant, sr.clip_velocity,
		       sr.rising_score, sr.computed_at,
		       COALESCE(ds.display_name, sr.twitch_login), COALESCE(ds.category, '')
		FROM streamer_rising sr
		LEFT JOIN LATERAL (
			SELECT display_name, category FROM directory_samples
			WHERE twitch_login = sr.twitch_login ORDER BY sampled_at DESC LIMIT 1
		) ds ON true
		WHERE sr."window" = 'today' AND sr.viewer_delta_pct < 0
		ORDER BY sr.viewer_delta_pct ASC
		LIMIT 5`)
	if err != nil {
		return nil, err
	}
	var topDroppers []RisingStreamer
	for droppers.Next() {
		var row RisingStreamer
		if err := droppers.Scan(
			&row.Login, &row.Window, &row.ViewersNow, &row.ViewersPrev, &row.ViewerDeltaPct,
			&row.RankNow, &row.RankPrev, &row.RankDelta, &row.NewEntrant, &row.ClipVelocity,
			&row.RisingScore, &row.ComputedAt, &row.DisplayName, &row.Category,
		); err != nil {
			droppers.Close()
			return nil, err
		}
		topDroppers = append(topDroppers, row)
	}
	droppers.Close()

	newEntrants, _ := s.pool.Query(ctx, `
		SELECT sr.twitch_login, sr."window", sr.viewers_now, sr.viewers_prev, sr.viewer_delta_pct,
		       sr.rank_now, sr.rank_prev, sr.rank_delta, sr.new_entrant, sr.clip_velocity,
		       sr.rising_score, sr.computed_at,
		       COALESCE(ds.display_name, sr.twitch_login), COALESCE(ds.category, '')
		FROM streamer_rising sr
		LEFT JOIN LATERAL (
			SELECT display_name, category FROM directory_samples
			WHERE twitch_login = sr.twitch_login ORDER BY sampled_at DESC LIMIT 1
		) ds ON true
		WHERE sr."window" = 'today' AND sr.new_entrant = true
		ORDER BY sr.rising_score DESC
		LIMIT 5`)
	var entrants []RisingStreamer
	for newEntrants.Next() {
		var row RisingStreamer
		if err := newEntrants.Scan(
			&row.Login, &row.Window, &row.ViewersNow, &row.ViewersPrev, &row.ViewerDeltaPct,
			&row.RankNow, &row.RankPrev, &row.RankDelta, &row.NewEntrant, &row.ClipVelocity,
			&row.RisingScore, &row.ComputedAt, &row.DisplayName, &row.Category,
		); err != nil {
			newEntrants.Close()
			return nil, err
		}
		entrants = append(entrants, row)
	}
	newEntrants.Close()

	bans, _ := s.ListFeed(ctx, "", "bans", "", "rank", "today", start, storyLimit, 0)
	topStories, _ := s.ListFeed(ctx, "published", "", "", "rank", "today", start, storyLimit, 0)
	_ = end

	return &DailyEdition{
		Date:         start.Format("2006-01-02"),
		TotalLive:    totalLive,
		TopGainers:   gainers,
		TopDroppers:  topDroppers,
		NewEntrants:  entrants,
		BansOfTheDay: bans,
		TopStories:   topStories,
	}, nil
}

func (s *Store) StreamerProfile(ctx context.Context, login string, since time.Time) (*StreamerStatProfile, error) {
	login = strings.ToLower(strings.TrimSpace(login))
	sample, err := s.LatestSampleForLogin(ctx, login)
	if err != nil {
		return nil, err
	}
	series, err := s.ViewerSeriesForLogin(ctx, login, since, 48)
	if err != nil {
		return nil, err
	}
	stories, err := s.SpreadForLogin(ctx, login, 10)
	if err != nil {
		return nil, err
	}
	profile := &StreamerStatProfile{
		Login:         login,
		ViewerSeries:  series,
		RecentStories: stories,
	}
	if sample != nil {
		profile.DisplayName = sample.DisplayName
		profile.Category = sample.Category
		profile.ViewersNow = sample.Viewers
		profile.RankNow = sample.Rank
		profile.IsLive = sample.IsLive
		profile.StatsSource = "directory_sample"
		profile.LastSampleAt = &sample.SampledAt
	} else {
		profile.StatsSource = "none"
	}
	rising, _ := s.RisingCandidates(ctx, "today", "", 100)
	for i := range rising {
		if strings.EqualFold(rising[i].Login, login) {
			profile.Rising = &rising[i]
			break
		}
	}
	var followersNow int64
	err = s.pool.QueryRow(ctx, `
		SELECT followers FROM streamer_follower_snapshots
		WHERE twitch_login = $1 ORDER BY sampled_at DESC LIMIT 1`, login).Scan(&followersNow)
	if err == nil {
		profile.FollowersNow = &followersNow
		var prev int64
		if err := s.pool.QueryRow(ctx, `
			SELECT followers FROM streamer_follower_snapshots
			WHERE twitch_login = $1 AND sampled_at < now() - interval '24 hours'
			ORDER BY sampled_at DESC LIMIT 1`, login).Scan(&prev); err == nil {
			delta := followersNow - prev
			profile.FollowerDelta = &delta
		}
	}
	return profile, nil
}

func (s *Store) InsertFollowerSnapshot(ctx context.Context, login string, followers int64, at time.Time) error {
	login = strings.ToLower(strings.TrimSpace(login))
	_, err := s.pool.Exec(ctx, `
		INSERT INTO streamer_follower_snapshots (twitch_login, followers, sampled_at)
		VALUES ($1, $2, $3)`, login, followers, at)
	return err
}
