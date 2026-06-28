package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Emote struct {
	ID              string
	Name            string
	OwnerID         string
	IsGlobal        bool
	Flags           int
	Animated        bool
	MimeType        string
	SourceHash      string
	Provider        string
	ProviderEmoteID string
	ProviderSetID   string
	SourceURL       string
	Status          int16
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type Channel struct {
	TwitchID         string
	Login            string
	DisplayName      string
	ActiveEmoteSetID *string
	UpdatedAt        time.Time
}

type Job struct {
	ID        int64
	EmoteID   string
	SourceKey string
	State     int16
	Attempts  int
	LastError *string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Store struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

func (s *Store) UpdateEmoteFlags(ctx context.Context, id string, flags int) error {
	_, err := s.db.Exec(ctx, `UPDATE emotes SET flags=$1, updated_at=now() WHERE id=$2::uuid`, flags, id)
	return err
}

func (s *Store) Ping(ctx context.Context) error {
	return s.db.Ping(ctx)
}

func (s *Store) UpsertEmote(ctx context.Context, e Emote) (string, error) {
	var id string
	var err error
	if e.ID != "" {
		err = s.db.QueryRow(ctx, `
			INSERT INTO emotes (id, name, owner_id, is_global, flags, animated, mime_type, source_hash, provider, provider_emote_id, provider_set_id, source_url, status)
			VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, ''), NULLIF($10, ''), NULLIF($11, ''), NULLIF($12, ''), $13)
			ON CONFLICT (id) DO UPDATE SET
				name=EXCLUDED.name, owner_id=EXCLUDED.owner_id, is_global=EXCLUDED.is_global,
				flags=EXCLUDED.flags, animated=EXCLUDED.animated, mime_type=EXCLUDED.mime_type,
				source_hash=EXCLUDED.source_hash, provider=EXCLUDED.provider,
				provider_emote_id=EXCLUDED.provider_emote_id, provider_set_id=EXCLUDED.provider_set_id,
				source_url=EXCLUDED.source_url, status=EXCLUDED.status, updated_at=now()
			RETURNING id`,
			e.ID, e.Name, e.OwnerID, e.IsGlobal, e.Flags, e.Animated, e.MimeType, e.SourceHash,
			e.Provider, e.ProviderEmoteID, e.ProviderSetID, e.SourceURL, e.Status,
		).Scan(&id)
	} else {
		err = s.db.QueryRow(ctx, `
			INSERT INTO emotes (name, owner_id, is_global, flags, animated, mime_type, source_hash, provider, provider_emote_id, provider_set_id, source_url, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''), NULLIF($9, ''), NULLIF($10, ''), NULLIF($11, ''), $12)
			RETURNING id`,
			e.Name, e.OwnerID, e.IsGlobal, e.Flags, e.Animated, e.MimeType, e.SourceHash,
			e.Provider, e.ProviderEmoteID, e.ProviderSetID, e.SourceURL, e.Status,
		).Scan(&id)
	}
	if err != nil {
		return "", err
	}
	return id, nil
}

func (s *Store) UpsertEmoteByHash(ctx context.Context, e Emote) (string, bool, error) {
	if e.Provider != "" && e.ProviderEmoteID != "" {
		existing, err := s.GetProviderEmote(ctx, e.Provider, e.ProviderEmoteID)
		if err == nil {
			return existing.ID, true, nil
		}
		if err != pgx.ErrNoRows {
			return "", false, err
		}
	}
	if e.SourceHash == "" {
		id, err := s.UpsertEmote(ctx, e)
		return id, false, err
	}
	if e.Provider != "" && e.ProviderEmoteID != "" {
		newID, err := s.UpsertEmote(ctx, e)
		return newID, false, err
	}
	var id string
	err := s.db.QueryRow(ctx, `SELECT id FROM emotes WHERE source_hash=$1 LIMIT 1`, e.SourceHash).Scan(&id)
	if err == nil {
		return id, true, nil
	}
	if err != pgx.ErrNoRows {
		return "", false, err
	}
	newID, err := s.UpsertEmote(ctx, e)
	return newID, false, err
}

func (s *Store) SetEmoteStatus(ctx context.Context, id string, status int16) error {
	_, err := s.db.Exec(ctx, `UPDATE emotes SET status=$1, updated_at=now() WHERE id=$2::uuid`, status, id)
	return err
}

func (s *Store) GetEmote(ctx context.Context, id string) (*Emote, error) {
	e := &Emote{}
	err := s.db.QueryRow(ctx, `
		SELECT id, name, COALESCE(owner_id,''), is_global, flags, animated, mime_type, COALESCE(source_hash,''),
			COALESCE(provider,''), COALESCE(provider_emote_id,''), COALESCE(provider_set_id,''), COALESCE(source_url,''),
			status, created_at, updated_at
		FROM emotes WHERE id=$1::uuid`, id,
	).Scan(&e.ID, &e.Name, &e.OwnerID, &e.IsGlobal, &e.Flags, &e.Animated, &e.MimeType, &e.SourceHash,
		&e.Provider, &e.ProviderEmoteID, &e.ProviderSetID, &e.SourceURL, &e.Status, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return e, nil
}

func (s *Store) GetProviderEmote(ctx context.Context, provider, providerEmoteID string) (*Emote, error) {
	e := &Emote{}
	err := s.db.QueryRow(ctx, `
		SELECT id, name, COALESCE(owner_id,''), is_global, flags, animated, mime_type, COALESCE(source_hash,''),
			COALESCE(provider,''), COALESCE(provider_emote_id,''), COALESCE(provider_set_id,''), COALESCE(source_url,''),
			status, created_at, updated_at
		FROM emotes WHERE provider=$1 AND provider_emote_id=$2 LIMIT 1`, provider, providerEmoteID,
	).Scan(&e.ID, &e.Name, &e.OwnerID, &e.IsGlobal, &e.Flags, &e.Animated, &e.MimeType, &e.SourceHash,
		&e.Provider, &e.ProviderEmoteID, &e.ProviderSetID, &e.SourceURL, &e.Status, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return e, nil
}

func (s *Store) InsertJob(ctx context.Context, emoteID, sourceKey string) (int64, error) {
	var id int64
	err := s.db.QueryRow(ctx, `
		INSERT INTO processing_jobs (emote_id, source_key, state) VALUES ($1::uuid, $2, 0)
		ON CONFLICT (emote_id, source_key) DO UPDATE SET updated_at=now()
		RETURNING id`,
		emoteID, sourceKey,
	).Scan(&id)
	return id, err
}

func (s *Store) UpsertEmoteSetByOwnerName(ctx context.Context, name, ownerID string) (string, error) {
	var id string
	err := s.db.QueryRow(ctx, `
		SELECT id FROM emote_sets WHERE name=$1 AND owner_id=$2 LIMIT 1`,
		name, ownerID,
	).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != pgx.ErrNoRows {
		return "", err
	}
	return s.CreateEmoteSet(ctx, name, ownerID)
}

func (s *Store) EmoteSetExistsByOwnerName(ctx context.Context, name, ownerID string) (bool, error) {
	var id string
	err := s.db.QueryRow(ctx, `
		SELECT id FROM emote_sets WHERE name=$1 AND owner_id=$2 LIMIT 1`,
		name, ownerID,
	).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *Store) ClaimJob(ctx context.Context) (*Job, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	j := &Job{}
	err = tx.QueryRow(ctx, `
		SELECT id, emote_id, source_key, state, attempts, last_error, created_at, updated_at
		FROM processing_jobs
		WHERE state IN (0, 2)
			OR (state=1 AND updated_at < now() - interval '2 minutes')
		ORDER BY
			CASE WHEN state=1 THEN 0 ELSE 1 END,
			id
		LIMIT 1
		FOR UPDATE SKIP LOCKED`,
	).Scan(&j.ID, &j.EmoteID, &j.SourceKey, &j.State, &j.Attempts, &j.LastError, &j.CreatedAt, &j.UpdatedAt)
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(ctx, `UPDATE processing_jobs SET state=1, attempts=attempts+1, updated_at=now() WHERE id=$1`, j.ID)
	if err != nil {
		return nil, err
	}
	return j, tx.Commit(ctx)
}

func (s *Store) FinishJob(ctx context.Context, id int64, success bool, lastErr string) error {
	state := int16(3)
	if !success {
		state = 4
	}
	var e *string
	if lastErr != "" {
		e = &lastErr
	}
	_, err := s.db.Exec(ctx, `UPDATE processing_jobs SET state=$1, last_error=$2, updated_at=now() WHERE id=$3`, state, e, id)
	return err
}

func (s *Store) RetryJob(ctx context.Context, id int64, lastErr string) error {
	_, err := s.db.Exec(ctx, `UPDATE processing_jobs SET state=2, last_error=$1, updated_at=now() WHERE id=$2`, lastErr, id)
	return err
}

func (s *Store) CreateEmoteSet(ctx context.Context, name, ownerID string) (string, error) {
	var id string
	var owner *string
	if ownerID != "" {
		owner = &ownerID
	}
	err := s.db.QueryRow(ctx, `INSERT INTO emote_sets (name, owner_id) VALUES ($1, $2) RETURNING id`, name, owner).Scan(&id)
	return id, err
}

func (s *Store) AddEmoteToSet(ctx context.Context, setID, emoteID string, alias *string) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO emote_set_items (emote_set_id, emote_id, alias) VALUES ($1::uuid, $2::uuid, $3)
		ON CONFLICT (emote_set_id, emote_id) DO UPDATE SET alias=EXCLUDED.alias`,
		setID, emoteID, alias,
	)
	return err
}

func (s *Store) RemoveEmoteFromSet(ctx context.Context, setID, emoteID string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM emote_set_items WHERE emote_set_id=$1::uuid AND emote_id=$2::uuid`, setID, emoteID)
	return err
}

func (s *Store) UpsertChannel(ctx context.Context, twitchID, login, displayName string) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO channels (twitch_id, login, display_name) VALUES ($1, $2, $3)
		ON CONFLICT (twitch_id) DO UPDATE SET login=EXCLUDED.login, display_name=EXCLUDED.display_name, updated_at=now()`,
		twitchID, login, displayName,
	)
	return err
}

func (s *Store) SetActiveEmoteSet(ctx context.Context, twitchID, setID string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE channels SET active_emote_set_id=$1::uuid, updated_at=now() WHERE twitch_id=$2`,
		setID, twitchID,
	)
	return err
}

func (s *Store) GetChannel(ctx context.Context, twitchID string) (*Channel, error) {
	c := &Channel{}
	err := s.db.QueryRow(ctx, `
		SELECT twitch_id, login, COALESCE(display_name,''), active_emote_set_id, updated_at
		FROM channels WHERE twitch_id=$1`, twitchID,
	).Scan(&c.TwitchID, &c.Login, &c.DisplayName, &c.ActiveEmoteSetID, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (s *Store) GetChannelByLogin(ctx context.Context, login string) (*Channel, error) {
	c := &Channel{}
	err := s.db.QueryRow(ctx, `
		SELECT twitch_id, login, COALESCE(display_name,''), active_emote_set_id, updated_at
		FROM channels WHERE login=$1`, login,
	).Scan(&c.TwitchID, &c.Login, &c.DisplayName, &c.ActiveEmoteSetID, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (s *Store) GetChannelByActiveSet(ctx context.Context, setID string) (*Channel, error) {
	c := &Channel{}
	err := s.db.QueryRow(ctx, `
		SELECT twitch_id, login, COALESCE(display_name,''), active_emote_set_id, updated_at
		FROM channels WHERE active_emote_set_id=$1::uuid`, setID,
	).Scan(&c.TwitchID, &c.Login, &c.DisplayName, &c.ActiveEmoteSetID, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return c, nil
}

type ChannelEmote struct {
	Name            string
	EmoteID         string
	ProviderEmoteID string
	IsGlobal        bool
	Flags           int
	Provider        string
}

type ProviderEmoteSummary struct {
	Provider string
	Ready    int
	Pending  int
	Failed   int
}

type ChannelProviderLoad struct {
	Provider      string
	State         string
	Count         int
	ExpectedCount int
	ImportedCount int
	Error         string
}

type ChannelProviderSetSnapshot struct {
	ProviderSetID string
	Count         int
	EmoteHash     string
}

type SetProviderEmote struct {
	EmoteID         string
	ProviderEmoteID string
	Name            string
}

func (s *Store) UpdateEmoteName(ctx context.Context, id, name string) error {
	_, err := s.db.Exec(ctx, `UPDATE emotes SET name=$1, updated_at=now() WHERE id=$2::uuid`, name, id)
	return err
}

func (s *Store) ListSetProviderEmotes(ctx context.Context, setID, provider string) ([]SetProviderEmote, error) {
	rows, err := s.db.Query(ctx, `
		SELECT e.id, COALESCE(e.provider_emote_id, ''), e.name
		FROM emote_set_items i
		JOIN emotes e ON e.id = i.emote_id
		WHERE i.emote_set_id = $1::uuid
			AND i.status = 1
			AND e.status = 1
			AND e.is_global = false
			AND COALESCE(e.provider, 'custom') = $2`, setID, provider,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SetProviderEmote
	for rows.Next() {
		var item SetProviderEmote
		if err := rows.Scan(&item.EmoteID, &item.ProviderEmoteID, &item.Name); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ListActiveSevenTVChannels(ctx context.Context) ([]Channel, error) {
	rows, err := s.db.Query(ctx, `
		SELECT c.twitch_id, c.login, c.display_name, c.active_emote_set_id, c.updated_at
		FROM channels c
		WHERE c.active_emote_set_id IS NOT NULL
			AND EXISTS (
				SELECT 1
				FROM emote_set_items i
				JOIN emotes e ON e.id = i.emote_id
				WHERE i.emote_set_id = c.active_emote_set_id
					AND i.status = 1
					AND e.status = 1
					AND COALESCE(e.provider, 'custom') = 'seventv'
					AND COALESCE(e.provider_set_id, '') <> ''
					AND COALESCE(e.provider_set_id, '') <> 'global'
			)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Channel
	for rows.Next() {
		var ch Channel
		if err := rows.Scan(&ch.TwitchID, &ch.Login, &ch.DisplayName, &ch.ActiveEmoteSetID, &ch.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, ch)
	}
	return out, rows.Err()
}

func (s *Store) GetChannelSevenTVProviderSetID(ctx context.Context, login string) (string, bool, error) {
	var setID string
	err := s.db.QueryRow(ctx, `
		SELECT COALESCE(e.provider_set_id, '')
		FROM channels c
		JOIN emote_set_items i ON i.emote_set_id = c.active_emote_set_id AND i.status = 1
		JOIN emotes e ON e.id = i.emote_id
		WHERE c.login = $1
			AND COALESCE(e.provider, 'custom') = 'seventv'
			AND COALESCE(e.provider_set_id, '') <> ''
			AND COALESCE(e.provider_set_id, '') <> 'global'
		ORDER BY i.added_at DESC
		LIMIT 1`, login,
	).Scan(&setID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return setID, setID != "", nil
}

func (s *Store) GetChannelEmotes(ctx context.Context, login string) ([]ChannelEmote, error) {
	rows, err := s.db.Query(ctx, `
		WITH ranked AS (
			SELECT COALESCE(i.alias, e.name) AS name, e.id AS emote_id, COALESCE(e.provider_emote_id, '') AS provider_emote_id, e.is_global, e.flags,
				COALESCE(e.provider, 'custom') AS provider,
				CASE COALESCE(e.provider, 'custom')
					WHEN 'twitch' THEN 1
					WHEN 'seventv' THEN 2
					WHEN 'bttv' THEN 3
					WHEN 'ffz' THEN 4
					WHEN 'custom' THEN 5
					ELSE 6
				END AS provider_rank
			FROM channels c
			JOIN emote_sets es ON es.id = c.active_emote_set_id
			JOIN emote_set_items i ON i.emote_set_id = es.id
			JOIN emotes e ON e.id = i.emote_id
			WHERE c.login=$1 AND e.status=1 AND i.status=1
			UNION ALL
			SELECT e.name, e.id, COALESCE(e.provider_emote_id, '') AS provider_emote_id, e.is_global, e.flags, COALESCE(e.provider, 'custom') AS provider,
				CASE COALESCE(e.provider, 'custom')
					WHEN 'twitch' THEN 1
					WHEN 'seventv' THEN 2
					WHEN 'bttv' THEN 3
					WHEN 'ffz' THEN 4
					WHEN 'custom' THEN 5
					ELSE 6
				END AS provider_rank
			FROM emotes e
			WHERE e.is_global=true AND e.status=1
		)
		SELECT name, emote_id, provider_emote_id, is_global, flags, provider
		FROM ranked
		ORDER BY provider_rank, name`, login,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ChannelEmote
	for rows.Next() {
		var ce ChannelEmote
		if err := rows.Scan(&ce.Name, &ce.EmoteID, &ce.ProviderEmoteID, &ce.IsGlobal, &ce.Flags, &ce.Provider); err != nil {
			return nil, err
		}
		result = append(result, ce)
	}
	return result, rows.Err()
}

func (s *Store) GetChannelEmoteSummary(ctx context.Context, login string) (ready, pending int, channelKnown bool, err error) {
	err = s.db.QueryRow(ctx, `
		WITH channel_state AS (
			SELECT twitch_id, active_emote_set_id
			FROM channels
			WHERE login=$1
		),
		channel_emotes AS (
			SELECT e.status
			FROM channel_state c
			JOIN emote_set_items i ON i.emote_set_id = c.active_emote_set_id AND i.status = 1
			JOIN emotes e ON e.id = i.emote_id
		)
		SELECT
			COUNT(*) FILTER (WHERE status=1),
			COUNT(*) FILTER (WHERE status=0),
			EXISTS(SELECT 1 FROM channel_state)
		FROM channel_emotes`, login,
	).Scan(&ready, &pending, &channelKnown)
	return ready, pending, channelKnown, err
}

func (s *Store) GetChannelActiveSetName(ctx context.Context, login string) (string, bool, error) {
	var name string
	err := s.db.QueryRow(ctx, `
		SELECT es.name
		FROM channels c
		JOIN emote_sets es ON es.id = c.active_emote_set_id
		WHERE c.login=$1`, login,
	).Scan(&name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return name, true, nil
}

func (s *Store) GetChannelProviderEmoteSummary(ctx context.Context, login string) (map[string]ProviderEmoteSummary, error) {
	rows, err := s.db.Query(ctx, `
		WITH channel_state AS (
			SELECT active_emote_set_id
			FROM channels
			WHERE login=$1
		),
		channel_emotes AS (
			SELECT COALESCE(e.provider, 'custom') AS provider, e.status
			FROM channel_state c
			JOIN emote_set_items i ON i.emote_set_id = c.active_emote_set_id AND i.status = 1
			JOIN emotes e ON e.id = i.emote_id
		)
		SELECT
			provider,
			COUNT(*) FILTER (WHERE status=1),
			COUNT(*) FILTER (WHERE status=0),
			COUNT(*) FILTER (WHERE status NOT IN (0, 1))
		FROM channel_emotes
		GROUP BY provider`, login,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	summary := make(map[string]ProviderEmoteSummary)
	for rows.Next() {
		var item ProviderEmoteSummary
		if err := rows.Scan(&item.Provider, &item.Ready, &item.Pending, &item.Failed); err != nil {
			return nil, err
		}
		summary[item.Provider] = item
	}
	return summary, rows.Err()
}

func (s *Store) GetChannelProviderSetSnapshot(ctx context.Context, login, provider string) (ChannelProviderSetSnapshot, bool, error) {
	var item ChannelProviderSetSnapshot
	err := s.db.QueryRow(ctx, `
		WITH provider_items AS (
			SELECT COALESCE(e.provider_set_id, '') AS provider_set_id, COUNT(*)
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
	).Scan(&item.ProviderSetID, &item.Count, &item.EmoteHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ChannelProviderSetSnapshot{}, false, nil
		}
		return ChannelProviderSetSnapshot{}, false, err
	}
	return item, true, nil
}

func (s *Store) UpsertChannelProviderLoad(ctx context.Context, twitchID, provider, state string, count, expectedCount, importedCount int, lastErr string) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO channel_emote_providers (twitch_id, provider, state, count, expected_count, imported_count, last_error)
		VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''))
		ON CONFLICT (twitch_id, provider) DO UPDATE SET
			state=EXCLUDED.state,
			count=EXCLUDED.count,
			expected_count=EXCLUDED.expected_count,
			imported_count=EXCLUDED.imported_count,
			last_error=EXCLUDED.last_error,
			updated_at=now()`, twitchID, provider, state, count, expectedCount, importedCount, lastErr,
	)
	return err
}

func (s *Store) GetChannelProviderLoads(ctx context.Context, login string) (map[string]ChannelProviderLoad, error) {
	rows, err := s.db.Query(ctx, `
		SELECT p.provider, p.state, p.count, COALESCE(p.expected_count, 0), COALESCE(p.imported_count, 0), COALESCE(p.last_error, '')
		FROM channel_emote_providers p
		JOIN channels c ON c.twitch_id = p.twitch_id
		WHERE c.login=$1`, login,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	loads := make(map[string]ChannelProviderLoad)
	for rows.Next() {
		var item ChannelProviderLoad
		if err := rows.Scan(&item.Provider, &item.State, &item.Count, &item.ExpectedCount, &item.ImportedCount, &item.Error); err != nil {
			return nil, err
		}
		loads[item.Provider] = item
	}
	return loads, rows.Err()
}

func (s *Store) GetChannelsForEmote(ctx context.Context, emoteID string) ([]string, error) {
	rows, err := s.db.Query(ctx, `
		WITH target AS (
			SELECT is_global FROM emotes WHERE id=$1::uuid
		)
		SELECT DISTINCT c.login
		FROM channels c
		JOIN target t ON true
		LEFT JOIN emote_set_items i
			ON i.emote_set_id = c.active_emote_set_id
			AND i.emote_id = $1::uuid
			AND i.status = 1
		WHERE t.is_global OR i.emote_id IS NOT NULL`, emoteID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []string
	for rows.Next() {
		var login string
		if err := rows.Scan(&login); err != nil {
			return nil, err
		}
		channels = append(channels, login)
	}
	return channels, rows.Err()
}
