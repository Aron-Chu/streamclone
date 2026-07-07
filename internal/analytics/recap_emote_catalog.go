package analytics

import (
	"context"
	"strings"
)

func recapEmoteCodes(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(names))
	out := make([]string, 0, len(names))
	for _, code := range names {
		key := strings.ToLower(strings.TrimSpace(code))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

func mergeRecapEmoteCatalogs(catalogs ...[]TopEmote) []TopEmote {
	byName := make(map[string]TopEmote)
	for _, catalog := range catalogs {
		for _, item := range catalog {
			key := strings.ToLower(strings.TrimSpace(item.Name))
			if key == "" {
				continue
			}
			prev, ok := byName[key]
			if !ok || recapTopEmoteCatalogBetter(item, prev) {
				byName[key] = item
			}
		}
	}
	out := make([]TopEmote, 0, len(byName))
	for _, item := range byName {
		out = append(out, item)
	}
	return out
}

func recapTopEmoteCatalogBetter(candidate, current TopEmote) bool {
	candidateHas := recapTopEmoteHasMetadata(candidate)
	currentHas := recapTopEmoteHasMetadata(current)
	if candidateHas != currentHas {
		return candidateHas
	}
	return candidate.Count > current.Count
}

func recapTopEmoteHasMetadata(item TopEmote) bool {
	return strings.TrimSpace(item.ImageURL) != "" || strings.TrimSpace(item.ID) != ""
}

func topEmoteFromRecapIdentity(provider, providerEmoteID, name, localEmoteID string, count int) TopEmote {
	provider = normalizeProvider(provider)
	name = strings.TrimSpace(name)
	providerEmoteID = strings.TrimSpace(providerEmoteID)
	localEmoteID = strings.TrimSpace(localEmoteID)
	id := localEmoteID
	if id == "" {
		id = providerEmoteID
	}
	key := strings.Join([]string{provider, id, name}, ":")
	return TopEmote{
		Key:      key,
		Name:     name,
		ID:       id,
		Provider: provider,
		ImageURL: emoteImageURL(provider, id),
		Count:    count,
	}
}

func (s *Store) RecapEmoteCatalogFromStreamHistory(ctx context.Context, streamID string, codes []string) ([]TopEmote, error) {
	if s == nil || s.db == nil || strings.TrimSpace(streamID) == "" || len(codes) == 0 {
		return nil, nil
	}
	rows, err := s.db.Query(ctx, `
		SELECT DISTINCT ON (lower(emote_name))
			provider, provider_emote_id, emote_name, COALESCE(local_emote_id, ''), use_count
		FROM emote_usage_stream_rollups
		WHERE stream_id = $1
		  AND lower(emote_name) = ANY($2)
		  AND provider IN ('seventv', '7tv')
		  AND identity_resolution IN ('provider_id', 'alias_fallback')
		ORDER BY lower(emote_name), use_count DESC`, streamID, codes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]TopEmote, 0, len(codes))
	for rows.Next() {
		var provider, providerEmoteID, name, localEmoteID string
		var useCount int64
		if err := rows.Scan(&provider, &providerEmoteID, &name, &localEmoteID, &useCount); err != nil {
			return nil, err
		}
		out = append(out, topEmoteFromRecapIdentity(provider, providerEmoteID, name, localEmoteID, int(useCount)))
	}
	return out, rows.Err()
}

func (s *Store) RecapEmoteCatalogFromChannelSnapshots(ctx context.Context, twitchID string, codes []string) ([]TopEmote, error) {
	if s == nil || s.db == nil || strings.TrimSpace(twitchID) == "" || len(codes) == 0 {
		return nil, nil
	}
	rows, err := s.db.Query(ctx, `
		SELECT DISTINCT ON (lower(i.alias))
			i.provider, i.provider_emote_id, i.alias, COALESCE(e.id::text, '')
		FROM channel_emote_set_snapshots s
		JOIN channel_emote_set_snapshot_items i ON i.snapshot_id = s.id
		LEFT JOIN emotes e ON e.provider = i.provider AND e.provider_emote_id = i.provider_emote_id
		WHERE s.twitch_id = $1
		  AND s.provider IN ('seventv', '7tv')
		  AND s.state = 'complete'
		  AND lower(i.alias) = ANY($2)
		ORDER BY lower(i.alias), s.fetched_at DESC`, twitchID, codes)
	if err != nil {
		if errorsIsMissingRelation(err) {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	out := make([]TopEmote, 0, len(codes))
	for rows.Next() {
		var provider, providerEmoteID, alias, localEmoteID string
		if err := rows.Scan(&provider, &providerEmoteID, &alias, &localEmoteID); err != nil {
			return nil, err
		}
		out = append(out, topEmoteFromRecapIdentity(provider, providerEmoteID, alias, localEmoteID, 0))
	}
	return out, rows.Err()
}

func errorsIsMissingRelation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "does not exist") || strings.Contains(msg, "undefined_table")
}
