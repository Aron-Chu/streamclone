package store

import (
	"context"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
)

var placeholderTitleRE = regexp.MustCompile(`(?i)^\d+\s+comments?$`)

// IsPlaceholderTitle reports Reddit-style comment-count placeholders and empty titles.
func IsPlaceholderTitle(title string) bool {
	title = strings.TrimSpace(title)
	if title == "" || title == "Story developing" {
		return true
	}
	return placeholderTitleRE.MatchString(title)
}

// TitleFromPermalinkSlug derives a readable headline from a Reddit permalink slug.
func TitleFromPermalinkSlug(permalink string) string {
	parts := strings.Split(strings.Trim(strings.TrimSpace(permalink), "/"), "/")
	for i, part := range parts {
		if strings.EqualFold(part, "comments") && i+2 < len(parts) {
			slug := strings.TrimSpace(parts[i+2])
			if slug == "" {
				return ""
			}
			return strings.Join(strings.Fields(strings.ReplaceAll(slug, "_", " ")), " ")
		}
	}
	return ""
}

// CleanSocialHeadline strips URLs and normalizes social item text for display.
func CleanSocialHeadline(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if idx := strings.Index(text, "https://"); idx > 0 {
		text = strings.TrimSpace(text[:idx])
	}
	if idx := strings.Index(text, "http://"); idx > 0 {
		text = strings.TrimSpace(text[:idx])
	}
	text = strings.Join(strings.Fields(text), " ")
	if len(text) > 220 {
		text = strings.TrimSpace(text[:220])
	}
	return text
}

// Priority: evidence preview title > cleaned social text > permalink slug > cluster title.
func ResolveDisplayTitle(clusterTitle, socialText string, previewTitles []string, permalink string) string {
	for _, previewTitle := range previewTitles {
		previewTitle = CleanSocialHeadline(previewTitle)
		if previewTitle != "" && !IsPlaceholderTitle(previewTitle) {
			return previewTitle
		}
	}
	if cleaned := CleanSocialHeadline(socialText); cleaned != "" && !IsPlaceholderTitle(cleaned) {
		return cleaned
	}
	if slug := TitleFromPermalinkSlug(permalink); slug != "" && !IsPlaceholderTitle(slug) {
		return slug
	}
	clusterTitle = CleanSocialHeadline(clusterTitle)
	if clusterTitle != "" && !IsPlaceholderTitle(clusterTitle) {
		return clusterTitle
	}
	return clusterTitle
}

type displayTitleInput struct {
	ClusterTitle  string
	SocialText    string
	Permalink     string
	PreviewTitles []string
}

// EnrichStoryDisplayTitles rewrites placeholder cluster titles using linked evidence.
func (s *Store) EnrichStoryDisplayTitles(ctx context.Context, cards []StoryCard) error {
	if len(cards) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(cards))
	for _, card := range cards {
		if card.Cluster.ID > 0 && IsPlaceholderTitle(card.Cluster.Title) {
			ids = append(ids, card.Cluster.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	inputs, err := s.displayTitleInputs(ctx, ids)
	if err != nil {
		return err
	}
	for i := range cards {
		input, ok := inputs[cards[i].Cluster.ID]
		if !ok {
			continue
		}
		resolved := ResolveDisplayTitle(cards[i].Cluster.Title, input.SocialText, input.PreviewTitles, input.Permalink)
		if resolved != "" && resolved != cards[i].Cluster.Title {
			cards[i].Cluster.Title = resolved
		}
	}
	return nil
}

func (s *Store) displayTitleInputs(ctx context.Context, clusterIDs []int64) (map[int64]displayTitleInput, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT c.id,
		       COALESCE(c.title, ''),
		       COALESCE(social.text, ''),
		       COALESCE(social.url, ''),
		       COALESCE(previews.titles, ARRAY[]::text[])
		FROM story_clusters c
		LEFT JOIN LATERAL (
			SELECT si.text, si.url
			FROM story_evidence ev
			JOIN social_items si ON si.id = ev.item_id
			WHERE ev.cluster_id = c.id
			  AND si.source IN ('reddit', 'streamerbans_post')
			ORDER BY length(COALESCE(si.text, '')) DESC, ev.id DESC
			LIMIT 1
		) social ON true
		LEFT JOIN LATERAL (
			SELECT array_agg(DISTINCT p.title ORDER BY p.title) AS titles
			FROM story_evidence ev
			JOIN story_evidence_previews sep ON sep.evidence_id = ev.id
			JOIN evidence_previews p ON p.id = sep.preview_id
			WHERE ev.cluster_id = c.id
			  AND COALESCE(NULLIF(TRIM(p.title), ''), '') <> ''
		) previews ON true
		WHERE c.id = ANY($1)`, clusterIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int64]displayTitleInput, len(clusterIDs))
	for rows.Next() {
		var id int64
		var input displayTitleInput
		if err := rows.Scan(&id, &input.ClusterTitle, &input.SocialText, &input.Permalink, &input.PreviewTitles); err != nil {
			return nil, err
		}
		out[id] = input
	}
	return out, rows.Err()
}

// ApplyDisplayTitle resolves the headline on a single story card (GetStory path).
func (s *Store) ApplyDisplayTitle(ctx context.Context, card *StoryCard) error {
	if card == nil || card.Cluster.ID <= 0 {
		return nil
	}
	if !IsPlaceholderTitle(card.Cluster.Title) && strings.TrimSpace(card.Cluster.Title) != "" {
		return nil
	}
	inputs, err := s.displayTitleInputs(ctx, []int64{card.Cluster.ID})
	if err != nil {
		return err
	}
	input, ok := inputs[card.Cluster.ID]
	if !ok {
		return nil
	}
	var previewTitles []string
	for _, preview := range card.EvidenceGallery {
		if title := strings.TrimSpace(preview.Title); title != "" {
			previewTitles = append(previewTitles, title)
		}
	}
	if len(previewTitles) == 0 {
		previewTitles = input.PreviewTitles
	}
	resolved := ResolveDisplayTitle(card.Cluster.Title, input.SocialText, previewTitles, input.Permalink)
	if resolved != "" {
		card.Cluster.Title = resolved
	}
	return nil
}

// CountPlaceholderRedditSocialText returns rows still using comment-count placeholders.
func (s *Store) CountPlaceholderRedditSocialText(ctx context.Context) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM social_items
		WHERE source = 'reddit'
		  AND COALESCE(text, '') ~ '^[0-9]+\\s+comments?$'`).Scan(&count)
	if err == pgx.ErrNoRows {
		return 0, nil
	}
	return count, err
}
