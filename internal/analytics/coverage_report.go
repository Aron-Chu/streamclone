package analytics

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"streamclone/internal/archive"
)

// ParseCoverageSince parses CLI/query since values ("7d", "24h", Go duration, RFC3339).
// Empty defaults to 7 days before now.
func ParseCoverageSince(raw string, now time.Time) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return now.Add(-7 * 24 * time.Hour), nil
	}
	if strings.HasSuffix(raw, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(raw, "d"))
		if err == nil && n > 0 {
			return now.Add(-time.Duration(n) * 24 * time.Hour), nil
		}
	}
	if d, err := time.ParseDuration(raw); err == nil && d > 0 {
		return now.Add(-d), nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("invalid since %q", raw)
}

const (
	CoverageLiveGood = 85.0
	CoveragePartial  = 20.0
)

type CoverageClass string

const (
	CoverageClassLiveGood   CoverageClass = "live_good"
	CoverageClassPartial    CoverageClass = "partial"
	CoverageClassTTRequired CoverageClass = "tt_required"
)

// ClassifyCoveragePct maps Tier-0 coverage percent to acceptance buckets.
func ClassifyCoveragePct(pct float64) CoverageClass {
	switch {
	case pct >= CoverageLiveGood:
		return CoverageClassLiveGood
	case pct >= CoveragePartial:
		return CoverageClassPartial
	default:
		return CoverageClassTTRequired
	}
}

type RosterSummary struct {
	TopN           int      `json:"topN"`
	TotalTracked   int      `json:"totalTracked"`
	AlwaysTracked  int      `json:"alwaysTracked"`
	WithRecentLive int      `json:"withRecentLive"`
	Logins         []string `json:"logins,omitempty"`
}

type StreamCoverageEntry struct {
	StreamID     string        `json:"streamId"`
	Login        string        `json:"login"`
	StartedAt    time.Time     `json:"startedAt"`
	EndedAt      *time.Time    `json:"endedAt,omitempty"`
	DurationMin  int           `json:"durationMin"`
	CoveragePct  float64       `json:"coveragePct"`
	Class        CoverageClass `json:"class"`
	ViewerSource string        `json:"viewerSource,omitempty"`
}

type StreamCoverageSummary struct {
	Total      int `json:"total"`
	LiveGood   int `json:"liveGood"`
	Partial    int `json:"partial"`
	TTRequired int `json:"ttRequired"`
	Live       int `json:"live"`
	Ended      int `json:"ended"`
}

type BackfillJobCounts struct {
	Tier   string `json:"tier"`
	Status string `json:"status"`
	Count  int    `json:"count"`
}

type ArchiveExportCounts struct {
	ArtifactType string `json:"artifactType"`
	Status       string `json:"status"`
	Count        int    `json:"count"`
}

type AzureGapCheck struct {
	RosterMissingVODIndex []string `json:"rosterMissingVodIndex,omitempty"`
	StreamsMissingRollups []string `json:"streamsMissingRollups,omitempty"`
}

type CoverageReport struct {
	GeneratedAt    time.Time             `json:"generatedAt"`
	Since          time.Time             `json:"since"`
	Roster         RosterSummary         `json:"roster"`
	BronzeIndex    []BronzeIndexState    `json:"bronzeIndex"`
	Streams        []StreamCoverageEntry `json:"streams"`
	StreamSummary  StreamCoverageSummary `json:"streamSummary"`
	BackfillJobs   []BackfillJobCounts   `json:"backfillJobs"`
	ArchiveExports []ArchiveExportCounts `json:"archiveExports"`
	AzureGaps      AzureGapCheck         `json:"azureGaps"`
}

// BuildCoverageReport assembles fleet progress for Bronze/Tier-0 acceptance runs.
func BuildCoverageReport(ctx context.Context, db *pgxpool.Pool, store *Store, since time.Time, rosterTopN int) (*CoverageReport, error) {
	if db == nil {
		return nil, fmt.Errorf("db unavailable")
	}
	if store == nil {
		store = NewStore(db)
	}
	if rosterTopN <= 0 {
		rosterTopN = 200
	}
	if since.IsZero() {
		since = time.Now().UTC().Add(-24 * time.Hour)
	}

	report := &CoverageReport{
		GeneratedAt: time.Now().UTC(),
		Since:       since.UTC(),
	}

	roster, err := queryRosterSummary(ctx, db, rosterTopN)
	if err != nil {
		return nil, fmt.Errorf("roster: %w", err)
	}
	report.Roster = roster

	bronze, err := ListBronzeIndexState(ctx, db, rosterTopN)
	if err != nil {
		return nil, fmt.Errorf("bronze index: %w", err)
	}
	report.BronzeIndex = bronze

	streams, summary, err := queryStreamCoverage(ctx, store, since)
	if err != nil {
		return nil, fmt.Errorf("streams: %w", err)
	}
	report.Streams = streams
	report.StreamSummary = summary

	backfill, err := queryBackfillJobCounts(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("backfill jobs: %w", err)
	}
	report.BackfillJobs = backfill

	exports, err := queryArchiveExportCounts(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("archive exports: %w", err)
	}
	report.ArchiveExports = exports

	gaps, err := queryAzureGaps(ctx, db, roster.Logins, streams)
	if err != nil {
		return nil, fmt.Errorf("azure gaps: %w", err)
	}
	report.AzureGaps = gaps

	return report, nil
}

func queryRosterSummary(ctx context.Context, db *pgxpool.Pool, topN int) (RosterSummary, error) {
	rows, err := db.Query(ctx, `
		SELECT login, is_always_tracked,
			(last_seen_live_at IS NOT NULL AND last_seen_live_at > now() - interval '15 minutes') AS recent_live
		FROM tracked_streamers
		ORDER BY last_rank ASC NULLS LAST, login ASC
		LIMIT $1`, topN)
	if err != nil {
		return RosterSummary{}, err
	}
	defer rows.Close()

	summary := RosterSummary{TopN: topN}
	for rows.Next() {
		var login string
		var always, recent bool
		if err := rows.Scan(&login, &always, &recent); err != nil {
			return RosterSummary{}, err
		}
		login = normalizeLogin(login)
		if login == "" {
			continue
		}
		summary.TotalTracked++
		summary.Logins = append(summary.Logins, login)
		if always {
			summary.AlwaysTracked++
		}
		if recent {
			summary.WithRecentLive++
		}
	}
	return summary, rows.Err()
}

func queryStreamCoverage(ctx context.Context, store *Store, since time.Time) ([]StreamCoverageEntry, StreamCoverageSummary, error) {
	rows, err := store.db.Query(ctx, `
		SELECT stream_id, login, started_at, ended_at, COALESCE(viewer_source,'unknown')
		FROM analytics_streams
		WHERE started_at >= $1
		  AND COALESCE(canonical_stream_id, stream_id) = stream_id
		ORDER BY started_at DESC`, since.UTC())
	if err != nil {
		return nil, StreamCoverageSummary{}, err
	}
	defer rows.Close()

	var entries []StreamCoverageEntry
	var summary StreamCoverageSummary
	for rows.Next() {
		var entry StreamCoverageEntry
		if err := rows.Scan(&entry.StreamID, &entry.Login, &entry.StartedAt, &entry.EndedAt, &entry.ViewerSource); err != nil {
			return nil, StreamCoverageSummary{}, err
		}
		entry.Login = normalizeLogin(entry.Login)
		entry.ViewerSource = normalizeViewerSource(entry.ViewerSource)

		stream, err := store.StreamByID(ctx, entry.StreamID)
		if err != nil {
			return nil, StreamCoverageSummary{}, err
		}
		rollups, err := store.RollupsByStream(ctx, entry.StreamID)
		if err != nil {
			return nil, StreamCoverageSummary{}, err
		}
		entry.CoveragePct = Tier0CoveragePct(stream, rollups)
		entry.Class = ClassifyCoveragePct(entry.CoveragePct)
		entry.DurationMin = streamDurationSeconds(stream, rollups) / 60

		entries = append(entries, entry)
		summary.Total++
		if entry.EndedAt == nil {
			summary.Live++
		} else {
			summary.Ended++
		}
		switch entry.Class {
		case CoverageClassLiveGood:
			summary.LiveGood++
		case CoverageClassPartial:
			summary.Partial++
		case CoverageClassTTRequired:
			summary.TTRequired++
		}
	}
	return entries, summary, rows.Err()
}

func queryBackfillJobCounts(ctx context.Context, db *pgxpool.Pool) ([]BackfillJobCounts, error) {
	rows, err := db.Query(ctx, `
		SELECT tier, status, COUNT(*)::int
		FROM backfill_jobs
		GROUP BY tier, status
		ORDER BY tier, status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BackfillJobCounts
	for rows.Next() {
		var row BackfillJobCounts
		if err := rows.Scan(&row.Tier, &row.Status, &row.Count); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// BackfillTierCounts returns aggregate job counts grouped by tier+status for the
// Silver and Gold corpus tiers only. It powers the hosted-safe public hub corpus
// pipeline (counts only — never per-job rows, logins, stream IDs, or errors).
func (s *Store) BackfillTierCounts(ctx context.Context) ([]BackfillJobCounts, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	rows, err := s.db.Query(ctx, `
		SELECT tier, status, COUNT(*)::int
		FROM backfill_jobs
		WHERE tier IN ('silver', 'gold', 'gold_full', 'gold_lite')
		GROUP BY tier, status
		ORDER BY tier, status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BackfillJobCounts
	for rows.Next() {
		var row BackfillJobCounts
		if err := rows.Scan(&row.Tier, &row.Status, &row.Count); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// CorpusSilverEligibleCount estimates how many Top-N channels have bronze VOD
// catalogs available for the silver enqueuer to scan. The actual VOD rows live
// in blob storage, so this is a readiness signal rather than a queue mutation
// path.
func (s *Store) CorpusSilverEligibleCount(ctx context.Context, topN int) (int, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	if topN <= 0 {
		topN = DefaultTop500MetadataTopN
	}
	var count int
	err := s.db.QueryRow(ctx, `
		WITH candidate_logins AS (
			SELECT t.login
			FROM tracked_streamers t
			JOIN bronze_index_state b ON b.login = t.login
			WHERE b.last_helix_at IS NOT NULL
			  AND b.helix_row_count > 0
			ORDER BY t.last_rank ASC NULLS LAST, t.login ASC
			LIMIT $1
		)
		SELECT COUNT(*)::int
		FROM candidate_logins c
		WHERE NOT EXISTS (
			SELECT 1
			FROM backfill_jobs bj
			WHERE bj.tier = 'silver'
			  AND bj.login = c.login
			  AND bj.status IN ('queued', 'running')
		)`, topN).Scan(&count)
	return count, err
}

// CorpusGoldEligibleCount counts silver-complete streams that do not yet have
// an active or completed gold-family job.
func (s *Store) CorpusGoldEligibleCount(ctx context.Context) (int, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	var count int
	err := s.db.QueryRow(ctx, `
		SELECT COUNT(DISTINCT silver.stream_id)::int
		FROM backfill_jobs silver
		WHERE silver.tier = 'silver'
		  AND silver.status = 'done'
		  AND silver.export_status = 'confirmed'
		  AND COALESCE(silver.stream_id, '') <> ''
		  AND NOT EXISTS (
			SELECT 1
			FROM backfill_jobs gold
			WHERE gold.stream_id = silver.stream_id
			  AND gold.tier IN ('gold', 'gold_full', 'gold_lite')
			  AND gold.status IN ('queued', 'running', 'done')
		  )`).Scan(&count)
	return count, err
}

// BackfillOldestQueuedAgeSeconds returns the age of the oldest queued job for
// one or more tiers. A nil value means no queued jobs are present.
func (s *Store) BackfillOldestQueuedAgeSeconds(ctx context.Context, tiers ...string) (*int, error) {
	if s == nil || s.db == nil || len(tiers) == 0 {
		return nil, nil
	}
	normalized := make([]string, 0, len(tiers))
	for _, tier := range tiers {
		tier = strings.ToLower(strings.TrimSpace(tier))
		if tier != "" {
			normalized = append(normalized, tier)
		}
	}
	if len(normalized) == 0 {
		return nil, nil
	}
	var rawSeconds int
	err := s.db.QueryRow(ctx, `
		SELECT COALESCE(FLOOR(EXTRACT(EPOCH FROM now() - MIN(created_at)))::int, -1)
		FROM backfill_jobs
		WHERE tier = ANY($1)
		  AND status = 'queued'`, normalized).Scan(&rawSeconds)
	if err != nil || rawSeconds < 0 {
		return nil, err
	}
	return &rawSeconds, nil
}

func queryArchiveExportCounts(ctx context.Context, db *pgxpool.Pool) ([]ArchiveExportCounts, error) {
	rows, err := db.Query(ctx, `
		SELECT artifact_type, export_status, COUNT(*)::int
		FROM archive_exports
		GROUP BY artifact_type, export_status
		ORDER BY artifact_type, export_status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ArchiveExportCounts
	for rows.Next() {
		var row ArchiveExportCounts
		if err := rows.Scan(&row.ArtifactType, &row.Status, &row.Count); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func queryAzureGaps(ctx context.Context, db *pgxpool.Pool, rosterLogins []string, streams []StreamCoverageEntry) (AzureGapCheck, error) {
	var gaps AzureGapCheck
	if len(rosterLogins) == 0 {
		return gaps, nil
	}

	rows, err := db.Query(ctx, `
		SELECT ts.login
		FROM unnest($1::text[]) AS ts(login)
		WHERE NOT EXISTS (
			SELECT 1 FROM archive_exports ae
			WHERE ae.artifact_type = $2
			  AND ae.natural_key = 'vod_index:' || ts.login
			  AND ae.export_status = $3
		)
		ORDER BY ts.login`, rosterLogins, archive.ArtifactBronzeVODIndex, archive.StatusConfirmed)
	if err != nil {
		return gaps, err
	}
	for rows.Next() {
		var login string
		if err := rows.Scan(&login); err != nil {
			rows.Close()
			return gaps, err
		}
		gaps.RosterMissingVODIndex = append(gaps.RosterMissingVODIndex, login)
	}
	rows.Close()

	if len(streams) == 0 {
		return gaps, nil
	}
	streamIDs := make([]string, 0, len(streams))
	for _, s := range streams {
		if s.StreamID != "" {
			streamIDs = append(streamIDs, s.StreamID)
		}
	}
	rows, err = db.Query(ctx, `
		SELECT sid
		FROM unnest($1::text[]) AS sid
		WHERE NOT EXISTS (
			SELECT 1 FROM archive_exports ae
			WHERE ae.artifact_type = $2
			  AND ae.natural_key = 'rollups:' || sid
			  AND ae.export_status = $3
		)
		ORDER BY sid`, streamIDs, archive.ArtifactAnalyticsRollups, archive.StatusConfirmed)
	if err != nil {
		return gaps, err
	}
	for rows.Next() {
		var streamID string
		if err := rows.Scan(&streamID); err != nil {
			rows.Close()
			return gaps, err
		}
		gaps.StreamsMissingRollups = append(gaps.StreamsMissingRollups, streamID)
	}
	return gaps, rows.Err()
}

// FormatCoverageSummary renders a human-readable report header.
func FormatCoverageSummary(r *CoverageReport) string {
	if r == nil {
		return "coverage report unavailable"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Coverage report (since %s, generated %s)\n",
		r.Since.Format(time.RFC3339), r.GeneratedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "Roster: %d tracked (top %d), %d always-tracked, %d live now\n",
		r.Roster.TotalTracked, r.Roster.TopN, r.Roster.AlwaysTracked, r.Roster.WithRecentLive)
	fmt.Fprintf(&b, "Bronze index rows: %d\n", len(r.BronzeIndex))
	fmt.Fprintf(&b, "Streams in window: %d total (%d ended, %d live) — live_good=%d partial=%d tt_required=%d\n",
		r.StreamSummary.Total, r.StreamSummary.Ended, r.StreamSummary.Live,
		r.StreamSummary.LiveGood, r.StreamSummary.Partial, r.StreamSummary.TTRequired)
	if len(r.BackfillJobs) > 0 {
		b.WriteString("Backfill jobs:\n")
		for _, row := range r.BackfillJobs {
			fmt.Fprintf(&b, "  %s/%s: %d\n", row.Tier, row.Status, row.Count)
		}
	}
	if len(r.ArchiveExports) > 0 {
		b.WriteString("Archive exports:\n")
		for _, row := range r.ArchiveExports {
			fmt.Fprintf(&b, "  %s/%s: %d\n", row.ArtifactType, row.Status, row.Count)
		}
	}
	if n := len(r.AzureGaps.RosterMissingVODIndex); n > 0 {
		fmt.Fprintf(&b, "Azure gap: %d roster logins missing confirmed bronze_vod_index\n", n)
	}
	if n := len(r.AzureGaps.StreamsMissingRollups); n > 0 {
		fmt.Fprintf(&b, "Azure gap: %d streams missing confirmed rollups export\n", n)
	}
	return strings.TrimRight(b.String(), "\n")
}
