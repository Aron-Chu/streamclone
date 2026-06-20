package store

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"
)

func mentionsLogin(text, login string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	login = strings.ToLower(strings.TrimSpace(login))
	if text == "" || login == "" {
		return false
	}
	if text == login {
		return true
	}
	compact := strings.ReplaceAll(text, " ", "")
	if compact == login || strings.Contains(compact, login) {
		return true
	}
	for _, field := range strings.FieldsFunc(text, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_'
	}) {
		if field == login {
			return true
		}
	}
	return strings.Contains(text, login)
}

func spreadRankScore(card StoryCard) int {
	score := 0
	if card.WindowScores != nil {
		score += card.WindowScores.SourceCount * 100
		if card.WindowScores.CredibilityScore >= 0.75 {
			score += 80
		} else if card.WindowScores.CredibilityScore >= 0.45 {
			score += 40
		}
		if !card.WindowScores.ComputedAt.IsZero() {
			age := time.Since(card.WindowScores.ComputedAt)
			if age < 6*time.Hour {
				score += 20
			} else if age < 24*time.Hour {
				score += 10
			}
		}
	}
	if card.Scores.Confidence != nil {
		switch strings.ToLower(strings.TrimSpace(*card.Scores.Confidence)) {
		case "corroborated", "high":
			score += 60
		case "developing", "medium":
			score += 30
		}
	}
	if len(card.Receipts) > 0 {
		score += len(card.Receipts) * 10
	}
	return score
}

func EntityDisplayAliases(ent *Entity) []string {
	if ent == nil {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		key := strings.ToLower(v)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, v)
	}
	add(ent.DisplayName)
	add(ent.TwitchLogin)
	if len(ent.Aliases) > 0 {
		var aliases []struct {
			Platform string `json:"platform"`
			Handle   string `json:"handle"`
		}
		if json.Unmarshal(ent.Aliases, &aliases) == nil {
			for _, alias := range aliases {
				add(alias.Handle)
			}
		}
	}
	return out
}

func (s *Store) SpreadForLogin(ctx context.Context, login string, limit int) ([]StoryCard, error) {
	ent, err := s.EntityByLogin(ctx, login)
	if err != nil || ent == nil {
		return []StoryCard{}, err
	}
	if limit <= 0 {
		limit = 10
	}
	fetchLimit := limit * 3
	if fetchLimit < 15 {
		fetchLimit = 15
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id FROM story_clusters
		WHERE entity_id = $1 AND state IN ('published', 'developing', 'unverified', 'settled')
		ORDER BY updated_at DESC LIMIT $2`, ent.ID, fetchLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cards []StoryCard
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		card, err := s.GetStory(ctx, id, "local")
		if err != nil || card == nil {
			continue
		}
		cards = append(cards, *card)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(cards, func(i, j int) bool {
		left := spreadRankScore(cards[i])
		right := spreadRankScore(cards[j])
		if left != right {
			return left > right
		}
		return cards[i].Cluster.UpdatedAt.After(cards[j].Cluster.UpdatedAt)
	})
	if len(cards) > limit {
		cards = cards[:limit]
	}
	return cards, nil
}

func (s *Store) SpreadProbableForLogin(ctx context.Context, login string, exclude map[int64]struct{}, limit int) ([]StoryCard, error) {
	if limit <= 0 {
		limit = 3
	}
	login = strings.ToLower(strings.TrimSpace(login))
	if login == "" {
		return nil, nil
	}
	ent, _ := s.EntityByLogin(ctx, login)
	var entityID int64
	aliases := []string{login}
	if ent != nil {
		entityID = ent.ID
		aliases = EntityDisplayAliases(ent)
	}
	since := time.Now().Add(-7 * 24 * time.Hour)
	rows, err := s.pool.Query(ctx, `
		SELECT id, COALESCE(title, ''), COALESCE(category, '')
		FROM story_clusters
		WHERE state IN ('published', 'developing', 'unverified', 'settled')
		  AND updated_at >= $1
		  AND ($2 = 0 OR entity_id IS NULL OR entity_id <> $2)
		ORDER BY updated_at DESC
		LIMIT 80`, since, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StoryCard
	for rows.Next() {
		if len(out) >= limit {
			break
		}
		var id int64
		var title, category string
		if err := rows.Scan(&id, &title, &category); err != nil {
			return nil, err
		}
		if exclude != nil {
			if _, skip := exclude[id]; skip {
				continue
			}
		}
		haystack := strings.TrimSpace(title + " " + category)
		matched := false
		for _, alias := range aliases {
			if mentionsLogin(haystack, alias) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		card, err := s.GetStory(ctx, id, "local")
		if err != nil || card == nil {
			continue
		}
		out = append(out, *card)
	}
	return out, rows.Err()
}

func (s *Store) CountUnresolvedMentionsForLogin(ctx context.Context, login string) (int, error) {
	login = strings.ToLower(strings.TrimSpace(login))
	if login == "" {
		return 0, nil
	}
	since := time.Now().Add(-48 * time.Hour)
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM (
			SELECT 1
			FROM social_items
			WHERE entity_id IS NULL
			  AND ingested_at >= $1
			  AND LOWER(COALESCE(text, '')) LIKE '%' || $2 || '%'
			LIMIT 100
		) capped`, since, login).Scan(&count)
	return count, err
}

type UnresolvedSocialItem struct {
	ID           int64
	Source       string
	Kind         string
	ExternalID   string
	URL          string
	Author       string
	Text         string
	FlairText    string
	CreatedAtSrc *time.Time
}

func (s *Store) ListUnresolvedSocialItemsMentioningLogin(ctx context.Context, login string, limit int) ([]UnresolvedSocialItem, error) {
	login = strings.ToLower(strings.TrimSpace(login))
	if login == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}
	since := time.Now().Add(-48 * time.Hour)
	rows, err := s.pool.Query(ctx, `
		SELECT id, source, kind, COALESCE(external_id, ''), COALESCE(url, ''),
		       COALESCE(author, ''), COALESCE(text, ''), created_at_src
		FROM social_items
		WHERE entity_id IS NULL
		  AND ingested_at >= $1
		  AND LOWER(COALESCE(text, '')) LIKE '%' || $2 || '%'
		ORDER BY ingested_at DESC
		LIMIT $3`, since, login, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UnresolvedSocialItem
	for rows.Next() {
		var row UnresolvedSocialItem
		if err := rows.Scan(&row.ID, &row.Source, &row.Kind, &row.ExternalID, &row.URL, &row.Author, &row.Text, &row.CreatedAtSrc); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

type EntityAlias struct {
	Platform string `json:"platform"`
	Handle   string `json:"handle"`
}

func ParseEntityAliases(raw json.RawMessage) []EntityAlias {
	if len(raw) == 0 {
		return nil
	}
	var aliases []EntityAlias
	if json.Unmarshal(raw, &aliases) != nil {
		return nil
	}
	return aliases
}

func (s *Store) MergeEntityAliases(ctx context.Context, entityID int64, platform, handle string) error {
	platform = strings.TrimSpace(platform)
	handle = strings.TrimSpace(handle)
	if entityID <= 0 || platform == "" || handle == "" {
		return nil
	}
	ent, err := s.entityByID(ctx, entityID)
	if err != nil || ent == nil {
		return err
	}
	aliases := ParseEntityAliases(ent.Aliases)
	for _, alias := range aliases {
		if strings.EqualFold(alias.Platform, platform) && strings.EqualFold(alias.Handle, handle) {
			return nil
		}
	}
	aliases = append(aliases, EntityAlias{Platform: platform, Handle: handle})
	merged, err := json.Marshal(aliases)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE streamer_entities SET aliases = $2, updated_at = now() WHERE id = $1`, entityID, merged)
	return err
}

func (s *Store) AttachSocialItemEntity(ctx context.Context, itemID int64, entityID int64) error {
	if itemID <= 0 || entityID <= 0 {
		return nil
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE social_items
		SET entity_id = $2
		WHERE id = $1 AND entity_id IS NULL`, itemID, entityID)
	return err
}

func (s *Store) EntityIDByAlias(ctx context.Context, platform, handle string) (*int64, error) {
	platform = strings.ToLower(strings.TrimSpace(platform))
	handle = strings.ToLower(strings.TrimSpace(handle))
	if platform == "" || handle == "" {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id
		FROM streamer_entities
		WHERE EXISTS (
			SELECT 1
			FROM jsonb_array_elements(COALESCE(aliases, '[]'::jsonb)) elem
			WHERE lower(elem->>'platform') = $1 AND lower(elem->>'handle') = $2
		)`, platform, handle)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) != 1 {
		return nil, nil
	}
	return &ids[0], nil
}
