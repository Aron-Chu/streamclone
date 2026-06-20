package store

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"streamclone/internal/social/reddit"
)

var redditSubredditRE = regexp.MustCompile(`(?i)reddit\.com/r/([^/]+)`)
var redditTitleVoteSuffixRE = regexp.MustCompile(`(?i)\s+(\d+)\s+votes?\s*[•·]\s*(\d+)\s+comments?\s*$`)

// CommunityPost is a social-native Pulse Wire trending item.
type CommunityPost struct {
	ID                  int64      `json:"id"`
	ExternalID          string     `json:"externalId,omitempty"`
	Title               string     `json:"title"`
	URL                 string     `json:"url"`
	Permalink           string     `json:"permalink,omitempty"`
	Source              string     `json:"source"`
	Subreddit           string     `json:"subreddit,omitempty"`
	Score               float64    `json:"score"`
	Comments            float64    `json:"comments"`
	ThumbnailURL        string     `json:"thumbnailUrl,omitempty"`
	DisplayThumbnailURL string     `json:"displayThumbnailUrl,omitempty"`
	PreviewKind         string     `json:"previewKind"`
	PreviewURL          string     `json:"previewUrl,omitempty"`
	SelfText            string     `json:"selfText,omitempty"`
	EmbedURL            string     `json:"embedUrl,omitempty"`
	EmbedHTML           string     `json:"embedHtml,omitempty"`
	LinkedPlatform      string     `json:"linkedPlatform,omitempty"`
	StreamerLogin       string     `json:"streamerLogin,omitempty"`
	StreamerDisplayName string     `json:"streamerDisplayName,omitempty"`
	Flair               string     `json:"flair,omitempty"`
	Category            string     `json:"category,omitempty"`
	PostedAt            *time.Time `json:"postedAt,omitempty"`
}

// CommunityFlair is a ranked Reddit link flair tag in the trending window.
type CommunityFlair struct {
	Flair string `json:"flair"`
	Count int    `json:"count"`
}

// UnlinkedEvidence is social evidence not yet represented on the Wire feed.
type UnlinkedEvidence struct {
	ID                  int64      `json:"id"`
	Title               string     `json:"title"`
	URL                 string     `json:"url"`
	Source              string     `json:"source"`
	Category            string     `json:"category,omitempty"`
	Score               float64    `json:"score,omitempty"`
	Comments            float64    `json:"comments,omitempty"`
	ViewCount           float64    `json:"viewCount,omitempty"`
	PreviewKind         string     `json:"previewKind"`
	PreviewURL          string     `json:"previewUrl,omitempty"`
	ThumbnailURL        string     `json:"thumbnailUrl,omitempty"`
	DisplayThumbnailURL string     `json:"displayThumbnailUrl,omitempty"`
	StreamerLogin       string     `json:"streamerLogin,omitempty"`
	StreamerDisplayName string     `json:"streamerDisplayName,omitempty"`
	PostedAt            *time.Time `json:"postedAt,omitempty"`
}

// TopClip is a ranked Twitch clip for the trending strip.
type TopClip struct {
	ID                  int64      `json:"id"`
	ExternalID          string     `json:"externalId,omitempty"`
	Title               string     `json:"title"`
	URL                 string     `json:"url"`
	ViewCount           float64    `json:"viewCount"`
	DurationSeconds     float64    `json:"durationSeconds,omitempty"`
	ThumbnailURL        string     `json:"thumbnailUrl,omitempty"`
	DisplayThumbnailURL string     `json:"displayThumbnailUrl,omitempty"`
	StreamerLogin       string     `json:"streamerLogin,omitempty"`
	StreamerDisplayName string     `json:"streamerDisplayName,omitempty"`
	PostedAt            *time.Time `json:"postedAt,omitempty"`
}

func socialItemHeadline(text string) string {
	return CleanSocialHeadline(text)
}

func parseMetricFloat(raw json.RawMessage, key string) float64 {
	if len(raw) == 0 {
		return 0
	}
	var metrics map[string]any
	if err := json.Unmarshal(raw, &metrics); err != nil {
		return 0
	}
	switch v := metrics[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case json.Number:
		f, _ := v.Float64()
		return f
	default:
		return 0
	}
}

func redditSubreddit(url string) string {
	if m := redditSubredditRE.FindStringSubmatch(url); len(m) > 1 {
		return m[1]
	}
	return ""
}

func parseVoteCommentFromTitle(text string) (score, comments float64, cleaned string) {
	text = strings.TrimSpace(text)
	match := redditTitleVoteSuffixRE.FindStringSubmatch(text)
	if len(match) < 3 {
		return 0, 0, text
	}
	if s, err := strconv.ParseFloat(match[1], 64); err == nil {
		score = s
	}
	if c, err := strconv.ParseFloat(match[2], 64); err == nil {
		comments = c
	}
	cleaned = strings.TrimSpace(redditTitleVoteSuffixRE.ReplaceAllString(text, ""))
	return score, comments, cleaned
}

func parseMetricString(raw json.RawMessage, key string) string {
	if len(raw) == 0 {
		return ""
	}
	var metrics map[string]any
	if err := json.Unmarshal(raw, &metrics); err != nil {
		return ""
	}
	switch v := metrics[key].(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return ""
	}
}

func communitySources(category string) []string {
	if category == "bans" {
		return []string{"streamerbans_post"}
	}
	return []string{"reddit"}
}

// ListCommunityFlairs returns Reddit link flairs ranked by post count in the window.
func (s *Store) ListCommunityFlairs(ctx context.Context, since time.Time, limit int) ([]CommunityFlair, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx, `
		SELECT TRIM(si.metrics->>'flair') AS flair, COUNT(*)::int AS cnt
		FROM social_items si
		WHERE si.source = 'reddit'
		  AND si.kind = 'post'
		  AND COALESCE(si.created_at_src, si.ingested_at) >= $1
		  AND NULLIF(TRIM(COALESCE(si.metrics->>'flair', '')), '') IS NOT NULL
		GROUP BY TRIM(si.metrics->>'flair')
		ORDER BY cnt DESC, flair ASC
		LIMIT $2`, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CommunityFlair
	for rows.Next() {
		var row CommunityFlair
		if err := rows.Scan(&row.Flair, &row.Count); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	if out == nil {
		out = []CommunityFlair{}
	}
	return out, rows.Err()
}

// ListCommunityPosts returns social-native items for the trending feed.
func (s *Store) ListCommunityPosts(ctx context.Context, sort, category, flair string, since time.Time, limit int) ([]CommunityPost, error) {
	if limit <= 0 || limit > 50 {
		limit = 30
	}
	sort = strings.ToLower(strings.TrimSpace(sort))
	if sort != "new" {
		sort = "hot"
	}
	category = strings.ToLower(strings.TrimSpace(category))
	flair = strings.TrimSpace(flair)
	sources := communitySources(category)
	orderBy := `(COALESCE((si.metrics->>'score')::double precision, 0)) DESC,
		COALESCE(si.created_at_src, si.ingested_at) DESC,
		si.id DESC`
	if sort == "new" {
		orderBy = `COALESCE(si.created_at_src, si.ingested_at) DESC, si.id DESC`
	}
	q := fmt.Sprintf(`
		SELECT si.id, si.external_id, si.text, si.url, si.source, si.metrics,
		       si.created_at_src, COALESCE(e.twitch_login, '') AS twitch_login,
		       COALESCE(e.display_name, '') AS display_name,
		       COALESCE(th.thumbnail_url, si.metrics->>'thumbnail_url', '') AS thumbnail_url,
		       COALESCE(th.preview_title, '') AS preview_title,
		       COALESCE(th.embed_url, '') AS th_embed_url,
		       COALESCE(th.embed_html, '') AS th_embed_html,
		       COALESCE(th.platform, '') AS th_platform,
		       COALESCE(th.preview_status, '') AS preview_status,
		       COALESCE(c.category, '') AS category,
		       COALESCE(link_pv.embed_url, '') AS link_embed_url,
		       COALESCE(link_pv.embed_html, '') AS link_embed_html,
		       COALESCE(link_pv.thumbnail_url, '') AS link_thumb,
		       COALESCE(link_pv.platform, '') AS link_platform
		FROM social_items si
		LEFT JOIN streamer_entities e ON e.id = si.entity_id
		LEFT JOIN LATERAL (
			SELECT cl.category
			FROM story_evidence ev
			JOIN story_clusters cl ON cl.id = ev.cluster_id
			WHERE ev.item_id = si.id
			ORDER BY ev.id DESC
			LIMIT 1
		) c ON true
		LEFT JOIN LATERAL (
			SELECT p.thumbnail_url,
			       p.title AS preview_title,
			       COALESCE(p.embed_url, '') AS embed_url,
			       COALESCE(p.embed_html, '') AS embed_html,
			       COALESCE(p.platform, '') AS platform,
			       CASE
			         WHEN COALESCE(p.error, '') <> '' THEN 'error'
			         WHEN COALESCE(p.embed_url, '') <> '' OR COALESCE(p.embed_html, '') <> '' OR COALESCE(p.thumbnail_url, '') <> '' OR COALESCE(p.title, '') <> '' THEN 'ready'
			         ELSE 'fallback'
			       END AS preview_status
			FROM story_evidence ev
			JOIN story_evidence_previews sep ON sep.evidence_id = ev.id
			JOIN evidence_previews p ON p.id = sep.preview_id
			WHERE ev.item_id = si.id
			ORDER BY sep.created_at DESC, ev.id DESC
			LIMIT 1
		) th ON true
		LEFT JOIN LATERAL (
			SELECT COALESCE(p.embed_url, '') AS embed_url,
			       COALESCE(p.embed_html, '') AS embed_html,
			       COALESCE(p.thumbnail_url, '') AS thumbnail_url,
			       COALESCE(p.platform, '') AS platform
			FROM story_evidence ev
			JOIN story_evidence_previews sep ON sep.evidence_id = ev.id
			JOIN evidence_previews p ON p.id = sep.preview_id
			WHERE ev.item_id = si.id
			  AND p.platform IN ('tiktok', 'x', 'youtube', 'twitch_clip')
			  AND (
			    COALESCE(p.embed_html, '') <> ''
			    OR COALESCE(p.embed_url, '') <> ''
			    OR COALESCE(p.thumbnail_url, '') <> ''
			  )
			ORDER BY
			  CASE p.platform
			    WHEN 'tiktok' THEN 1
			    WHEN 'x' THEN 2
			    WHEN 'youtube' THEN 3
			    ELSE 4
			  END,
			  sep.created_at DESC,
			  ev.id DESC
			LIMIT 1
		) link_pv ON true
		WHERE si.source = ANY($1)
		  AND si.kind = 'post'
		  AND COALESCE(si.created_at_src, si.ingested_at) >= $2
		  AND ($3 = '' OR LOWER(TRIM(COALESCE(si.metrics->>'flair', ''))) = LOWER($3))
		ORDER BY %s
		LIMIT $4`, orderBy)
	fetchLimit := limit
	if category != "" {
		fetchLimit = limit * 4
		if fetchLimit > 120 {
			fetchLimit = 120
		}
	}
	rows, err := s.pool.Query(ctx, q, sources, since, flair, fetchLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CommunityPost
	for rows.Next() {
		var post CommunityPost
		var text string
		var metricsRaw []byte
		var thumb, previewTitle, previewStatus, cat string
		var thEmbedURL, thEmbedHTML, thPlatform string
		var linkEmbedURL, linkEmbedHTML, linkThumb, linkPlatform string
		if err := rows.Scan(
			&post.ID, &post.ExternalID, &text, &post.URL, &post.Source, &metricsRaw,
			&post.PostedAt, &post.StreamerLogin, &post.StreamerDisplayName,
			&thumb, &previewTitle, &thEmbedURL, &thEmbedHTML, &thPlatform, &previewStatus, &cat,
			&linkEmbedURL, &linkEmbedHTML, &linkThumb, &linkPlatform,
		); err != nil {
			return nil, err
		}
		previewTitle = strings.TrimSpace(previewTitle)
		previewTitles := []string{}
		if previewTitle != "" {
			previewTitles = append(previewTitles, previewTitle)
		}
		post.Title = ResolveDisplayTitle("", text, previewTitles, post.URL)
		if post.Title == "" {
			continue
		}
		post.Score = parseMetricFloat(metricsRaw, "score")
		post.Comments = parseMetricFloat(metricsRaw, "comments")
		if post.Score == 0 || post.Comments == 0 {
			if score, comments, cleaned := parseVoteCommentFromTitle(text); score > 0 || comments > 0 {
				if post.Score == 0 && score > 0 {
					post.Score = score
				}
				if post.Comments == 0 && comments > 0 {
					post.Comments = comments
				}
				if cleanedTitle := ResolveDisplayTitle("", cleaned, previewTitles, post.URL); cleanedTitle != "" && !IsPlaceholderTitle(cleanedTitle) {
					post.Title = cleanedTitle
				}
			}
		}
		post.SelfText = parseMetricString(metricsRaw, "selftext")
		externalURL := parseMetricString(metricsRaw, "external_url")
		post.ThumbnailURL = strings.TrimSpace(thumb)
		metricsThumb := parseMetricString(metricsRaw, "thumbnail_url")
		previewText := firstNonEmpty(externalURL, text)
		kind, raw, proxied := communityPreviewKind(previewStatus, post.ThumbnailURL, metricsThumb, previewText, post.URL)
		if kind == "none" && strings.TrimSpace(linkThumb) != "" {
			if lk, lr, lp := classifyThumb(linkThumb); lk != "none" {
				kind, raw, proxied = lk, lr, lp
			}
		}
		post.PreviewKind = kind
		post.PreviewURL = proxied
		if kind != "none" {
			post.ThumbnailURL = raw
			post.DisplayThumbnailURL = firstNonEmpty(proxied, displayThumbnailURL(post.ThumbnailURL, metricsThumb))
		} else {
			post.ThumbnailURL = ""
			post.PreviewKind = "none"
			post.DisplayThumbnailURL = ""
		}
		if linkEmbedHTML != "" || linkEmbedURL != "" || linkPlatform != "" {
			post.LinkedPlatform = strings.TrimSpace(linkPlatform)
			post.EmbedURL = strings.TrimSpace(linkEmbedURL)
			post.EmbedHTML = strings.TrimSpace(linkEmbedHTML)
		} else if thEmbedHTML != "" || thEmbedURL != "" {
			post.LinkedPlatform = strings.TrimSpace(thPlatform)
			post.EmbedURL = strings.TrimSpace(thEmbedURL)
			post.EmbedHTML = strings.TrimSpace(thEmbedHTML)
		}
		post.Category = strings.TrimSpace(cat)
		post.Flair = parseMetricString(metricsRaw, "flair")
		if pulseCat := parseMetricString(metricsRaw, "pulse_category"); pulseCat != "" {
			post.Category = pulseCat
		}
		if post.Source == "streamerbans_post" {
			post.Category = "bans"
			post.Subreddit = "StreamerBans"
		} else {
			post.Subreddit = redditSubreddit(post.URL)
			post.URL = reddit.CanonicalPermalink(post.URL)
		}
		post.Permalink = post.URL
		out = append(out, post)
	}
	if out == nil {
		out = []CommunityPost{}
	}
	return out, rows.Err()
}

// ListTopClips returns top Twitch clips by view count in the window.
func (s *Store) ListTopClips(ctx context.Context, since time.Time, limit int) ([]TopClip, error) {
	if limit <= 0 || limit > 24 {
		limit = 12
	}
	rows, err := s.pool.Query(ctx, `
		SELECT si.id, si.external_id, si.text, si.url, si.metrics, si.created_at_src,
		       COALESCE(e.twitch_login, '') AS twitch_login,
		       COALESCE(e.display_name, '') AS display_name,
		       COALESCE(NULLIF(th.thumbnail_url, ''), NULLIF(si.metrics->>'thumbnail_url', ''), '') AS thumbnail_url
		FROM social_items si
		LEFT JOIN streamer_entities e ON e.id = si.entity_id
		LEFT JOIN LATERAL (
			SELECT p.thumbnail_url
			FROM story_evidence ev
			JOIN story_evidence_previews sep ON sep.evidence_id = ev.id
			JOIN evidence_previews p ON p.id = sep.preview_id
			WHERE ev.item_id = si.id
			  AND COALESCE(p.thumbnail_url, '') <> ''
			ORDER BY sep.created_at DESC, ev.id DESC
			LIMIT 1
		) th ON true
		WHERE si.source = 'twitch_clip'
		  AND si.kind = 'clip'
		  AND COALESCE(si.created_at_src, si.ingested_at) >= $1
		ORDER BY (COALESCE((si.metrics->>'views')::double precision, 0)) DESC,
		         COALESCE(si.created_at_src, si.ingested_at) DESC,
		         si.id DESC
		LIMIT $2`, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TopClip
	for rows.Next() {
		var clip TopClip
		var text string
		var metricsRaw []byte
		var thumb string
		if err := rows.Scan(
			&clip.ID, &clip.ExternalID, &text, &clip.URL, &metricsRaw,
			&clip.PostedAt, &clip.StreamerLogin, &clip.StreamerDisplayName, &thumb,
		); err != nil {
			return nil, err
		}
		clip.Title = socialItemHeadline(text)
		if clip.Title == "" {
			continue
		}
		clip.ViewCount = parseMetricFloat(metricsRaw, "views")
		clip.DurationSeconds = parseMetricFloat(metricsRaw, "duration")
		metricsThumb := parseMetricString(metricsRaw, "thumbnail_url")
		clip.ThumbnailURL = preferServingThumb(metricsThumb, thumb)
		clip.DisplayThumbnailURL = displayThumbnailURL(thumb, metricsThumb)
		out = append(out, clip)
	}
	return out, rows.Err()
}

// ListUnlinkedEvidence returns in-window social items not yet visible on the Wire feed.
func (s *Store) ListUnlinkedEvidence(ctx context.Context, since time.Time, limit int) ([]UnlinkedEvidence, error) {
	if limit <= 0 || limit > 40 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx, `
		SELECT si.id, si.text, si.url, si.source, si.metrics, si.created_at_src,
		       COALESCE(e.twitch_login, ''), COALESCE(e.display_name, ''),
		       COALESCE(th.thumbnail_url, si.metrics->>'thumbnail_url', ''),
		       COALESCE(c.category, ''), COALESCE(c.title, '')
		FROM social_items si
		LEFT JOIN streamer_entities e ON e.id = si.entity_id
		LEFT JOIN story_evidence ev ON ev.item_id = si.id
		LEFT JOIN story_clusters c ON c.id = ev.cluster_id
		LEFT JOIN LATERAL (
			SELECT p.thumbnail_url
			FROM story_evidence ev2
			JOIN story_evidence_previews sep ON sep.evidence_id = ev2.id
			JOIN evidence_previews p ON p.id = sep.preview_id
			WHERE ev2.item_id = si.id
			  AND COALESCE(p.thumbnail_url, '') <> ''
			ORDER BY sep.created_at DESC, ev2.id DESC
			LIMIT 1
		) th ON true
		WHERE si.kind IN ('post', 'clip')
		  AND COALESCE(si.created_at_src, si.ingested_at) >= $1
		  AND (
		    ev.id IS NULL
		    OR COALESCE(c.title, '') = 'Story developing'
		    OR NOT EXISTS (
		      SELECT 1 FROM story_evidence evw
		      WHERE evw.cluster_id = c.id
		        AND COALESCE(evw.occurred_at, evw.created_at) >= $1
		    )
		  )
		ORDER BY (COALESCE((si.metrics->>'score')::double precision, (si.metrics->>'views')::double precision, 0)) DESC,
		         COALESCE(si.created_at_src, si.ingested_at) DESC,
		         si.id DESC
		LIMIT $2`, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UnlinkedEvidence
	for rows.Next() {
		var row UnlinkedEvidence
		var text string
		var metricsRaw []byte
		var thumb, cat string
		var clusterTitle string
		if err := rows.Scan(
			&row.ID, &text, &row.URL, &row.Source, &metricsRaw, &row.PostedAt,
			&row.StreamerLogin, &row.StreamerDisplayName, &thumb, &cat, &clusterTitle,
		); err != nil {
			return nil, err
		}
		row.Title = socialItemHeadline(text)
		if row.Title == "" {
			continue
		}
		row.Score = parseMetricFloat(metricsRaw, "score")
		row.Comments = parseMetricFloat(metricsRaw, "comments")
		row.ViewCount = parseMetricFloat(metricsRaw, "views")
		row.Category = strings.TrimSpace(cat)
		if pulseCat := parseMetricString(metricsRaw, "pulse_category"); pulseCat != "" {
			row.Category = pulseCat
		}
		kind, raw, proxied := resolvePreview(thumb, parseMetricString(metricsRaw, "thumbnail_url"), text, row.URL)
		row.PreviewKind = kind
		row.PreviewURL = proxied
		metricsThumb := parseMetricString(metricsRaw, "thumbnail_url")
		if kind != "none" {
			row.ThumbnailURL = raw
			row.DisplayThumbnailURL = firstNonEmpty(proxied, displayThumbnailURL(raw, metricsThumb))
		} else {
			row.DisplayThumbnailURL = ""
		}
		out = append(out, row)
	}
	if out == nil {
		out = []UnlinkedEvidence{}
	}
	return out, rows.Err()
}

// BackfillRedditCanonicalURLs rewrites old.reddit.com permalinks to www.reddit.com.
func (s *Store) BackfillRedditCanonicalURLs(ctx context.Context, limit int) (int, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE social_items
		SET url = regexp_replace(url, '^https://old\\.reddit\\.com', 'https://www.reddit.com')
		WHERE id IN (
			SELECT id FROM social_items
			WHERE source = 'reddit'
			  AND url LIKE 'https://old.reddit.com%'
			ORDER BY id DESC
			LIMIT $1
		)`, limit)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// CountRedditZeroScoreMetrics counts reddit social_items still storing zero score.
func (s *Store) CountRedditZeroScoreMetrics(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM social_items
		WHERE source = 'reddit'
		  AND COALESCE(metrics->>'score', '0') IN ('0', '0.0', '')`).Scan(&n)
	return n, err
}

// ListClustersMissingEntity returns recent clusters without a linked streamer entity.
func (s *Store) ListClustersMissingEntity(ctx context.Context, limit int) ([]struct {
	ID    int64
	Title string
}, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, COALESCE(title, '')
		FROM story_clusters
		WHERE entity_id IS NULL
		  AND COALESCE(title, '') <> ''
		  AND COALESCE(title, '') <> 'Story developing'
		  AND state IN ('published', 'developing', 'unverified', 'settled')
		  AND updated_at >= now() - interval '7 days'
		ORDER BY updated_at DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []struct {
		ID    int64
		Title string
	}
	for rows.Next() {
		var row struct {
			ID    int64
			Title string
		}
		if err := rows.Scan(&row.ID, &row.Title); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// BackfillPlaceholderRedditSocialText rewrites comment-count placeholders using the Reddit permalink slug.
func (s *Store) BackfillPlaceholderRedditSocialText(ctx context.Context, limit int) (int, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE social_items si
		SET text = initcap(replace((regexp_match(si.url, '/comments/[^/]+/([^/?#]+)'))[1], '_', ' '))
		WHERE si.source = 'reddit'
		  AND COALESCE(si.text, '') ~ '^[0-9]+\\s+comments?$'
		  AND si.url ~ '/comments/[^/]+/[^/?#]+'
		  AND si.id IN (
		    SELECT id FROM social_items
		    WHERE source = 'reddit'
		      AND COALESCE(text, '') ~ '^[0-9]+\\s+comments?$'
		    ORDER BY id DESC
		    LIMIT $1
		  )`, limit)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// BackfillPlaceholderClusterTitles replaces placeholder cluster titles with the best linked evidence title.
func (s *Store) BackfillPlaceholderClusterTitles(ctx context.Context, limit int) (int, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	tag, err := s.pool.Exec(ctx, `
		WITH picked AS (
			SELECT DISTINCT ON (c.id) c.id AS cluster_id,
			       COALESCE(
			         NULLIF(TRIM(p.title), ''),
			         NULLIF(TRIM(SPLIT_PART(
			           REGEXP_REPLACE(COALESCE(si.text, ''), 'https?://\\S+', '', 'g'),
			           E'\\n', 1
			         )), ''),
			         initcap(replace((regexp_match(si.url, '/comments/[^/]+/([^/?#]+)'))[1], '_', ' '))
			       ) AS headline
			FROM story_clusters c
			JOIN story_evidence ev ON ev.cluster_id = c.id
			JOIN social_items si ON si.id = ev.item_id
			LEFT JOIN story_evidence_previews sep ON sep.evidence_id = ev.id
			LEFT JOIN evidence_previews p ON p.id = sep.preview_id
			WHERE (
			    COALESCE(c.title, '') = 'Story developing'
			    OR COALESCE(c.title, '') ~ '^[0-9]+\\s+comments?$'
			  )
			  AND COALESCE(si.text, '') <> ''
			ORDER BY c.id, length(COALESCE(p.title, si.text)) DESC, ev.id DESC
			LIMIT $1
		)
		UPDATE story_clusters c
		SET title = picked.headline,
		    summary = COALESCE(NULLIF(c.summary, ''), 'Wire-native social evidence grouped from global source ingest.'),
		    updated_at = now()
		FROM picked
		WHERE c.id = picked.cluster_id
		  AND picked.headline <> ''
		  AND picked.headline <> 'Story developing'
		  AND picked.headline !~ '^[0-9]+\\s+comments?$'`, limit)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}
