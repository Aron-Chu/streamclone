package archive

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PgxEmoteSnapshotDB reads emote metadata from Postgres for cold export.
type PgxEmoteSnapshotDB struct {
	db *pgxpool.Pool
}

func NewPgxEmoteSnapshotDB(db *pgxpool.Pool) *PgxEmoteSnapshotDB {
	return &PgxEmoteSnapshotDB{db: db}
}

func (d *PgxEmoteSnapshotDB) ListProviderEmotes(ctx context.Context, login, provider string) ([]EmoteSnapshotLine, error) {
	if d == nil || d.db == nil {
		return nil, fmt.Errorf("emote snapshot db unavailable")
	}
	login = strings.ToLower(strings.TrimSpace(login))
	provider = strings.ToLower(strings.TrimSpace(provider))
	rows, err := d.db.Query(ctx, `
		SELECT e.id::text, COALESCE(e.provider_emote_id,''), e.name
		FROM channels c
		JOIN emote_set_items i ON i.emote_set_id = c.active_emote_set_id AND i.status = 1
		JOIN emotes e ON e.id = i.emote_id AND e.status = 1
		WHERE c.login = $1
		  AND COALESCE(e.provider, 'custom') = $2
		  AND e.is_global = false
		ORDER BY e.name ASC`, login, provider)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EmoteSnapshotLine
	for rows.Next() {
		var line EmoteSnapshotLine
		if err := rows.Scan(&line.EmoteID, &line.ProviderEmoteID, &line.Name); err != nil {
			return nil, err
		}
		line.Provider = provider
		out = append(out, line)
	}
	return out, rows.Err()
}

func (d *PgxEmoteSnapshotDB) ProviderSetSnapshot(ctx context.Context, login, provider string) (providerSetID, emoteHash string, count int, ok bool, err error) {
	if d == nil || d.db == nil {
		return "", "", 0, false, fmt.Errorf("emote snapshot db unavailable")
	}
	login = strings.ToLower(strings.TrimSpace(login))
	provider = strings.ToLower(strings.TrimSpace(provider))
	err = d.db.QueryRow(ctx, `
		WITH provider_items AS (
			SELECT COALESCE(e.provider_set_id, '') AS provider_set_id, COUNT(*)::int AS count
			FROM channels c
			JOIN emote_set_items i ON i.emote_set_id = c.active_emote_set_id AND i.status = 1
			JOIN emotes e ON e.id = i.emote_id
			WHERE c.login = $1 AND COALESCE(e.provider, 'custom') = $2 AND e.is_global = false
			GROUP BY COALESCE(e.provider_set_id, '')
			ORDER BY COUNT(*) DESC
			LIMIT 1
		),
		provider_ids AS (
			SELECT array_agg(e.provider_emote_id ORDER BY e.provider_emote_id COLLATE "C") AS ids
			FROM channels c
			JOIN emote_set_items i ON i.emote_set_id = c.active_emote_set_id AND i.status = 1
			JOIN emotes e ON e.id = i.emote_id
			WHERE c.login = $1 AND COALESCE(e.provider, 'custom') = $2 AND e.is_global = false
		)
		SELECT p.provider_set_id, p.count,
			COALESCE(encode(digest(array_to_string(ids, ','), 'sha256'), 'hex'), '')
		FROM provider_items p
		CROSS JOIN provider_ids`, login, provider,
	).Scan(&providerSetID, &count, &emoteHash)
	if err != nil {
		return "", "", 0, false, err
	}
	return providerSetID, emoteHash, count, count > 0 || providerSetID != "", nil
}

func (d *PgxEmoteSnapshotDB) ListSnapshotLogins(ctx context.Context, limit int) ([]string, error) {
	if d == nil || d.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 500
	}
	rows, err := d.db.Query(ctx, `
		SELECT login
		FROM (
			SELECT DISTINCT c.login, ts.last_rank
			FROM channels c
			LEFT JOIN tracked_streamers ts ON ts.login = c.login
			WHERE c.active_emote_set_id IS NOT NULL
		) q
		ORDER BY last_rank ASC NULLS LAST, login ASC
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
		if login = strings.ToLower(strings.TrimSpace(login)); login != "" {
			out = append(out, login)
		}
	}
	return out, rows.Err()
}

func (d *PgxEmoteSnapshotDB) ListAlwaysTrackedLogins(ctx context.Context) ([]string, error) {
	if d == nil || d.db == nil {
		return nil, nil
	}
	rows, err := d.db.Query(ctx, `
		SELECT login FROM tracked_streamers WHERE is_always_tracked = true ORDER BY login ASC`)
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
		if login = strings.ToLower(strings.TrimSpace(login)); login != "" {
			out = append(out, login)
		}
	}
	return out, rows.Err()
}
