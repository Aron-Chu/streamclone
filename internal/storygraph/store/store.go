package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"streamclone/internal/archive"
	"streamclone/internal/storygraph/evidenceurl"
	"streamclone/internal/storygraph/windowmath"
)

// Store is the Postgres repository for story graph data.
type Store struct {
	pool                    *pgxpool.Pool
	archiveProtectRetention bool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) WithArchiveProtectRetention(enabled bool) *Store {
	if s != nil {
		s.archiveProtectRetention = enabled
	}
	return s
}

func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

type Entity struct {
	ID          int64           `json:"id"`
	TwitchLogin string          `json:"login,omitempty"`
	TwitchID    string          `json:"twitchId,omitempty"`
	DisplayName string          `json:"displayName,omitempty"`
	AvatarURL   string          `json:"avatarUrl,omitempty"`
	Aliases     json.RawMessage `json:"aliases,omitempty"`
}

type MomentFingerprint struct {
	ID                int64              `json:"id"`
	EntityID          *int64             `json:"entityId,omitempty"`
	StreamID          string             `json:"streamId"`
	VODID             string             `json:"vodId,omitempty"`
	VODOffsetS        int                `json:"vodOffsetS"`
	TranscriptKW      json.RawMessage    `json:"quotes,omitempty"`
	TopEmotes         json.RawMessage    `json:"topEmotes,omitempty"`
	ChatSpikeSummary  string             `json:"chatSpikeSummary,omitempty"`
	OriginConfidence  *float64           `json:"originConfidence,omitempty"`
	OriginSpikePoints []OriginSpikePoint `json:"originSpikePoints,omitempty"`
	Game              string             `json:"game,omitempty"`
	FPVersion         int                `json:"fpVersion"`
	CreatedAt         time.Time          `json:"createdAt"`
}

type OriginSpikePoint struct {
	OffsetS           int `json:"offsetS"`
	RelativeS         int `json:"relativeS"`
	ChatCount         int `json:"chatCount"`
	TotalEmoteCount   int `json:"totalEmoteCount"`
	SevenTVEmoteCount int `json:"sevenTvEmoteCount"`
	ViewerMax         int `json:"viewerMax"`
}

type OriginCandidateStream struct {
	StreamID   string
	VODID      string
	Login      string
	StartedAt  time.Time
	LastSeenAt time.Time
}

type SocialItem struct {
	ID           int64           `json:"id"`
	Source       string          `json:"source"`
	Kind         string          `json:"kind"`
	ExternalID   string          `json:"externalId"`
	URL          string          `json:"url"`
	Author       string          `json:"author,omitempty"`
	CreatedAtSrc *time.Time      `json:"createdAtSrc,omitempty"`
	Text         string          `json:"text,omitempty"`
	Metrics      json.RawMessage `json:"metrics,omitempty"`
	EntityID     *int64          `json:"entityId,omitempty"`
	ExpiresAt    time.Time       `json:"expiresAt"`
}

type StoryCluster struct {
	ID                 int64      `json:"id"`
	EntityID           *int64     `json:"entityId,omitempty"`
	MomentFPID         *int64     `json:"momentFpId,omitempty"`
	Title              string     `json:"title,omitempty"`
	Summary            string     `json:"summary,omitempty"`
	Category           string     `json:"category,omitempty"`
	StoryClass         string     `json:"storyClass,omitempty"`
	State              string     `json:"state"`
	OriginSearchStatus string     `json:"originSearchStatus,omitempty"`
	OriginCheckedAt    *time.Time `json:"originCheckedAt,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

type Evidence struct {
	ID         int64      `json:"id"`
	ClusterID  int64      `json:"clusterId"`
	ItemID     *int64     `json:"itemId,omitempty"`
	MomentFPID *int64     `json:"momentFpId,omitempty"`
	SourceType string     `json:"sourceType"`
	SourceURL  string     `json:"sourceUrl,omitempty"`
	MatchConf  float64    `json:"matchConf"`
	Weight     float64    `json:"weight"`
	OccurredAt *time.Time `json:"occurredAt,omitempty"`
}

type Scores struct {
	Trend      *float64        `json:"trend"`
	Volatility *float64        `json:"volatility"`
	Confidence *string         `json:"confidence"`
	Sentiment  *float64        `json:"sentiment"`
	Factors    json.RawMessage `json:"factors,omitempty"`
	UpdatedAt  time.Time       `json:"updatedAt"`
}

type StoryCard struct {
	Cluster          StoryCluster       `json:"story"`
	Entity           *Entity            `json:"entity,omitempty"`
	Origin           *MomentFingerprint `json:"origin,omitempty"`
	Scores           Scores             `json:"scores"`
	WindowScores     *WindowScore       `json:"windowScores,omitempty"`
	Receipts         []Receipt          `json:"receipts,omitempty"`
	WindowReceipts   []Receipt          `json:"windowReceipts,omitempty"`
	Timeline         []TimelineStep     `json:"timeline,omitempty"`
	WindowTimeline   []TimelineStep     `json:"windowTimeline,omitempty"`
	EvidenceGallery  []EvidencePreview  `json:"evidenceGallery,omitempty"`
	MatchExplanation []MatchExplanation `json:"matchExplanation,omitempty"`
	OperatorActions  []OperatorAction   `json:"operatorActions,omitempty"`
	Tracked          bool               `json:"tracked"`
}

type OperatorAction struct {
	ID         int64           `json:"id"`
	ClusterID  int64           `json:"clusterId"`
	Action     string          `json:"action"`
	Operator   string          `json:"operator"`
	Note       string          `json:"note,omitempty"`
	BeforeData json.RawMessage `json:"beforeData,omitempty"`
	AfterData  json.RawMessage `json:"afterData,omitempty"`
	CreatedAt  time.Time       `json:"createdAt"`
}

type WatchEntry struct {
	ID        int64     `json:"id"`
	UserRef   string    `json:"userRef,omitempty"`
	Kind      string    `json:"kind"`
	Value     string    `json:"value"`
	Label     string    `json:"label,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

type Receipt struct {
	SourceType    string `json:"sourceType"`
	Risk          string `json:"risk"`
	Pct           int    `json:"pct"`
	URL           string `json:"url,omitempty"`
	ThumbnailURL  string `json:"thumbnailUrl,omitempty"`
	PreviewStatus string `json:"previewStatus,omitempty"`
	PreviewID     int64  `json:"previewId,omitempty"`
}

type TimelineStep struct {
	At         time.Time `json:"at"`
	SourceType string    `json:"sourceType"`
	Label      string    `json:"label"`
	SourceURL  string    `json:"sourceUrl,omitempty"`
}

type EvidencePreview struct {
	ID            int64      `json:"id,omitempty"`
	CanonicalURL  string     `json:"canonicalUrl"`
	Platform      string     `json:"platform"`
	ProviderName  string     `json:"providerName,omitempty"`
	Title         string     `json:"title,omitempty"`
	Author        string     `json:"author,omitempty"`
	ThumbnailURL  string     `json:"thumbnailUrl,omitempty"`
	EmbedURL      string     `json:"embedUrl,omitempty"`
	EmbedHTML     string     `json:"embedHtml,omitempty"`
	CreatedAtSrc  *time.Time `json:"createdAtSrc,omitempty"`
	FetchedAt     time.Time  `json:"fetchedAt,omitempty"`
	HTTPStatus    int        `json:"httpStatus,omitempty"`
	Error         string     `json:"error,omitempty"`
	RetryCount    int        `json:"retryCount,omitempty"`
	NextFetchAt   *time.Time `json:"nextFetchAt,omitempty"`
	ExpiresAt     time.Time  `json:"expiresAt,omitempty"`
	PreviewStatus string     `json:"previewStatus"`
	MatchKind     string     `json:"matchKind,omitempty"`
	Note          string     `json:"note,omitempty"`
}

type MatchExplanation struct {
	SourceType    string   `json:"sourceType"`
	MatchedBy     string   `json:"matchedBy"`
	Confidence    float64  `json:"confidence"`
	Author        string   `json:"author,omitempty"`
	Factors       []string `json:"factors,omitempty"`
	PreviewStatus string   `json:"previewStatus,omitempty"`
	SourceURL     string   `json:"sourceUrl,omitempty"`
	PreviewID     int64    `json:"previewId,omitempty"`
	EvidenceID    int64    `json:"evidenceId,omitempty"`
}

type TrendingStreamer struct {
	Login           string    `json:"login"`
	DisplayName     string    `json:"displayName"`
	StoryCount      int       `json:"storyCount"`
	EvidenceCount   int       `json:"evidenceCount"`
	SourceDiversity int       `json:"sourceDiversity"`
	Volatility      *float64  `json:"volatility"`
	LastSeen        time.Time `json:"lastSeen"`
}

func (s *Store) UpsertEntity(ctx context.Context, login, twitchID, display string, aliases json.RawMessage) (int64, error) {
	if aliases == nil {
		aliases = json.RawMessage("[]")
	}
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO streamer_entities (twitch_login, twitch_id, display_name, aliases, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (twitch_login) DO UPDATE SET
			twitch_id = COALESCE(EXCLUDED.twitch_id, streamer_entities.twitch_id),
			display_name = COALESCE(NULLIF(EXCLUDED.display_name, ''), streamer_entities.display_name),
			aliases = EXCLUDED.aliases,
			updated_at = now()
		RETURNING id`, login, twitchID, display, aliases).Scan(&id)
	return id, err
}

func (s *Store) EntityByLogin(ctx context.Context, login string) (*Entity, error) {
	var e Entity
	err := s.pool.QueryRow(ctx, `
		SELECT id, twitch_login, twitch_id, display_name, aliases
		FROM streamer_entities WHERE LOWER(twitch_login) = LOWER($1)`, login).Scan(
		&e.ID, &e.TwitchLogin, &e.TwitchID, &e.DisplayName, &e.Aliases)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (s *Store) InsertFingerprint(ctx context.Context, fp MomentFingerprint) (int64, error) {
	if fp.TranscriptKW == nil {
		fp.TranscriptKW = json.RawMessage("[]")
	}
	if fp.TopEmotes == nil {
		fp.TopEmotes = json.RawMessage("[]")
	}
	if fp.FPVersion == 0 {
		fp.FPVersion = 1
	}
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO moment_fingerprints (entity_id, stream_id, vod_id, vod_offset_s, transcript_kw, top_emotes, chat_spike_summary, origin_confidence, game, fp_version)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (stream_id, vod_offset_s, fp_version) DO UPDATE SET
			vod_id = COALESCE(NULLIF(EXCLUDED.vod_id, ''), moment_fingerprints.vod_id),
			transcript_kw = EXCLUDED.transcript_kw,
			top_emotes = EXCLUDED.top_emotes,
			chat_spike_summary = EXCLUDED.chat_spike_summary,
			origin_confidence = EXCLUDED.origin_confidence,
			game = EXCLUDED.game
		RETURNING id`,
		fp.EntityID, fp.StreamID, fp.VODID, fp.VODOffsetS, fp.TranscriptKW, fp.TopEmotes, fp.ChatSpikeSummary, fp.OriginConfidence, fp.Game, fp.FPVersion).Scan(&id)
	return id, err
}

func (s *Store) ListOriginCandidateStreams(ctx context.Context, logins []string, since time.Time, limit int) ([]OriginCandidateStream, error) {
	if limit <= 0 {
		limit = 25
	}
	normalized := make([]string, 0, len(logins))
	seen := map[string]struct{}{}
	for _, login := range logins {
		login = strings.ToLower(strings.TrimSpace(login))
		if login == "" {
			continue
		}
		if _, ok := seen[login]; ok {
			continue
		}
		seen[login] = struct{}{}
		normalized = append(normalized, login)
	}
	rows, err := s.pool.Query(ctx, `
		SELECT stream_id, COALESCE(vod_id, ''), LOWER(login), started_at, last_seen_at
		FROM analytics_streams
		WHERE COALESCE(login, '') <> ''
		  AND last_seen_at >= $1
		  AND (
		    CARDINALITY($2::text[]) = 0
		    OR LOWER(login) = ANY($2::text[])
		  )
		ORDER BY last_seen_at DESC
		LIMIT $3`, since, normalized, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]OriginCandidateStream, 0, limit)
	for rows.Next() {
		var row OriginCandidateStream
		if err := rows.Scan(&row.StreamID, &row.VODID, &row.Login, &row.StartedAt, &row.LastSeenAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) ListFingerprintsForStream(ctx context.Context, streamID string, limit int) ([]MomentFingerprint, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, entity_id, stream_id, COALESCE(vod_id, ''), vod_offset_s, transcript_kw, top_emotes,
		       COALESCE(chat_spike_summary, ''), origin_confidence, game, fp_version, created_at
		FROM moment_fingerprints
		WHERE stream_id = $1
		ORDER BY vod_offset_s ASC
		LIMIT $2`, streamID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]MomentFingerprint, 0, limit)
	for rows.Next() {
		var fp MomentFingerprint
		if err := rows.Scan(&fp.ID, &fp.EntityID, &fp.StreamID, &fp.VODID, &fp.VODOffsetS, &fp.TranscriptKW, &fp.TopEmotes, &fp.ChatSpikeSummary, &fp.OriginConfidence, &fp.Game, &fp.FPVersion, &fp.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, fp)
	}
	return out, rows.Err()
}

type OriginClusterCandidate struct {
	ID        int64
	Title     string
	UpdatedAt time.Time
}

func (s *Store) ListOriginClusterCandidates(ctx context.Context, entityID int64, from, to time.Time, limit int) ([]OriginClusterCandidate, error) {
	if limit <= 0 {
		limit = 25
	}
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT c.id, COALESCE(c.title, ''), c.updated_at
		FROM story_clusters c
		JOIN story_evidence ev ON ev.cluster_id = c.id
		WHERE c.entity_id = $1
		  AND c.moment_fp_id IS NULL
		  AND c.state IN ('published', 'developing', 'unverified')
		  AND COALESCE(ev.occurred_at, ev.created_at) BETWEEN $2 AND $3
		ORDER BY c.updated_at DESC
		LIMIT $4`, entityID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]OriginClusterCandidate, 0, limit)
	for rows.Next() {
		var row OriginClusterCandidate
		if err := rows.Scan(&row.ID, &row.Title, &row.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) AttachOriginToCluster(ctx context.Context, clusterID, momentFPID int64, confidence float64, occurredAt time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		UPDATE story_clusters
		SET moment_fp_id = COALESCE(moment_fp_id, $2),
		    origin_search_status = 'matched',
		    origin_checked_at = now(),
		    updated_at = now()
		WHERE id = $1`, clusterID, momentFPID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO story_evidence (cluster_id, moment_fp_id, source_type, match_conf, weight, occurred_at)
		SELECT $1, $2, 'pulse_origin', $3, 1.0, $4
		WHERE NOT EXISTS (
			SELECT 1 FROM story_evidence
			WHERE cluster_id = $1 AND moment_fp_id = $2 AND source_type = 'pulse_origin'
		)`, clusterID, momentFPID, confidence, occurredAt); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) MarkOriginSearched(ctx context.Context, clusterID int64, status string) error {
	status = strings.TrimSpace(status)
	if clusterID <= 0 || status == "" {
		return nil
	}
	if status != "searched_missing" && status != "searched_no_fingerprints" {
		return fmt.Errorf("unknown origin search status %q", status)
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE story_clusters
		SET origin_search_status = CASE WHEN moment_fp_id IS NULL THEN $2 ELSE 'matched' END,
		    origin_checked_at = now(),
		    updated_at = now()
		WHERE id = $1`, clusterID, status)
	return err
}

func (s *Store) UpsertSocialItem(ctx context.Context, item SocialItem, provenance json.RawMessage, snapshot []byte) (int64, error) {
	if item.Metrics == nil {
		item.Metrics = json.RawMessage("{}")
	}
	if provenance == nil {
		provenance = json.RawMessage("{}")
	}
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO social_items (source, kind, external_id, url, author, created_at_src, text, metrics, entity_id, provenance, snapshot_sha256, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (source, external_id) DO UPDATE SET
			text = CASE
				WHEN COALESCE(EXCLUDED.text, '') ~ '^[0-9]+\\s+comments?$'
				     AND COALESCE(social_items.text, '') <> ''
				     AND COALESCE(social_items.text, '') !~ '^[0-9]+\\s+comments?$'
				THEN social_items.text
				ELSE EXCLUDED.text
			END,
			metrics = (
				COALESCE(social_items.metrics, '{}'::jsonb) || COALESCE(EXCLUDED.metrics, '{}'::jsonb)
			) || COALESCE(jsonb_strip_nulls(jsonb_build_object(
				'thumbnail_url', CASE
					WHEN COALESCE(EXCLUDED.metrics->>'thumbnail_url', '') = ''
					     AND COALESCE(social_items.metrics->>'thumbnail_url', '') <> ''
					THEN social_items.metrics->'thumbnail_url'
				END,
				'thumbnail_source', CASE
					WHEN COALESCE(EXCLUDED.metrics->>'thumbnail_source', '') = ''
					     AND COALESCE(social_items.metrics->>'thumbnail_source', '') <> ''
					THEN social_items.metrics->'thumbnail_source'
				END,
				'thumbnail_status', CASE
					WHEN COALESCE(EXCLUDED.metrics->>'thumbnail_status', '') = ''
					     AND COALESCE(social_items.metrics->>'thumbnail_status', '') <> ''
					THEN social_items.metrics->'thumbnail_status'
				END
			)), '{}'::jsonb),
			entity_id = COALESCE(EXCLUDED.entity_id, social_items.entity_id),
			expires_at = EXCLUDED.expires_at
		RETURNING id`,
		item.Source, item.Kind, item.ExternalID, item.URL, item.Author, item.CreatedAtSrc,
		item.Text, item.Metrics, item.EntityID, provenance, snapshot, item.ExpiresAt).Scan(&id)
	return id, err
}

func (s *Store) ClusterByItemID(ctx context.Context, itemID int64) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		SELECT cluster_id
		FROM story_evidence
		WHERE item_id = $1
		ORDER BY id DESC
		LIMIT 1`, itemID).Scan(&id)
	if err == pgx.ErrNoRows {
		return 0, nil
	}
	return id, err
}

func (s *Store) FindRecentClusterForTitle(ctx context.Context, entityID *int64, title string) (int64, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return 0, pgx.ErrNoRows
	}
	var id int64
	if entityID != nil {
		err := s.pool.QueryRow(ctx, `
			SELECT id
			FROM story_clusters
			WHERE entity_id = $1
			  AND LOWER(COALESCE(title, '')) = LOWER($2)
			  AND state IN ('published', 'developing', 'unverified', 'settled')
			  AND updated_at >= now() - interval '7 days'
			ORDER BY updated_at DESC
			LIMIT 1`, *entityID, title).Scan(&id)
		return id, err
	}
	err := s.pool.QueryRow(ctx, `
		SELECT id
		FROM story_clusters
		WHERE entity_id IS NULL
		  AND LOWER(COALESCE(title, '')) = LOWER($1)
		  AND state IN ('published', 'developing', 'unverified', 'settled')
		  AND updated_at >= now() - interval '7 days'
		ORDER BY updated_at DESC
		LIMIT 1`, title).Scan(&id)
	return id, err
}

// FindClusterByCanonicalURL returns a recent cluster linked to a canonical evidence URL.
func (s *Store) FindClusterByCanonicalURL(ctx context.Context, canonicalURL string, since time.Time) (int64, error) {
	canonicalURL = strings.TrimSpace(canonicalURL)
	if canonicalURL == "" {
		return 0, pgx.ErrNoRows
	}
	var id int64
	err := s.pool.QueryRow(ctx, `
		SELECT c.id
		FROM story_clusters c
		WHERE c.state IN ('published', 'developing', 'unverified', 'settled')
		  AND c.updated_at >= $2
		  AND (
		    EXISTS (
		      SELECT 1
		      FROM story_evidence_previews sep
		      JOIN evidence_previews p ON p.id = sep.preview_id
		      WHERE sep.cluster_id = c.id AND p.canonical_url = $1
		    )
		    OR EXISTS (
		      SELECT 1 FROM story_evidence ev
		      WHERE ev.cluster_id = c.id AND ev.source_url = $1
		    )
		  )
		ORDER BY c.updated_at DESC
		LIMIT 1`, canonicalURL, since).Scan(&id)
	if err == pgx.ErrNoRows {
		return 0, nil
	}
	return id, err
}

// ListRecentClusterIDsForEntity returns recent cluster ids for title-similarity fusion.
func (s *Store) ListRecentClusterIDsForEntity(ctx context.Context, entityID int64, since time.Time, limit int) ([]int64, error) {
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id
		FROM story_clusters
		WHERE entity_id = $1
		  AND state IN ('published', 'developing', 'unverified', 'settled')
		  AND updated_at >= $2
		ORDER BY updated_at DESC
		LIMIT $3`, entityID, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ListEvidenceHeadlinesForCluster returns distinct social/preview headlines attached to a cluster.
func (s *Store) ListEvidenceHeadlinesForCluster(ctx context.Context, clusterID int64, limit int) ([]string, error) {
	if limit <= 0 || limit > 20 {
		limit = 8
	}
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT headline FROM (
			SELECT LEFT(TRIM(COALESCE(si.text, '')), 500) AS headline
			FROM story_evidence ev
			JOIN social_items si ON si.id = ev.item_id
			WHERE ev.cluster_id = $1 AND TRIM(COALESCE(si.text, '')) <> ''
			UNION
			SELECT TRIM(COALESCE(p.title, '')) AS headline
			FROM story_evidence_previews sep
			JOIN evidence_previews p ON p.id = sep.preview_id
			WHERE sep.cluster_id = $1 AND TRIM(COALESCE(p.title, '')) <> ''
		) headlines
		WHERE headline <> ''
		LIMIT $2`, clusterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var headline string
		if err := rows.Scan(&headline); err != nil {
			return nil, err
		}
		out = append(out, headline)
	}
	return out, rows.Err()
}

// ListClusterEvidenceSources returns distinct social/evidence source names linked to a cluster.
func (s *Store) ListClusterEvidenceSources(ctx context.Context, clusterID int64) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT source_name FROM (
			SELECT TRIM(COALESCE(si.source, '')) AS source_name
			FROM story_evidence ev
			JOIN social_items si ON si.id = ev.item_id
			WHERE ev.cluster_id = $1
			UNION
			SELECT TRIM(COALESCE(ev.source_type, '')) AS source_name
			FROM story_evidence ev
			WHERE ev.cluster_id = $1
		) sources
		WHERE source_name <> ''`, clusterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var source string
		if err := rows.Scan(&source); err != nil {
			return nil, err
		}
		out = append(out, source)
	}
	return out, rows.Err()
}

func (s *Store) ClusterMeta(ctx context.Context, id int64) (title, category string, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT COALESCE(title, ''), COALESCE(category, '')
		FROM story_clusters WHERE id = $1`, id).Scan(&title, &category)
	return title, category, err
}

func (s *Store) UpdateClusterMeta(ctx context.Context, id int64, entityID *int64, title, summary, category, state string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE story_clusters
		SET entity_id = COALESCE(story_clusters.entity_id, $2),
		    title = CASE WHEN $3 <> '' THEN $3 ELSE story_clusters.title END,
		    summary = CASE WHEN $4 <> '' THEN $4 ELSE story_clusters.summary END,
		    category = CASE WHEN $5 <> '' THEN $5 ELSE story_clusters.category END,
		    state = CASE
		    	WHEN story_clusters.state IN ('settled', 'suppressed') THEN story_clusters.state
		    	WHEN $6 = 'published' AND story_clusters.state IN ('developing', 'unverified') THEN 'published'
		    	WHEN $6 = 'unverified' AND story_clusters.state = 'developing' THEN 'unverified'
		    	WHEN $6 <> '' THEN $6
		    	ELSE story_clusters.state
		    END,
		    updated_at = now()
		WHERE id = $1`, id, entityID, title, summary, category, state)
	return err
}

func (s *Store) ClusterByMomentFP(ctx context.Context, momentFPID int64) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `SELECT id FROM story_clusters WHERE moment_fp_id = $1 LIMIT 1`, momentFPID).Scan(&id)
	return id, err
}

func (s *Store) InsertCluster(ctx context.Context, c StoryCluster) (int64, error) {
	if c.State == "" {
		c.State = "developing"
	}
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO story_clusters (entity_id, moment_fp_id, title, summary, category, story_class, state)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7)
		RETURNING id`,
		c.EntityID, c.MomentFPID, c.Title, c.Summary, c.Category, c.StoryClass, c.State).Scan(&id)
	return id, err
}

func (s *Store) UpdateClusterState(ctx context.Context, id int64, state string) error {
	_, err := s.pool.Exec(ctx, `UPDATE story_clusters SET state = $2, updated_at = now() WHERE id = $1`, id, state)
	return err
}

func (s *Store) InsertEvidence(ctx context.Context, ev Evidence) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO story_evidence (cluster_id, item_id, moment_fp_id, source_type, source_url, match_conf, weight, occurred_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id`,
		ev.ClusterID, ev.ItemID, ev.MomentFPID, ev.SourceType, ev.SourceURL, ev.MatchConf, ev.Weight, ev.OccurredAt).Scan(&id)
	return id, err
}

func (s *Store) GetEvidencePreviewByCanonical(ctx context.Context, canonicalURL string) (*EvidencePreview, error) {
	var p EvidencePreview
	var httpStatus *int
	err := s.pool.QueryRow(ctx, `
		SELECT id, canonical_url, platform, COALESCE(provider_name, ''),
		       COALESCE(title, ''), COALESCE(author, ''), COALESCE(thumbnail_url, ''),
		       COALESCE(embed_url, ''), COALESCE(embed_html, ''), created_at_src,
		       fetched_at, http_status, COALESCE(error, ''), retry_count, next_fetch_at, expires_at
		FROM evidence_previews
		WHERE canonical_url = $1`, canonicalURL).Scan(
		&p.ID, &p.CanonicalURL, &p.Platform, &p.ProviderName, &p.Title, &p.Author,
		&p.ThumbnailURL, &p.EmbedURL, &p.EmbedHTML, &p.CreatedAtSrc, &p.FetchedAt,
		&httpStatus, &p.Error, &p.RetryCount, &p.NextFetchAt, &p.ExpiresAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if httpStatus != nil {
		p.HTTPStatus = *httpStatus
	}
	p.PreviewStatus = computePreviewStatus(p)
	return &p, nil
}

func (s *Store) FindPreviewLinkByCanonical(ctx context.Context, clusterID int64, canonicalURL string) (*EvidencePreview, bool, error) {
	var p EvidencePreview
	var httpStatus *int
	err := s.pool.QueryRow(ctx, `
		SELECT p.id, p.canonical_url, p.platform, COALESCE(p.provider_name, ''),
		       COALESCE(p.title, ''), COALESCE(p.author, ''), COALESCE(p.thumbnail_url, ''),
		       COALESCE(p.embed_url, ''), COALESCE(p.embed_html, ''), p.created_at_src,
		       p.fetched_at, p.http_status, COALESCE(p.error, ''), p.retry_count, p.next_fetch_at, p.expires_at,
		       sep.match_kind, COALESCE(sep.note, '')
		FROM evidence_previews p
		JOIN story_evidence_previews sep ON sep.preview_id = p.id
		WHERE sep.cluster_id = $1 AND p.canonical_url = $2
		LIMIT 1`, clusterID, canonicalURL).Scan(
		&p.ID, &p.CanonicalURL, &p.Platform, &p.ProviderName, &p.Title, &p.Author,
		&p.ThumbnailURL, &p.EmbedURL, &p.EmbedHTML, &p.CreatedAtSrc, &p.FetchedAt,
		&httpStatus, &p.Error, &p.RetryCount, &p.NextFetchAt, &p.ExpiresAt, &p.MatchKind, &p.Note)
	if err == pgx.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if httpStatus != nil {
		p.HTTPStatus = *httpStatus
	}
	p.PreviewStatus = computePreviewStatus(p)
	return &p, true, nil
}

func (s *Store) FindManualEvidenceByURL(ctx context.Context, clusterID int64, canonicalURL string) (*Evidence, error) {
	var ev Evidence
	err := s.pool.QueryRow(ctx, `
		SELECT id, cluster_id, item_id, moment_fp_id, source_type, source_url, match_conf, weight, occurred_at
		FROM story_evidence
		WHERE cluster_id = $1 AND source_url = $2
		ORDER BY id DESC
		LIMIT 1`, clusterID, canonicalURL).Scan(
		&ev.ID, &ev.ClusterID, &ev.ItemID, &ev.MomentFPID, &ev.SourceType,
		&ev.SourceURL, &ev.MatchConf, &ev.Weight, &ev.OccurredAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ev, nil
}

func computePreviewStatus(p EvidencePreview) string {
	if p.Error != "" {
		return "error"
	}
	if p.EmbedURL != "" || p.EmbedHTML != "" || p.ThumbnailURL != "" || p.Title != "" {
		return "ready"
	}
	return "fallback"
}

func (s *Store) UpsertEvidencePreview(ctx context.Context, preview EvidencePreview) (int64, error) {
	if preview.ExpiresAt.IsZero() {
		preview.ExpiresAt = time.Now().Add(7 * 24 * time.Hour)
	}
	if preview.FetchedAt.IsZero() {
		preview.FetchedAt = time.Now()
	}
	if preview.NextFetchAt == nil {
		next := preview.ExpiresAt
		if preview.Error != "" {
			next = preview.FetchedAt.Add(defaultPreviewRetryDelay)
		}
		preview.NextFetchAt = &next
	}
	if preview.Error != "" && preview.RetryCount == 0 {
		preview.RetryCount = 1
	}
	if preview.Error == "" {
		preview.RetryCount = 0
	}
	var httpStatus any
	if preview.HTTPStatus > 0 {
		httpStatus = preview.HTTPStatus
	}
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO evidence_previews (
			canonical_url, platform, provider_name, title, author, thumbnail_url,
			embed_url, embed_html, created_at_src, fetched_at, http_status, error,
			retry_count, next_fetch_at, expires_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		ON CONFLICT (canonical_url) DO UPDATE SET
			platform = EXCLUDED.platform,
			provider_name = EXCLUDED.provider_name,
			title = COALESCE(NULLIF(EXCLUDED.title, ''), evidence_previews.title),
			author = COALESCE(NULLIF(EXCLUDED.author, ''), evidence_previews.author),
			thumbnail_url = COALESCE(NULLIF(EXCLUDED.thumbnail_url, ''), evidence_previews.thumbnail_url),
			embed_url = COALESCE(NULLIF(EXCLUDED.embed_url, ''), evidence_previews.embed_url),
			embed_html = CASE
				WHEN EXCLUDED.embed_html <> '' THEN EXCLUDED.embed_html
				ELSE evidence_previews.embed_html
			END,
			created_at_src = COALESCE(EXCLUDED.created_at_src, evidence_previews.created_at_src),
			fetched_at = EXCLUDED.fetched_at,
			http_status = EXCLUDED.http_status,
			error = EXCLUDED.error,
			retry_count = CASE
				WHEN COALESCE(EXCLUDED.error, '') <> '' THEN LEAST(evidence_previews.retry_count + 1, 6)
				ELSE 0
			END,
			next_fetch_at = CASE
				WHEN COALESCE(EXCLUDED.error, '') <> '' THEN EXCLUDED.fetched_at + CASE LEAST(evidence_previews.retry_count + 1, 6)
					WHEN 1 THEN interval '1 hour'
					WHEN 2 THEN interval '2 hours'
					WHEN 3 THEN interval '4 hours'
					WHEN 4 THEN interval '8 hours'
					ELSE interval '24 hours'
				END
				ELSE EXCLUDED.next_fetch_at
			END,
			expires_at = EXCLUDED.expires_at
		RETURNING id`,
		preview.CanonicalURL, preview.Platform, preview.ProviderName, preview.Title, preview.Author,
		preview.ThumbnailURL, preview.EmbedURL, preview.EmbedHTML, preview.CreatedAtSrc,
		preview.FetchedAt, httpStatus, preview.Error, preview.RetryCount, preview.NextFetchAt, preview.ExpiresAt).Scan(&id)
	return id, err
}

func (s *Store) LinkEvidencePreview(ctx context.Context, clusterID int64, evidenceID *int64, previewID int64, matchKind, note string) error {
	if matchKind == "" {
		matchKind = "url"
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO story_evidence_previews (cluster_id, evidence_id, preview_id, match_kind, note)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (cluster_id, preview_id) DO UPDATE SET
			evidence_id = COALESCE(EXCLUDED.evidence_id, story_evidence_previews.evidence_id),
			match_kind = EXCLUDED.match_kind,
			note = COALESCE(NULLIF(EXCLUDED.note, ''), story_evidence_previews.note)`,
		clusterID, evidenceID, previewID, matchKind, note)
	return err
}

func (s *Store) ListEvidencePreviewsForCluster(ctx context.Context, clusterID int64) ([]EvidencePreview, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT p.id, p.canonical_url, p.platform, COALESCE(p.provider_name, ''),
		       COALESCE(p.title, ''), COALESCE(p.author, ''), COALESCE(p.thumbnail_url, ''),
		       COALESCE(p.embed_url, ''), COALESCE(p.embed_html, ''), p.created_at_src,
		       p.fetched_at, COALESCE(p.http_status, 0), COALESCE(p.error, ''),
		       p.retry_count, p.next_fetch_at, p.expires_at, sep.match_kind, COALESCE(sep.note, ''),
		       CASE
		         WHEN COALESCE(p.error, '') <> '' THEN 'error'
		         WHEN COALESCE(p.embed_url, '') <> '' OR COALESCE(p.embed_html, '') <> '' OR COALESCE(p.thumbnail_url, '') <> '' OR COALESCE(p.title, '') <> '' THEN 'ready'
		         ELSE 'fallback'
		       END AS preview_status
		FROM story_evidence_previews sep
		JOIN evidence_previews p ON p.id = sep.preview_id
		WHERE sep.cluster_id = $1
		ORDER BY
			CASE WHEN sep.match_kind = 'manual' THEN 0 ELSE 1 END,
			sep.created_at DESC,
			p.fetched_at DESC`, clusterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EvidencePreview
	for rows.Next() {
		var p EvidencePreview
		if err := rows.Scan(&p.ID, &p.CanonicalURL, &p.Platform, &p.ProviderName, &p.Title, &p.Author,
			&p.ThumbnailURL, &p.EmbedURL, &p.EmbedHTML, &p.CreatedAtSrc, &p.FetchedAt,
			&p.HTTPStatus, &p.Error, &p.RetryCount, &p.NextFetchAt, &p.ExpiresAt, &p.MatchKind, &p.Note, &p.PreviewStatus); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return applyEvidencePreviewDisplayThumbs(filterAttachableEvidencePreviews(out)), rows.Err()
}

// ListEvidencePreviewsForClusters loads up to perCluster attachable previews for many clusters in one query.
func (s *Store) ListEvidencePreviewsForClusters(ctx context.Context, clusterIDs []int64, perCluster int) (map[int64][]EvidencePreview, error) {
	if len(clusterIDs) == 0 {
		return map[int64][]EvidencePreview{}, nil
	}
	if perCluster <= 0 || perCluster > 8 {
		perCluster = 4
	}
	rows, err := s.pool.Query(ctx, `
		SELECT cluster_id, id, canonical_url, platform, COALESCE(provider_name, ''),
		       COALESCE(title, ''), COALESCE(author, ''), COALESCE(thumbnail_url, ''),
		       COALESCE(embed_url, ''), COALESCE(embed_html, ''), created_at_src,
		       fetched_at, COALESCE(http_status, 0), COALESCE(error, ''),
		       retry_count, next_fetch_at, expires_at, match_kind, COALESCE(note, ''),
		       preview_status
		FROM (
			SELECT sep.cluster_id,
			       p.id, p.canonical_url, p.platform, p.provider_name,
			       p.title, p.author, p.thumbnail_url,
			       p.embed_url, p.embed_html, p.created_at_src,
			       p.fetched_at, p.http_status, p.error,
			       p.retry_count, p.next_fetch_at, p.expires_at,
			       sep.match_kind, sep.note,
			       CASE
			         WHEN COALESCE(p.error, '') <> '' THEN 'error'
			         WHEN COALESCE(p.embed_url, '') <> '' OR COALESCE(p.embed_html, '') <> '' OR COALESCE(p.thumbnail_url, '') <> '' OR COALESCE(p.title, '') <> '' THEN 'ready'
			         ELSE 'fallback'
			       END AS preview_status,
			       ROW_NUMBER() OVER (
			         PARTITION BY sep.cluster_id
			         ORDER BY
			           CASE WHEN sep.match_kind = 'manual' THEN 0 ELSE 1 END,
			           sep.created_at DESC,
			           p.fetched_at DESC
			       ) AS rn
			FROM story_evidence_previews sep
			JOIN evidence_previews p ON p.id = sep.preview_id
			WHERE sep.cluster_id = ANY($1)
		) ranked
		WHERE rn <= $2
		ORDER BY cluster_id, rn`, clusterIDs, perCluster)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int64][]EvidencePreview, len(clusterIDs))
	for rows.Next() {
		var clusterID int64
		var p EvidencePreview
		if err := rows.Scan(&clusterID, &p.ID, &p.CanonicalURL, &p.Platform, &p.ProviderName, &p.Title, &p.Author,
			&p.ThumbnailURL, &p.EmbedURL, &p.EmbedHTML, &p.CreatedAtSrc, &p.FetchedAt,
			&p.HTTPStatus, &p.Error, &p.RetryCount, &p.NextFetchAt, &p.ExpiresAt, &p.MatchKind, &p.Note, &p.PreviewStatus); err != nil {
			return nil, err
		}
		link := evidenceurl.Link{CanonicalURL: p.CanonicalURL, Platform: p.Platform}
		if !evidenceurl.Attachable(link) {
			continue
		}
		out[clusterID] = append(out[clusterID], p)
	}
	for id, previews := range out {
		out[id] = applyEvidencePreviewDisplayThumbs(previews)
	}
	return out, rows.Err()
}

func applyEvidencePreviewDisplayThumbs(previews []EvidencePreview) []EvidencePreview {
	if len(previews) == 0 {
		return previews
	}
	for i := range previews {
		if clip := twitchClipPreviewURL(previews[i].CanonicalURL); clip != "" && strings.TrimSpace(previews[i].ThumbnailURL) == "" {
			previews[i].ThumbnailURL = displayThumbnailURL(clip, "")
			continue
		}
		previews[i].ThumbnailURL = displayThumbnailURL(previews[i].ThumbnailURL, "")
	}
	return previews
}

func filterAttachableEvidencePreviews(previews []EvidencePreview) []EvidencePreview {
	if len(previews) == 0 {
		return previews
	}
	filtered := make([]EvidencePreview, 0, len(previews))
	for _, p := range previews {
		link := evidenceurl.Link{CanonicalURL: p.CanonicalURL, Platform: p.Platform}
		if evidenceurl.Attachable(link) {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

func (s *Store) ListEvidencePreviewsDueForRefresh(ctx context.Context, limit int) ([]EvidencePreview, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, canonical_url, platform, COALESCE(provider_name, ''),
		       COALESCE(title, ''), COALESCE(author, ''), COALESCE(thumbnail_url, ''),
		       COALESCE(embed_url, ''), COALESCE(embed_html, ''), created_at_src,
		       fetched_at, COALESCE(http_status, 0), COALESCE(error, ''),
		       retry_count, next_fetch_at, expires_at,
		       CASE
		         WHEN COALESCE(error, '') <> '' THEN 'error'
		         WHEN COALESCE(embed_url, '') <> '' OR COALESCE(embed_html, '') <> '' OR COALESCE(thumbnail_url, '') <> '' OR COALESCE(title, '') <> '' THEN 'ready'
		         ELSE 'fallback'
		       END AS preview_status
		FROM evidence_previews
		WHERE next_fetch_at <= now()
		ORDER BY next_fetch_at ASC, fetched_at ASC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EvidencePreview
	for rows.Next() {
		var p EvidencePreview
		if err := rows.Scan(&p.ID, &p.CanonicalURL, &p.Platform, &p.ProviderName, &p.Title, &p.Author,
			&p.ThumbnailURL, &p.EmbedURL, &p.EmbedHTML, &p.CreatedAtSrc, &p.FetchedAt,
			&p.HTTPStatus, &p.Error, &p.RetryCount, &p.NextFetchAt, &p.ExpiresAt, &p.PreviewStatus); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) MatchExplanationForCluster(ctx context.Context, clusterID int64) ([]MatchExplanation, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT ev.source_type,
		       COALESCE(NULLIF(sep.match_kind, ''), CASE WHEN COALESCE(ev.source_url, '') <> '' THEN 'url' ELSE 'title' END),
		       COALESCE(ev.match_conf, 0),
		       CASE
		         WHEN p.id IS NULL THEN ''
		         WHEN COALESCE(p.error, '') <> '' THEN 'error'
		         WHEN COALESCE(p.embed_url, '') <> '' OR COALESCE(p.embed_html, '') <> '' OR COALESCE(p.thumbnail_url, '') <> '' OR COALESCE(p.title, '') <> '' THEN 'ready'
		         ELSE 'fallback'
		       END,
		       COALESCE(ev.source_url, ''),
		       COALESCE(p.id, 0),
		       ev.id,
		       COALESCE(NULLIF(si.author, ''), NULLIF(p.author, ''), '')
		FROM story_evidence ev
		LEFT JOIN social_items si ON si.id = ev.item_id
		LEFT JOIN story_evidence_previews sep ON sep.evidence_id = ev.id
		LEFT JOIN evidence_previews p ON p.id = sep.preview_id
		WHERE ev.cluster_id = $1
		ORDER BY COALESCE(ev.occurred_at, ev.created_at) DESC`, clusterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MatchExplanation{}
	for rows.Next() {
		var m MatchExplanation
		if err := rows.Scan(&m.SourceType, &m.MatchedBy, &m.Confidence, &m.PreviewStatus, &m.SourceURL, &m.PreviewID, &m.EvidenceID, &m.Author); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	duplicateAuthors := duplicateAuthorEvidence(out)
	for i := range out {
		out[i].Factors = matchFactors(out[i])
		if duplicateAuthors[out[i].EvidenceID] {
			out[i].Factors = append(out[i].Factors, duplicateAuthorFactor(out[i].Author))
		}
	}
	return out, nil
}

func matchFactors(m MatchExplanation) []string {
	factors := []string{m.SourceType}
	if m.MatchedBy != "" {
		factors = append(factors, "matched_by:"+m.MatchedBy)
	}
	if m.SourceURL != "" {
		factors = append(factors, "source_url")
	}
	if m.PreviewStatus != "" {
		factors = append(factors, "preview:"+m.PreviewStatus)
	}
	return factors
}

func duplicateAuthorEvidence(matches []MatchExplanation) map[int64]bool {
	evidenceByAuthor := map[string]map[int64]struct{}{}
	evidenceKeys := map[int64][]string{}
	for _, m := range matches {
		key := duplicateAuthorKey(m.SourceType, m.Author)
		if key == "" || m.EvidenceID == 0 {
			continue
		}
		if evidenceByAuthor[key] == nil {
			evidenceByAuthor[key] = map[int64]struct{}{}
		}
		evidenceByAuthor[key][m.EvidenceID] = struct{}{}
		evidenceKeys[m.EvidenceID] = append(evidenceKeys[m.EvidenceID], key)
	}
	out := map[int64]bool{}
	for evidenceID, keys := range evidenceKeys {
		for _, key := range keys {
			if len(evidenceByAuthor[key]) > 1 {
				out[evidenceID] = true
				break
			}
		}
	}
	return out
}

func duplicateAuthorKey(sourceType, author string) string {
	normalized := normalizeEvidenceAuthor(author)
	if normalized == "" {
		return ""
	}
	return sourceType + "\x00" + normalized
}

func duplicateAuthorFactor(author string) string {
	normalized := normalizeEvidenceAuthor(author)
	if normalized == "" {
		return "duplicate_author"
	}
	if len(normalized) > 48 {
		normalized = normalized[:48]
	}
	return "duplicate_author:" + normalized
}

func normalizeEvidenceAuthor(author string) string {
	normalized := strings.ToLower(strings.TrimSpace(author))
	normalized = strings.TrimPrefix(normalized, "@")
	normalized = strings.Join(strings.Fields(normalized), " ")
	switch normalized {
	case "", "[deleted]", "deleted", "unknown", "n/a", "na", "automoderator":
		return ""
	default:
		return normalized
	}
}

func (s *Store) UpsertScores(ctx context.Context, clusterID int64, sc Scores) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO story_scores (cluster_id, trend, volatility, confidence, sentiment, factors, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, now())
		ON CONFLICT (cluster_id) DO UPDATE SET
			trend = COALESCE(EXCLUDED.trend, story_scores.trend),
			volatility = COALESCE(EXCLUDED.volatility, story_scores.volatility),
			confidence = COALESCE(EXCLUDED.confidence, story_scores.confidence),
			sentiment = COALESCE(EXCLUDED.sentiment, story_scores.sentiment),
			factors = COALESCE(EXCLUDED.factors, story_scores.factors),
			updated_at = now()`,
		clusterID, sc.Trend, sc.Volatility, sc.Confidence, sc.Sentiment, sc.Factors)
	return err
}

type TrendSnapshot struct {
	At    time.Time
	Trend float64
}

type ClusterSocialItem struct {
	ID         int64
	Source     string
	ExternalID string
	Metrics    json.RawMessage
}

type SocialMetricSnapshot struct {
	ItemID     int64           `json:"itemId"`
	At         time.Time       `json:"at"`
	Source     string          `json:"source"`
	ExternalID string          `json:"externalId"`
	Metrics    json.RawMessage `json:"metrics,omitempty"`
	Comments   *int            `json:"comments,omitempty"`
}

func (s *Store) InsertTrendSnapshot(ctx context.Context, clusterID int64, at time.Time, trend float64) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO trend_snapshots (cluster_id, at, trend, volatility)
		VALUES ($1, $2, $3, NULL)
		ON CONFLICT (cluster_id, at) DO UPDATE SET trend = EXCLUDED.trend`,
		clusterID, at, trend)
	return err
}

func (s *Store) ListTrendSnapshots(ctx context.Context, clusterID int64, limit int) ([]TrendSnapshot, error) {
	if limit <= 0 {
		limit = maxTrendSnapshots
	}
	rows, err := s.pool.Query(ctx, `
		SELECT at, trend
		FROM (
			SELECT at, trend
			FROM trend_snapshots
			WHERE cluster_id = $1
			ORDER BY at DESC
			LIMIT $2
		) recent
		ORDER BY at ASC`, clusterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]TrendSnapshot, 0, limit)
	for rows.Next() {
		var snap TrendSnapshot
		if err := rows.Scan(&snap.At, &snap.Trend); err != nil {
			return nil, err
		}
		out = append(out, snap)
	}
	return out, rows.Err()
}

func (s *Store) EvidenceWeightSince(ctx context.Context, clusterID int64, since time.Time) (float64, error) {
	var total float64
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(weight), 0)
		FROM story_evidence
		WHERE cluster_id = $1
		  AND COALESCE(occurred_at, created_at) >= $2`, clusterID, since).Scan(&total)
	return total, err
}

func (s *Store) ClusterSocialItems(ctx context.Context, clusterID int64) ([]ClusterSocialItem, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT si.id, si.source, si.external_id, si.metrics
		FROM story_evidence ev
		JOIN social_items si ON si.id = ev.item_id
		WHERE ev.cluster_id = $1`, clusterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ClusterSocialItem
	for rows.Next() {
		var item ClusterSocialItem
		if err := rows.Scan(&item.ID, &item.Source, &item.ExternalID, &item.Metrics); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) UpdateSocialItemMetrics(ctx context.Context, itemID int64, metrics json.RawMessage) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE social_items SET metrics = $2 WHERE id = $1`, itemID, metrics)
	return err
}

func (s *Store) InsertSocialMetricSnapshot(ctx context.Context, itemID int64, at time.Time, source, externalID string, metrics json.RawMessage, comments *int) error {
	if metrics == nil {
		metrics = json.RawMessage("{}")
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO social_item_metric_snapshots (item_id, at, source, external_id, metrics, comments)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (item_id, at) DO UPDATE SET
			metrics = EXCLUDED.metrics,
			comments = EXCLUDED.comments`,
		itemID, at.UTC(), source, externalID, metrics, comments)
	return err
}

func (s *Store) ListSocialMetricSnapshots(ctx context.Context, itemID int64, since time.Time, limit int) ([]SocialMetricSnapshot, error) {
	if limit <= 0 {
		limit = 24
	}
	rows, err := s.pool.Query(ctx, `
		SELECT item_id, at, source, external_id, metrics, comments
		FROM (
			SELECT item_id, at, source, external_id, metrics, comments
			FROM social_item_metric_snapshots
			WHERE item_id = $1 AND at >= $2
			ORDER BY at DESC
			LIMIT $3
		) recent
		ORDER BY at ASC`, itemID, since.UTC(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]SocialMetricSnapshot, 0, limit)
	for rows.Next() {
		var snap SocialMetricSnapshot
		if err := rows.Scan(&snap.ItemID, &snap.At, &snap.Source, &snap.ExternalID, &snap.Metrics, &snap.Comments); err != nil {
			return nil, err
		}
		out = append(out, snap)
	}
	return out, rows.Err()
}

func (s *Store) ListSampleableClusterIDs(ctx context.Context) ([]int64, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id
		FROM story_clusters
		WHERE state IN ('published', 'developing', 'unverified')
		  AND updated_at >= now() - interval '7 days'
		ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

const (
	defaultPreviewRetryDelay = time.Hour
	maxTrendSnapshots        = 24
)

func (s *Store) ClusterTitle(ctx context.Context, id int64) (string, error) {
	var title string
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(title, '') FROM story_clusters WHERE id = $1`, id).Scan(&title)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return title, err
}

func (s *Store) ListFeed(ctx context.Context, state, category, login, sort, window string, since time.Time, limit int, afterID int64) ([]StoryCard, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	sort = strings.ToLower(strings.TrimSpace(sort))
	if sort != "volatility" && sort != "updated" {
		sort = "rank"
	}
	window = strings.ToLower(strings.TrimSpace(window))
	if window == "" {
		window = "24h"
	}
	evidenceSince := since
	if evidenceSince.IsZero() {
		evidenceSince = time.Now().UTC().Add(-24 * time.Hour)
	}
	orderBy := `
		COALESCE(ws.rank_score, 0) DESC,
		(CASE WHEN COALESCE(evs.source_count, 0) >= 2 THEN 1 ELSE 0 END) DESC,
		(CASE WHEN COALESCE(evs.has_reddit, false) THEN 1 ELSE 0 END) DESC,
		(CASE WHEN COALESCE(evs.has_streamerbans, false) THEN 1 ELSE 0 END) DESC,
		COALESCE(sc.trend, 0) DESC NULLS LAST,
		c.updated_at DESC,
		c.id DESC`
	if sort == "updated" {
		orderBy = `c.updated_at DESC, c.id DESC`
	}
	if sort == "volatility" {
		orderBy = `
		COALESCE(sc.volatility, 0) DESC NULLS LAST,
		COALESCE(ws.rank_score, 0) DESC,
		c.updated_at DESC,
		c.id DESC`
	}
	q := fmt.Sprintf(`
		SELECT c.id, c.entity_id, c.moment_fp_id, c.title, c.summary, c.category, COALESCE(c.story_class, ''), c.state,
		       COALESCE(c.origin_search_status, ''), c.origin_checked_at, c.created_at, c.updated_at
		FROM story_clusters c
		LEFT JOIN streamer_entities e ON e.id = c.entity_id
		LEFT JOIN story_scores sc ON sc.cluster_id = c.id
		LEFT JOIN story_window_scores ws ON ws.cluster_id = c.id AND ws."window" = $7
		LEFT JOIN LATERAL (
			SELECT COUNT(DISTINCT ev.source_type)::int AS source_count,
			       BOOL_OR(ev.source_type = 'reddit_thread') AS has_reddit,
			       BOOL_OR(ev.source_type = 'streamerbans_post') AS has_streamerbans
			FROM story_evidence ev
			WHERE ev.cluster_id = c.id
			  AND COALESCE(ev.occurred_at, ev.created_at) >= $6
		) evs ON true
		WHERE (($1 = '' AND c.state IN ('published', 'developing', 'unverified', 'settled')) OR c.state = $1)
		  AND ($2 = '' OR c.category = $2)
		  AND ($3 = '' OR LOWER(COALESCE(e.twitch_login, '')) = LOWER($3))
		  AND ($4 = 0 OR c.id < $4)
		  AND COALESCE(c.title, '') <> 'Story developing'
		  AND EXISTS (
		    SELECT 1 FROM story_evidence evw
		    WHERE evw.cluster_id = c.id
		      AND COALESCE(evw.occurred_at, evw.created_at) >= $6
		  )
		ORDER BY %s
		LIMIT $5`, orderBy)
	rows, err := s.pool.Query(ctx, q, state, category, login, afterID, limit, evidenceSince, window)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StoryCard
	for rows.Next() {
		var card StoryCard
		var entityID, momentID *int64
		if err := rows.Scan(&card.Cluster.ID, &entityID, &momentID, &card.Cluster.Title, &card.Cluster.Summary,
			&card.Cluster.Category, &card.Cluster.StoryClass, &card.Cluster.State, &card.Cluster.OriginSearchStatus,
			&card.Cluster.OriginCheckedAt, &card.Cluster.CreatedAt, &card.Cluster.UpdatedAt); err != nil {
			return nil, err
		}
		card.Cluster.EntityID = entityID
		card.Cluster.MomentFPID = momentID
		if entityID != nil {
			card.Entity, _ = s.entityByID(ctx, *entityID)
		}
		if momentID != nil {
			card.Origin, _ = s.fingerprintByID(ctx, *momentID)
		}
		card.Scores, _ = s.scoresForCluster(ctx, card.Cluster.ID)
		if ws, _ := s.WindowScoreForCluster(ctx, card.Cluster.ID, window); ws != nil {
			ws.RecentSourceDelta, _ = s.RecentSourceDelta(ctx, card.Cluster.ID, evidenceSince, time.Now().UTC())
			card.WindowScores = ws
		} else if agg, _ := s.ClusterWindowEvidence(ctx, card.Cluster.ID, evidenceSince); agg != nil {
			now := time.Now().UTC()
			out := windowmath.Compute(windowmath.Input{
				Window:          window,
				Since:           evidenceSince,
				Now:             now,
				EvidenceCount:   agg.EvidenceCount,
				SourceCount:     agg.SourceCount,
				WeightedSum:     agg.WeightedSum,
				LatestAt:        agg.LatestAt,
				DominantSource:  agg.DominantSource,
				Category:        agg.Category,
				Trend:           agg.Trend,
				HasReddit:       agg.HasReddit,
				HasStreamerBans: agg.HasStreamerBans,
				OnlyTwitch:      agg.OnlyTwitch,
			})
			card.WindowScores = &WindowScore{
				Window:        window,
				Since:         evidenceSince,
				EvidenceCount: agg.EvidenceCount,
				SourceCount:   agg.SourceCount,
				RecentSourceDelta: func() int {
					delta, _ := s.RecentSourceDelta(ctx, card.Cluster.ID, evidenceSince, now)
					return delta
				}(),
				VelocityScore:    out.VelocityScore,
				CredibilityScore: out.CredibilityScore,
				ImpactScore:      out.ImpactScore,
				MomentumScore:    out.MomentumScore,
				FreshnessScore:   out.FreshnessScore,
				RankScore:        out.RankScore,
				DominantSource:   agg.DominantSource,
				ComputedAt:       card.Cluster.UpdatedAt,
				Status:           "fallback",
			}
		}
		card.WindowReceipts, _ = s.receiptsForClusterSince(ctx, card.Cluster.ID, evidenceSince)
		card.Receipts = card.WindowReceipts
		card.WindowTimeline, _ = s.timelineForClusterSince(ctx, card.Cluster.ID, evidenceSince)
		card.Timeline = card.WindowTimeline
		out = append(out, card)
	}
	if len(out) > 0 {
		clusterIDs := make([]int64, len(out))
		for i := range out {
			clusterIDs[i] = out[i].Cluster.ID
		}
		if galleries, err := s.ListEvidencePreviewsForClusters(ctx, clusterIDs, 4); err == nil {
			for i := range out {
				out[i].EvidenceGallery = galleries[out[i].Cluster.ID]
			}
		}
	}
	if err := s.EnrichStoryDisplayTitles(ctx, out); err != nil {
		return nil, err
	}
	return out, rows.Err()
}

func (s *Store) GetStory(ctx context.Context, id int64, userRef string) (*StoryCard, error) {
	var card StoryCard
	var entityID, momentID *int64
	err := s.pool.QueryRow(ctx, `
		SELECT id, entity_id, moment_fp_id, title, summary, category, COALESCE(story_class, ''), state,
		       COALESCE(origin_search_status, ''), origin_checked_at, created_at, updated_at
		FROM story_clusters WHERE id = $1`, id).Scan(
		&card.Cluster.ID, &entityID, &momentID, &card.Cluster.Title, &card.Cluster.Summary,
		&card.Cluster.Category, &card.Cluster.StoryClass, &card.Cluster.State, &card.Cluster.OriginSearchStatus,
		&card.Cluster.OriginCheckedAt, &card.Cluster.CreatedAt, &card.Cluster.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	card.Cluster.EntityID = entityID
	card.Cluster.MomentFPID = momentID
	if entityID != nil {
		card.Entity, _ = s.entityByID(ctx, *entityID)
	}
	if momentID != nil {
		card.Origin, _ = s.fingerprintByID(ctx, *momentID)
	}
	card.Scores, _ = s.scoresForCluster(ctx, id)
	card.Receipts, _ = s.receiptsForCluster(ctx, id)
	card.Timeline, _ = s.timelineForCluster(ctx, id)
	card.EvidenceGallery, _ = s.ListEvidencePreviewsForCluster(ctx, id)
	card.MatchExplanation, _ = s.MatchExplanationForCluster(ctx, id)
	card.OperatorActions, _ = s.ListOperatorActions(ctx, id, 10)
	card.Tracked, _ = s.isFollowed(ctx, id, userRef)
	if ws, _ := s.WindowScoreForCluster(ctx, id, "24h"); ws != nil {
		ws.RecentSourceDelta, _ = s.RecentSourceDelta(ctx, id, ws.Since, time.Now().UTC())
		card.WindowScores = ws
	}
	if err := s.ApplyDisplayTitle(ctx, &card); err != nil {
		return nil, err
	}
	return &card, nil
}

func (s *Store) ListOperatorActions(ctx context.Context, clusterID int64, limit int) ([]OperatorAction, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, cluster_id, action, operator, COALESCE(note, ''), before_data, after_data, created_at
		FROM story_operator_actions
		WHERE cluster_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2`, clusterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OperatorAction
	for rows.Next() {
		var item OperatorAction
		if err := rows.Scan(&item.ID, &item.ClusterID, &item.Action, &item.Operator, &item.Note, &item.BeforeData, &item.AfterData, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) MarkStory(ctx context.Context, clusterID int64, action, operator, note string) (*OperatorAction, error) {
	action = strings.ToLower(strings.TrimSpace(action))
	operator = strings.TrimSpace(operator)
	if operator == "" {
		operator = "operator"
	}
	note = strings.TrimSpace(note)
	var nextCategory, nextClass, nextState string
	switch action {
	case "mark_not_news":
		nextCategory = "not_news"
		nextClass = "not_news"
		nextState = "settled"
	case "mark_community_meta":
		nextCategory = "community_meta"
		nextClass = "community_meta"
	case "mark_debunked":
		nextCategory = "debunked"
		nextClass = "debunked"
		nextState = "settled"
	case "manual_suppress":
		nextState = "suppressed"
	default:
		return nil, fmt.Errorf("unknown operator action %q", action)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var before StoryCluster
	err = tx.QueryRow(ctx, `
		SELECT id, entity_id, moment_fp_id, COALESCE(title, ''), COALESCE(summary, ''),
		       COALESCE(category, ''), COALESCE(story_class, ''), state, created_at, updated_at
		FROM story_clusters WHERE id = $1`, clusterID).Scan(
		&before.ID, &before.EntityID, &before.MomentFPID, &before.Title, &before.Summary,
		&before.Category, &before.StoryClass, &before.State, &before.CreatedAt, &before.UpdatedAt)
	if err != nil {
		return nil, err
	}

	after := before
	if nextCategory != "" {
		after.Category = nextCategory
	}
	if nextClass != "" {
		after.StoryClass = nextClass
	}
	if nextState != "" {
		after.State = nextState
	}
	tag, err := tx.Exec(ctx, `
		UPDATE story_clusters
		SET category = CASE WHEN $2 <> '' THEN $2 ELSE category END,
		    story_class = CASE WHEN $3 <> '' THEN $3 ELSE story_class END,
		    state = CASE WHEN $4 <> '' THEN $4 ELSE state END,
		    updated_at = now()
		WHERE id = $1`, clusterID, nextCategory, nextClass, nextState)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, pgx.ErrNoRows
	}

	beforeData, _ := json.Marshal(map[string]string{
		"category":   before.Category,
		"storyClass": before.StoryClass,
		"state":      before.State,
	})
	afterData, _ := json.Marshal(map[string]string{
		"category":   after.Category,
		"storyClass": after.StoryClass,
		"state":      after.State,
	})

	var item OperatorAction
	err = tx.QueryRow(ctx, `
		INSERT INTO story_operator_actions (cluster_id, action, operator, note, before_data, after_data)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6)
		RETURNING id, cluster_id, action, operator, COALESCE(note, ''), before_data, after_data, created_at`,
		clusterID, action, operator, note, beforeData, afterData).Scan(
		&item.ID, &item.ClusterID, &item.Action, &item.Operator, &item.Note,
		&item.BeforeData, &item.AfterData, &item.CreatedAt)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Store) ConfirmStoryEntity(ctx context.Context, clusterID, entityID int64, operator, note string) (*OperatorAction, error) {
	operator = strings.TrimSpace(operator)
	if operator == "" {
		operator = "operator"
	}
	note = strings.TrimSpace(note)
	if entityID <= 0 {
		return nil, fmt.Errorf("entity id is required")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var before StoryCluster
	err = tx.QueryRow(ctx, `
		SELECT id, entity_id, moment_fp_id, COALESCE(title, ''), COALESCE(summary, ''),
		       COALESCE(category, ''), COALESCE(story_class, ''), state, created_at, updated_at
		FROM story_clusters WHERE id = $1`, clusterID).Scan(
		&before.ID, &before.EntityID, &before.MomentFPID, &before.Title, &before.Summary,
		&before.Category, &before.StoryClass, &before.State, &before.CreatedAt, &before.UpdatedAt)
	if err != nil {
		return nil, err
	}

	var next Entity
	err = tx.QueryRow(ctx, `
		SELECT id, twitch_login, twitch_id, display_name, aliases
		FROM streamer_entities WHERE id = $1`, entityID).Scan(
		&next.ID, &next.TwitchLogin, &next.TwitchID, &next.DisplayName, &next.Aliases)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("entity %d not found", entityID)
	}
	if err != nil {
		return nil, err
	}

	var prev *Entity
	if before.EntityID != nil {
		var found Entity
		if err := tx.QueryRow(ctx, `
			SELECT id, twitch_login, twitch_id, display_name, aliases
			FROM streamer_entities WHERE id = $1`, *before.EntityID).Scan(
			&found.ID, &found.TwitchLogin, &found.TwitchID, &found.DisplayName, &found.Aliases); err == nil {
			prev = &found
		}
	}

	tag, err := tx.Exec(ctx, `
		UPDATE story_clusters
		SET entity_id = $2, updated_at = now()
		WHERE id = $1`, clusterID, entityID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, pgx.ErrNoRows
	}

	beforeData, _ := json.Marshal(operatorEntityAuditData(before.EntityID, prev))
	afterData, _ := json.Marshal(operatorEntityAuditData(&next.ID, &next))

	var item OperatorAction
	err = tx.QueryRow(ctx, `
		INSERT INTO story_operator_actions (cluster_id, action, operator, note, before_data, after_data)
		VALUES ($1, 'confirm_streamer_entity', $2, NULLIF($3, ''), $4, $5)
		RETURNING id, cluster_id, action, operator, COALESCE(note, ''), before_data, after_data, created_at`,
		clusterID, operator, note, beforeData, afterData).Scan(
		&item.ID, &item.ClusterID, &item.Action, &item.Operator, &item.Note,
		&item.BeforeData, &item.AfterData, &item.CreatedAt)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &item, nil
}

func operatorEntityAuditData(entityID *int64, entity *Entity) map[string]any {
	data := map[string]any{
		"entityId": entityID,
	}
	if entity != nil {
		data["entityLogin"] = entity.TwitchLogin
		data["entityDisplayName"] = entity.DisplayName
	}
	return data
}

func (s *Store) ConfirmStoryOrigin(ctx context.Context, clusterID, momentFPID int64, operator, note string) (*OperatorAction, error) {
	operator = strings.TrimSpace(operator)
	if operator == "" {
		operator = "operator"
	}
	note = strings.TrimSpace(note)
	if momentFPID <= 0 {
		return nil, fmt.Errorf("moment fingerprint id is required")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var before StoryCluster
	err = tx.QueryRow(ctx, `
		SELECT id, entity_id, moment_fp_id, COALESCE(title, ''), COALESCE(summary, ''),
		       COALESCE(category, ''), COALESCE(story_class, ''), state, created_at, updated_at
		FROM story_clusters WHERE id = $1`, clusterID).Scan(
		&before.ID, &before.EntityID, &before.MomentFPID, &before.Title, &before.Summary,
		&before.Category, &before.StoryClass, &before.State, &before.CreatedAt, &before.UpdatedAt)
	if err != nil {
		return nil, err
	}

	next, err := fingerprintAuditData(ctx, tx, momentFPID)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("moment fingerprint %d not found", momentFPID)
	}
	if err != nil {
		return nil, err
	}
	prev := map[string]any{"momentFpId": before.MomentFPID}
	if before.MomentFPID != nil {
		if data, err := fingerprintAuditData(ctx, tx, *before.MomentFPID); err == nil {
			prev = data
		}
	}

	tag, err := tx.Exec(ctx, `
		UPDATE story_clusters
		SET moment_fp_id = $2,
		    origin_search_status = 'matched',
		    origin_checked_at = now(),
		    updated_at = now()
		WHERE id = $1`, clusterID, momentFPID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, pgx.ErrNoRows
	}

	beforeData, _ := json.Marshal(prev)
	afterData, _ := json.Marshal(next)

	var item OperatorAction
	err = tx.QueryRow(ctx, `
		INSERT INTO story_operator_actions (cluster_id, action, operator, note, before_data, after_data)
		VALUES ($1, 'confirm_origin_moment', $2, NULLIF($3, ''), $4, $5)
		RETURNING id, cluster_id, action, operator, COALESCE(note, ''), before_data, after_data, created_at`,
		clusterID, operator, note, beforeData, afterData).Scan(
		&item.ID, &item.ClusterID, &item.Action, &item.Operator, &item.Note,
		&item.BeforeData, &item.AfterData, &item.CreatedAt)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &item, nil
}

func fingerprintAuditData(ctx context.Context, q pgx.Tx, momentFPID int64) (map[string]any, error) {
	var streamID, vodID string
	var vodOffsetS int
	err := q.QueryRow(ctx, `
		SELECT stream_id, COALESCE(vod_id, ''), vod_offset_s
		FROM moment_fingerprints WHERE id = $1`, momentFPID).Scan(&streamID, &vodID, &vodOffsetS)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"momentFpId": momentFPID,
		"streamId":   streamID,
		"vodId":      vodID,
		"vodOffsetS": vodOffsetS,
	}, nil
}

func (s *Store) MergeDuplicateStory(ctx context.Context, sourceClusterID, targetClusterID int64, operator, note string) (*OperatorAction, error) {
	operator = strings.TrimSpace(operator)
	if operator == "" {
		operator = "operator"
	}
	note = strings.TrimSpace(note)
	if sourceClusterID <= 0 || targetClusterID <= 0 {
		return nil, fmt.Errorf("source and target story ids are required")
	}
	if sourceClusterID == targetClusterID {
		return nil, fmt.Errorf("source and target story ids must differ")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	source, err := clusterAuditData(ctx, tx, sourceClusterID)
	if err != nil {
		return nil, err
	}
	target, err := clusterAuditData(ctx, tx, targetClusterID)
	if err != nil {
		return nil, err
	}
	evidenceIDs, err := evidenceIDsForCluster(ctx, tx, sourceClusterID)
	if err != nil {
		return nil, err
	}
	previewIDs, err := previewIDsForCluster(ctx, tx, sourceClusterID)
	if err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO story_evidence_previews (cluster_id, evidence_id, preview_id, match_kind, note)
		SELECT $2, evidence_id, preview_id, match_kind, note
		FROM story_evidence_previews
		WHERE cluster_id = $1
		ON CONFLICT (cluster_id, preview_id) DO UPDATE SET
			evidence_id = COALESCE(EXCLUDED.evidence_id, story_evidence_previews.evidence_id),
			match_kind = EXCLUDED.match_kind,
			note = COALESCE(NULLIF(EXCLUDED.note, ''), story_evidence_previews.note)`,
		sourceClusterID, targetClusterID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM story_evidence_previews WHERE cluster_id = $1`, sourceClusterID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE story_evidence SET cluster_id = $2 WHERE cluster_id = $1`, sourceClusterID, targetClusterID); err != nil {
		return nil, err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE story_clusters
		SET state = 'suppressed', updated_at = now()
		WHERE id = $1`, sourceClusterID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, pgx.ErrNoRows
	}
	if _, err := tx.Exec(ctx, `UPDATE story_clusters SET updated_at = now() WHERE id = $1`, targetClusterID); err != nil {
		return nil, err
	}

	beforeData, _ := json.Marshal(map[string]any{
		"source":      source,
		"target":      target,
		"evidenceIds": evidenceIDs,
		"previewIds":  previewIDs,
	})
	afterData, _ := json.Marshal(map[string]any{
		"sourceClusterId": sourceClusterID,
		"targetClusterId": targetClusterID,
		"sourceState":     "suppressed",
		"evidenceIds":     evidenceIDs,
		"previewIds":      previewIDs,
		"movedEvidence":   len(evidenceIDs),
		"movedPreviews":   len(previewIDs),
	})

	item, err := insertOperatorAction(ctx, tx, sourceClusterID, "merge_duplicate_story", operator, note, beforeData, afterData)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Store) SplitUnrelatedEvidence(ctx context.Context, sourceClusterID int64, evidenceIDs []int64, title, operator, note string) (*OperatorAction, error) {
	operator = strings.TrimSpace(operator)
	if operator == "" {
		operator = "operator"
	}
	note = strings.TrimSpace(note)
	title = strings.TrimSpace(title)
	if sourceClusterID <= 0 {
		return nil, fmt.Errorf("source story id is required")
	}
	evidenceIDs = uniquePositiveInt64s(evidenceIDs)
	if len(evidenceIDs) == 0 {
		return nil, fmt.Errorf("at least one evidence id is required")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	source, err := clusterAuditData(ctx, tx, sourceClusterID)
	if err != nil {
		return nil, err
	}
	ownedEvidenceIDs, err := ownedEvidenceIDs(ctx, tx, sourceClusterID, evidenceIDs)
	if err != nil {
		return nil, err
	}
	if len(ownedEvidenceIDs) != len(evidenceIDs) {
		return nil, fmt.Errorf("all evidence ids must belong to story %d", sourceClusterID)
	}
	previewIDs, err := previewIDsForEvidence(ctx, tx, sourceClusterID, evidenceIDs)
	if err != nil {
		return nil, err
	}
	if title == "" {
		title = splitStoryTitle(ctx, tx, sourceClusterID, evidenceIDs)
	}

	var newClusterID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO story_clusters (entity_id, title, summary, category, story_class, state)
		SELECT entity_id, $2, summary, category, story_class, 'developing'
		FROM story_clusters
		WHERE id = $1
		RETURNING id`, sourceClusterID, title).Scan(&newClusterID)
	if err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx, `UPDATE story_evidence SET cluster_id = $2 WHERE cluster_id = $1 AND id = ANY($3)`, sourceClusterID, newClusterID, evidenceIDs); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE story_evidence_previews
		SET cluster_id = $2
		WHERE cluster_id = $1
		  AND evidence_id = ANY($3)`, sourceClusterID, newClusterID, evidenceIDs); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE story_clusters SET updated_at = now() WHERE id = $1`, sourceClusterID); err != nil {
		return nil, err
	}

	beforeData, _ := json.Marshal(map[string]any{
		"source":      source,
		"evidenceIds": ownedEvidenceIDs,
		"previewIds":  previewIDs,
	})
	afterData, _ := json.Marshal(map[string]any{
		"sourceClusterId": sourceClusterID,
		"newClusterId":    newClusterID,
		"evidenceIds":     ownedEvidenceIDs,
		"previewIds":      previewIDs,
		"title":           title,
	})
	item, err := insertOperatorAction(ctx, tx, sourceClusterID, "split_unrelated_evidence", operator, note, beforeData, afterData)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return item, nil
}

func insertOperatorAction(ctx context.Context, tx pgx.Tx, clusterID int64, action, operator, note string, beforeData, afterData []byte) (*OperatorAction, error) {
	var item OperatorAction
	err := tx.QueryRow(ctx, `
		INSERT INTO story_operator_actions (cluster_id, action, operator, note, before_data, after_data)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6)
		RETURNING id, cluster_id, action, operator, COALESCE(note, ''), before_data, after_data, created_at`,
		clusterID, action, operator, note, beforeData, afterData).Scan(
		&item.ID, &item.ClusterID, &item.Action, &item.Operator, &item.Note,
		&item.BeforeData, &item.AfterData, &item.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func clusterAuditData(ctx context.Context, q pgx.Tx, clusterID int64) (map[string]any, error) {
	var entityID, momentFPID *int64
	var title, category, storyClass, state string
	err := q.QueryRow(ctx, `
		SELECT entity_id, moment_fp_id, COALESCE(title, ''), COALESCE(category, ''), COALESCE(story_class, ''), state
		FROM story_clusters WHERE id = $1`, clusterID).Scan(&entityID, &momentFPID, &title, &category, &storyClass, &state)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"clusterId":  clusterID,
		"entityId":   entityID,
		"momentFpId": momentFPID,
		"title":      title,
		"category":   category,
		"storyClass": storyClass,
		"state":      state,
	}, nil
}

func evidenceIDsForCluster(ctx context.Context, q pgx.Tx, clusterID int64) ([]int64, error) {
	rows, err := q.Query(ctx, `SELECT id FROM story_evidence WHERE cluster_id = $1 ORDER BY id`, clusterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func previewIDsForCluster(ctx context.Context, q pgx.Tx, clusterID int64) ([]int64, error) {
	rows, err := q.Query(ctx, `SELECT preview_id FROM story_evidence_previews WHERE cluster_id = $1 ORDER BY preview_id`, clusterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func previewIDsForEvidence(ctx context.Context, q pgx.Tx, clusterID int64, evidenceIDs []int64) ([]int64, error) {
	rows, err := q.Query(ctx, `
		SELECT preview_id
		FROM story_evidence_previews
		WHERE cluster_id = $1 AND evidence_id = ANY($2)
		ORDER BY preview_id`, clusterID, evidenceIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func ownedEvidenceIDs(ctx context.Context, q pgx.Tx, clusterID int64, evidenceIDs []int64) ([]int64, error) {
	rows, err := q.Query(ctx, `
		SELECT id
		FROM story_evidence
		WHERE cluster_id = $1 AND id = ANY($2)
		ORDER BY id`, clusterID, evidenceIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func splitStoryTitle(ctx context.Context, q pgx.Tx, clusterID int64, evidenceIDs []int64) string {
	var title string
	_ = q.QueryRow(ctx, `
		SELECT COALESCE(si.text, ev.source_type, 'Split evidence')
		FROM story_evidence ev
		LEFT JOIN social_items si ON si.id = ev.item_id
		WHERE ev.cluster_id = $1 AND ev.id = ANY($2)
		ORDER BY ev.id
		LIMIT 1`, clusterID, evidenceIDs).Scan(&title)
	title = strings.TrimSpace(title)
	if title == "" {
		return "Split evidence review"
	}
	if len(title) > 140 {
		title = strings.TrimSpace(title[:140])
	}
	return title
}

func uniquePositiveInt64s(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	out := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func (s *Store) ListDeveloping(ctx context.Context, limit int) ([]StoryCard, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id
		FROM story_clusters
		WHERE state IN ('developing', 'unverified')
		ORDER BY updated_at DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StoryCard
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		card, err := s.GetStory(ctx, id, "local")
		if err != nil || card == nil {
			continue
		}
		out = append(out, *card)
	}
	return out, rows.Err()
}

func (s *Store) ListRising(ctx context.Context, since time.Time, limit int) ([]struct {
	ID         int64    `json:"id"`
	Title      string   `json:"title"`
	Volatility *float64 `json:"volatility"`
}, error) {
	if limit <= 0 {
		limit = 5
	}
	if since.IsZero() {
		since = time.Now().Add(-24 * time.Hour)
	}
	rows, err := s.pool.Query(ctx, `
		SELECT c.id, COALESCE(c.title, ''), s.volatility
		FROM story_clusters c
		JOIN story_scores s ON s.cluster_id = c.id
		WHERE c.state IN ('published', 'developing', 'unverified')
		  AND s.volatility IS NOT NULL
		  AND EXISTS (
		    SELECT 1 FROM story_evidence ev
		    WHERE ev.cluster_id = c.id
		      AND COALESCE(ev.occurred_at, ev.created_at) >= $2
		  )
		ORDER BY s.volatility DESC, c.updated_at DESC
		LIMIT $1`, limit, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []struct {
		ID         int64    `json:"id"`
		Title      string   `json:"title"`
		Volatility *float64 `json:"volatility"`
	}
	for rows.Next() {
		var row struct {
			ID         int64    `json:"id"`
			Title      string   `json:"title"`
			Volatility *float64 `json:"volatility"`
		}
		if err := rows.Scan(&row.ID, &row.Title, &row.Volatility); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// ListRisingStoryCards returns full story cards for the highest-volatility clusters in a window.
func (s *Store) ListRisingStoryCards(ctx context.Context, since time.Time, limit int) ([]StoryCard, error) {
	rows, err := s.ListRising(ctx, since, limit)
	if err != nil {
		return nil, err
	}
	out := make([]StoryCard, 0, len(rows))
	for _, row := range rows {
		card, err := s.GetStory(ctx, row.ID, "local")
		if err != nil || card == nil {
			continue
		}
		if ws, _ := s.WindowScoreForCluster(ctx, row.ID, "24h"); ws != nil {
			card.WindowScores = ws
		}
		out = append(out, *card)
	}
	return out, nil
}

func (s *Store) SourceMix(ctx context.Context, since time.Time) (map[string]int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT source_type, COUNT(*)::int
		FROM story_evidence
		WHERE COALESCE(occurred_at, created_at) >= $1
		GROUP BY source_type`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var src string
		var n int
		if err := rows.Scan(&src, &n); err != nil {
			return nil, err
		}
		out[src] = n
	}
	return out, rows.Err()
}

// LastEvidenceBySource returns the most recent evidence timestamp per source_type.
func (s *Store) LastEvidenceBySource(ctx context.Context) (map[string]time.Time, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT source_type, MAX(COALESCE(occurred_at, created_at))
		FROM story_evidence
		GROUP BY source_type`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]time.Time{}
	for rows.Next() {
		var src string
		var ts time.Time
		if err := rows.Scan(&src, &ts); err != nil {
			return nil, err
		}
		out[src] = ts
	}
	return out, rows.Err()
}

func (s *Store) Follow(ctx context.Context, clusterID int64, userRef string) error {
	if userRef == "" {
		userRef = "local"
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO story_follows (cluster_id, user_ref) VALUES ($1, $2)
		ON CONFLICT DO NOTHING`, clusterID, userRef)
	return err
}

func (s *Store) Unfollow(ctx context.Context, clusterID int64, userRef string) error {
	if userRef == "" {
		userRef = "local"
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM story_follows WHERE cluster_id = $1 AND user_ref = $2`, clusterID, userRef)
	return err
}

func normalizeWatchEntry(kind, value, label string) (string, string, string, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	value = strings.ToLower(strings.TrimSpace(value))
	label = strings.TrimSpace(label)
	switch kind {
	case "category":
		switch value {
		case "drama", "funny", "bans", "records", "esports", "unverified", "not_news", "community_meta", "debunked":
		default:
			return "", "", "", fmt.Errorf("unsupported watch category %q", value)
		}
	case "keyword":
		value = strings.Join(strings.Fields(value), " ")
		if len(value) < 2 || len(value) > 80 {
			return "", "", "", fmt.Errorf("keyword watch must be 2-80 characters")
		}
	default:
		return "", "", "", fmt.Errorf("unsupported watch kind %q", kind)
	}
	if label == "" {
		label = value
	}
	return kind, value, label, nil
}

func (s *Store) ListWatchEntries(ctx context.Context, userRef string) ([]WatchEntry, error) {
	if userRef == "" {
		userRef = "local"
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_ref, kind, value, COALESCE(label, ''), created_at
		FROM story_watch_entries
		WHERE user_ref = $1
		ORDER BY created_at DESC, id DESC`, userRef)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WatchEntry{}
	for rows.Next() {
		var item WatchEntry
		if err := rows.Scan(&item.ID, &item.UserRef, &item.Kind, &item.Value, &item.Label, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) UpsertWatchEntry(ctx context.Context, userRef, kind, value, label string) (*WatchEntry, error) {
	if userRef == "" {
		userRef = "local"
	}
	kind, value, label, err := normalizeWatchEntry(kind, value, label)
	if err != nil {
		return nil, err
	}
	var item WatchEntry
	err = s.pool.QueryRow(ctx, `
		INSERT INTO story_watch_entries (user_ref, kind, value, label)
		VALUES ($1, $2, $3, NULLIF($4, ''))
		ON CONFLICT (user_ref, kind, value) DO UPDATE SET
			label = COALESCE(NULLIF(EXCLUDED.label, ''), story_watch_entries.label),
			created_at = story_watch_entries.created_at
		RETURNING id, user_ref, kind, value, COALESCE(label, ''), created_at`,
		userRef, kind, value, label).Scan(&item.ID, &item.UserRef, &item.Kind, &item.Value, &item.Label, &item.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Store) DeleteWatchEntry(ctx context.Context, userRef string, id int64) error {
	if userRef == "" {
		userRef = "local"
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM story_watch_entries WHERE id = $1 AND user_ref = $2`, id, userRef)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) DeleteExpiredItems(ctx context.Context) (int64, error) {
	if s.archiveProtectRetention {
		var missing int64
		err := s.pool.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM social_items si
			WHERE si.expires_at < now()
			  AND NOT EXISTS (
				SELECT 1
				FROM archive_exports ae
				WHERE ae.artifact_type = $1
				  AND ae.natural_key = si.source || ':' || si.external_id
				  AND ae.export_status = 'confirmed'
			  )`,
			archive.ArtifactSocialItem,
		).Scan(&missing)
		if err != nil {
			return 0, err
		}
		if err := archive.BlockIfMissing(archive.ArtifactSocialItem, missing); err != nil {
			return 0, err
		}
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM social_items WHERE expires_at < now()`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *Store) ListTrendingStreamers(ctx context.Context, since time.Time, limit int) ([]TrendingStreamer, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	if since.IsZero() {
		since = time.Now().Add(-24 * time.Hour)
	}
	rows, err := s.pool.Query(ctx, `
		SELECT e.twitch_login,
		       COALESCE(NULLIF(e.display_name, ''), e.twitch_login) AS display_name,
		       COUNT(DISTINCT c.id)::int AS story_count,
		       COUNT(ev.id)::int AS evidence_count,
		       COUNT(DISTINCT ev.source_type)::int AS source_diversity,
		       MAX(sc.volatility) AS volatility,
		       MAX(GREATEST(c.updated_at, COALESCE(ev.occurred_at, ev.created_at))) AS last_seen
		FROM story_clusters c
		JOIN streamer_entities e ON e.id = c.entity_id
		JOIN story_evidence ev ON ev.cluster_id = c.id
		  AND COALESCE(ev.occurred_at, ev.created_at) >= $2
		LEFT JOIN story_scores sc ON sc.cluster_id = c.id
		WHERE c.state IN ('published', 'developing', 'unverified')
		GROUP BY e.twitch_login, COALESCE(NULLIF(e.display_name, ''), e.twitch_login)
		ORDER BY story_count DESC, evidence_count DESC, source_diversity DESC, last_seen DESC, volatility DESC NULLS LAST
		LIMIT $1`, limit, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]TrendingStreamer, 0, limit)
	for rows.Next() {
		var row TrendingStreamer
		if err := rows.Scan(&row.Login, &row.DisplayName, &row.StoryCount, &row.EvidenceCount, &row.SourceDiversity, &row.Volatility, &row.LastSeen); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) entityByID(ctx context.Context, id int64) (*Entity, error) {
	var e Entity
	err := s.pool.QueryRow(ctx, `
		SELECT id, twitch_login, twitch_id, display_name, aliases FROM streamer_entities WHERE id = $1`, id).
		Scan(&e.ID, &e.TwitchLogin, &e.TwitchID, &e.DisplayName, &e.Aliases)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (s *Store) fingerprintByID(ctx context.Context, id int64) (*MomentFingerprint, error) {
	var fp MomentFingerprint
	var entityID *int64
	err := s.pool.QueryRow(ctx, `
		SELECT id, entity_id, stream_id, COALESCE(vod_id, ''), vod_offset_s, transcript_kw, top_emotes,
		       COALESCE(chat_spike_summary, ''), origin_confidence, game, fp_version, created_at
		FROM moment_fingerprints WHERE id = $1`, id).
		Scan(&fp.ID, &entityID, &fp.StreamID, &fp.VODID, &fp.VODOffsetS, &fp.TranscriptKW, &fp.TopEmotes, &fp.ChatSpikeSummary, &fp.OriginConfidence, &fp.Game, &fp.FPVersion, &fp.CreatedAt)
	if err != nil {
		return nil, err
	}
	fp.EntityID = entityID
	fp.OriginSpikePoints, _ = s.originSpikePoints(ctx, fp.StreamID, fp.VODOffsetS, 180)
	return &fp, nil
}

func (s *Store) originSpikePoints(ctx context.Context, streamID string, offsetS, radiusS int) ([]OriginSpikePoint, error) {
	streamID = strings.TrimSpace(streamID)
	if streamID == "" || offsetS < 0 {
		return nil, nil
	}
	if radiusS <= 0 {
		radiusS = 180
	}
	rows, err := s.pool.Query(ctx, `
		SELECT GREATEST(0, EXTRACT(EPOCH FROM (r.minute_ts - a.started_at))::int) AS offset_s,
		       EXTRACT(EPOCH FROM (r.minute_ts - a.started_at))::int - $2 AS relative_s,
		       r.chat_count,
		       r.total_emote_count,
		       r.seventv_emote_count,
		       r.viewer_max
		FROM analytics_minute_rollups r
		JOIN analytics_streams a ON a.stream_id = r.stream_id
		WHERE r.stream_id = $1
		  AND r.minute_ts BETWEEN a.started_at + (($2 - $3)::double precision * interval '1 second')
		                      AND a.started_at + (($2 + $3)::double precision * interval '1 second')
		ORDER BY r.minute_ts ASC`, streamID, offsetS, radiusS)
	if err != nil {
		// Older/lightweight test schemas may omit Analytics tables; origin cards should
		// still render their non-graph proof fields.
		return nil, nil
	}
	defer rows.Close()
	out := make([]OriginSpikePoint, 0, 9)
	for rows.Next() {
		var point OriginSpikePoint
		if err := rows.Scan(&point.OffsetS, &point.RelativeS, &point.ChatCount, &point.TotalEmoteCount, &point.SevenTVEmoteCount, &point.ViewerMax); err != nil {
			return nil, err
		}
		out = append(out, point)
	}
	return out, rows.Err()
}

// ScoresForCluster returns current story_scores for a cluster.
func (s *Store) ScoresForCluster(ctx context.Context, id int64) (Scores, error) {
	return s.scoresForCluster(ctx, id)
}

func (s *Store) scoresForCluster(ctx context.Context, id int64) (Scores, error) {
	var sc Scores
	err := s.pool.QueryRow(ctx, `
		SELECT trend, volatility, confidence, sentiment, factors, updated_at
		FROM story_scores WHERE cluster_id = $1`, id).
		Scan(&sc.Trend, &sc.Volatility, &sc.Confidence, &sc.Sentiment, &sc.Factors, &sc.UpdatedAt)
	if err == pgx.ErrNoRows {
		return Scores{}, nil
	}
	return sc, err
}

// ReceiptsForCluster returns per-source receipt percentages.
func (s *Store) ReceiptsForCluster(ctx context.Context, id int64) ([]Receipt, error) {
	return s.receiptsForCluster(ctx, id)
}

func (s *Store) receiptsForCluster(ctx context.Context, id int64) ([]Receipt, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT ev.source_type,
		       COALESCE(MAX(sr.source_risk), ''),
		       ROUND(100 * AVG(ev.match_conf))::int,
		       COALESCE(MAX(ev.source_url), ''),
		       COALESCE(MAX(
		         CASE
		           WHEN p.id IS NULL THEN ''
		           WHEN COALESCE(p.error, '') <> '' THEN 'error'
		           WHEN COALESCE(p.embed_url, '') <> '' OR COALESCE(p.embed_html, '') <> '' OR COALESCE(p.thumbnail_url, '') <> '' OR COALESCE(p.title, '') <> '' THEN 'ready'
		           ELSE 'fallback'
		         END
		       ), ''),
		       COALESCE(MAX(p.id), 0),
		       COALESCE(MAX(NULLIF(p.thumbnail_url, '')), '')
		FROM story_evidence ev
		LEFT JOIN source_reliability sr ON sr.source_type = ev.source_type
		LEFT JOIN story_evidence_previews sep ON sep.evidence_id = ev.id
		LEFT JOIN evidence_previews p ON p.id = sep.preview_id
		WHERE ev.cluster_id = $1
		GROUP BY ev.source_type`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Receipt
	for rows.Next() {
		var r Receipt
		var thumb string
		if err := rows.Scan(&r.SourceType, &r.Risk, &r.Pct, &r.URL, &r.PreviewStatus, &r.PreviewID, &thumb); err != nil {
			return nil, err
		}
		r.ThumbnailURL = receiptDisplayThumbnail(thumb, r.URL)
		out = append(out, r)
	}
	return out, rows.Err()
}

func receiptDisplayThumbnail(storedThumb, sourceURL string) string {
	if clip := twitchClipPreviewURL(sourceURL); clip != "" && strings.TrimSpace(storedThumb) == "" {
		return displayThumbnailURL(clip, "")
	}
	return displayThumbnailURL(storedThumb, "")
}

func (s *Store) receiptsForClusterSince(ctx context.Context, id int64, since time.Time) ([]Receipt, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT ev.source_type,
		       COALESCE(MAX(sr.source_risk), ''),
		       ROUND(100 * AVG(ev.match_conf))::int,
		       COALESCE(MAX(ev.source_url), ''),
		       COALESCE(MAX(
		         CASE
		           WHEN p.id IS NULL THEN ''
		           WHEN COALESCE(p.error, '') <> '' THEN 'error'
		           WHEN COALESCE(p.embed_url, '') <> '' OR COALESCE(p.embed_html, '') <> '' OR COALESCE(p.thumbnail_url, '') <> '' OR COALESCE(p.title, '') <> '' THEN 'ready'
		           ELSE 'fallback'
		         END
		       ), ''),
		       COALESCE(MAX(p.id), 0),
		       COALESCE(MAX(NULLIF(p.thumbnail_url, '')), '')
		FROM story_evidence ev
		LEFT JOIN source_reliability sr ON sr.source_type = ev.source_type
		LEFT JOIN story_evidence_previews sep ON sep.evidence_id = ev.id
		LEFT JOIN evidence_previews p ON p.id = sep.preview_id
		WHERE ev.cluster_id = $1
		  AND COALESCE(ev.occurred_at, ev.created_at) >= $2
		GROUP BY ev.source_type
		ORDER BY MAX(COALESCE(ev.occurred_at, ev.created_at)) DESC`, id, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Receipt
	for rows.Next() {
		var r Receipt
		var thumb string
		if err := rows.Scan(&r.SourceType, &r.Risk, &r.Pct, &r.URL, &r.PreviewStatus, &r.PreviewID, &thumb); err != nil {
			return nil, err
		}
		r.ThumbnailURL = receiptDisplayThumbnail(thumb, r.URL)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) timelineForCluster(ctx context.Context, id int64) ([]TimelineStep, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT COALESCE(occurred_at, created_at), source_type, COALESCE(source_url, '')
		FROM story_evidence WHERE cluster_id = $1
		ORDER BY COALESCE(occurred_at, created_at) ASC`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TimelineStep
	for rows.Next() {
		var step TimelineStep
		if err := rows.Scan(&step.At, &step.SourceType, &step.SourceURL); err != nil {
			return nil, err
		}
		step.Label = timelineLabel(step.SourceType)
		out = append(out, step)
	}
	return out, rows.Err()
}

func (s *Store) timelineForClusterSince(ctx context.Context, id int64, since time.Time) ([]TimelineStep, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT COALESCE(occurred_at, created_at), source_type, COALESCE(source_url, '')
		FROM story_evidence
		WHERE cluster_id = $1
		  AND COALESCE(occurred_at, created_at) >= $2
		ORDER BY COALESCE(occurred_at, created_at) ASC`, id, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TimelineStep
	for rows.Next() {
		var step TimelineStep
		if err := rows.Scan(&step.At, &step.SourceType, &step.SourceURL); err != nil {
			return nil, err
		}
		step.Label = timelineLabel(step.SourceType)
		out = append(out, step)
	}
	return out, rows.Err()
}

func (s *Store) isFollowed(ctx context.Context, clusterID int64, userRef string) (bool, error) {
	if userRef == "" {
		userRef = "local"
	}
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT 1 FROM story_follows WHERE cluster_id = $1 AND user_ref = $2 LIMIT 1`, clusterID, userRef).Scan(&n)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

// ReclassifyRecentCategories re-runs category classification on recent clusters.
func (s *Store) ReclassifyRecentCategories(ctx context.Context, limit int, classify func(title, flair string) string) (int, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.pool.Query(ctx, `
		SELECT c.id, COALESCE(c.title, ''), COALESCE(c.category, ''),
		       COALESCE(src.source, ''), COALESCE(src.flair, '')
		FROM story_clusters c
		LEFT JOIN LATERAL (
			SELECT si.source, COALESCE(si.metrics->>'flair', '') AS flair
			FROM story_evidence ev
			JOIN social_items si ON si.id = ev.item_id
			WHERE ev.cluster_id = c.id
			ORDER BY ev.id DESC
			LIMIT 1
		) src ON true
		ORDER BY c.updated_at DESC
		LIMIT $1`, limit)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	updated := 0
	for rows.Next() {
		var id int64
		var title, existing, source, flair string
		if err := rows.Scan(&id, &title, &existing, &source, &flair); err != nil {
			return updated, err
		}
		if source == "streamerbans_post" {
			continue
		}
		next := classify(title, flair)
		if next == "" || next == existing {
			continue
		}
		tag, err := s.pool.Exec(ctx, `
			UPDATE story_clusters SET category = $2, updated_at = now() WHERE id = $1`, id, next)
		if err != nil {
			return updated, err
		}
		if tag.RowsAffected() > 0 {
			updated++
		}
	}
	return updated, rows.Err()
}

func timelineLabel(sourceType string) string {
	switch sourceType {
	case "pulse_origin":
		return "Origin moment"
	case "reddit_thread":
		return "Reddit pickup"
	case "youtube_video":
		return "YouTube spread"
	case "twitch_clip":
		return "Twitch clip"
	default:
		return fmt.Sprintf("%s linked", sourceType)
	}
}
