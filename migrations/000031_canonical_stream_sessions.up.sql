-- Phase B: canonical stream session layer (one logical session per broadcast).

CREATE TABLE analytics_stream_sessions (
    canonical_stream_id TEXT PRIMARY KEY,
    login               VARCHAR(50) NOT NULL,
    twitch_stream_id    TEXT NOT NULL DEFAULT '',
    tt_stream_id        TEXT NOT NULL DEFAULT '',
    vod_id              TEXT NOT NULL DEFAULT '',
    started_at          TIMESTAMPTZ NOT NULL,
    ended_at            TIMESTAMPTZ,
    title               TEXT,
    category            TEXT,
    viewer_source       TEXT NOT NULL DEFAULT 'unknown',
    source_confidence   TEXT NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT analytics_stream_sessions_viewer_source_check
        CHECK (viewer_source IN ('live', 'tt', 'merged', 'restored', 'unknown'))
);

CREATE TABLE analytics_stream_aliases (
    alias_stream_id     TEXT PRIMARY KEY,
    canonical_stream_id TEXT NOT NULL REFERENCES analytics_stream_sessions (canonical_stream_id) ON DELETE CASCADE,
    alias_kind          TEXT NOT NULL DEFAULT 'unknown',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE analytics_streams
    ADD COLUMN IF NOT EXISTS canonical_stream_id TEXT,
    ADD COLUMN IF NOT EXISTS viewer_source TEXT NOT NULL DEFAULT 'unknown';

ALTER TABLE analytics_streams
    DROP CONSTRAINT IF EXISTS analytics_streams_viewer_source_check;

ALTER TABLE analytics_streams
    ADD CONSTRAINT analytics_streams_viewer_source_check
        CHECK (viewer_source IN ('live', 'tt', 'merged', 'restored', 'unknown'));

CREATE INDEX idx_stream_sessions_login_started
    ON analytics_stream_sessions (login, started_at DESC);

CREATE INDEX idx_stream_sessions_login_window
    ON analytics_stream_sessions (login, started_at, ended_at);

CREATE INDEX idx_stream_sessions_vod
    ON analytics_stream_sessions (login, vod_id)
    WHERE vod_id <> '';

CREATE INDEX idx_stream_sessions_twitch_id
    ON analytics_stream_sessions (twitch_stream_id)
    WHERE twitch_stream_id <> '';

CREATE INDEX idx_stream_sessions_tt_id
    ON analytics_stream_sessions (tt_stream_id)
    WHERE tt_stream_id <> '';

CREATE INDEX idx_analytics_streams_canonical
    ON analytics_streams (canonical_stream_id)
    WHERE canonical_stream_id IS NOT NULL;

CREATE INDEX idx_stream_aliases_canonical
    ON analytics_stream_aliases (canonical_stream_id);

-- Backfill sessions from existing stream rows (self-canonical until merge pass).
INSERT INTO analytics_stream_sessions (
    canonical_stream_id, login, twitch_stream_id, tt_stream_id, vod_id,
    started_at, ended_at, title, category, viewer_source, source_confidence
)
SELECT
    stream_id,
    login,
    CASE WHEN broadcaster_id <> 'pending' THEN stream_id ELSE '' END,
    '',
    COALESCE(vod_id, ''),
    started_at,
    ended_at,
    title,
    category,
    CASE
        WHEN viewer_samples >= 3 AND broadcaster_id <> 'pending' THEN 'live'
        WHEN viewer_samples > 0 OR peak_viewers > 0 OR avg_viewers > 0 THEN 'tt'
        ELSE 'unknown'
    END,
    'migration'
FROM analytics_streams
ON CONFLICT (canonical_stream_id) DO NOTHING;

UPDATE analytics_streams
SET canonical_stream_id = stream_id
WHERE canonical_stream_id IS NULL;

-- Merge prefetch stubs into richer overlapping sessions for the same login/hour.
WITH ranked AS (
    SELECT
        s.stream_id,
        s.login,
        s.started_at,
        s.broadcaster_id,
        s.viewer_samples,
        s.chat_messages,
        s.peak_viewers,
        ROW_NUMBER() OVER (
            PARTITION BY s.login, date_trunc('hour', s.started_at)
            ORDER BY
                (CASE WHEN s.broadcaster_id = 'pending' THEN 0 ELSE 2 END)
                + (CASE WHEN COALESCE(s.viewer_samples, 0) + COALESCE(s.chat_messages, 0) > 0 THEN 3 ELSE 0 END)
                + (CASE WHEN COALESCE(s.peak_viewers, 0) > 0 THEN 1 ELSE 0 END) DESC,
                s.started_at ASC,
                s.stream_id ASC
        ) AS rn
    FROM analytics_streams s
),
groups AS (
    SELECT login, date_trunc('hour', started_at) AS hour_bucket, MIN(stream_id) FILTER (WHERE rn = 1) AS canonical_id
    FROM ranked
    GROUP BY login, date_trunc('hour', started_at)
    HAVING COUNT(*) > 1
),
merge_map AS (
    SELECT r.stream_id AS alias_id, g.canonical_id
    FROM ranked r
    JOIN groups g
      ON r.login = g.login
     AND date_trunc('hour', r.started_at) = g.hour_bucket
    WHERE r.stream_id <> g.canonical_id
)
INSERT INTO analytics_stream_aliases (alias_stream_id, canonical_stream_id, alias_kind)
SELECT alias_id, canonical_id, 'migration_merge'
FROM merge_map
ON CONFLICT (alias_stream_id) DO NOTHING;

UPDATE analytics_streams AS st
SET canonical_stream_id = a.canonical_stream_id
FROM analytics_stream_aliases a
WHERE st.stream_id = a.alias_stream_id;

UPDATE analytics_stream_sessions sess
SET viewer_source = st.viewer_source,
    vod_id = COALESCE(NULLIF(st.vod_id, ''), sess.vod_id),
    ended_at = COALESCE(sess.ended_at, st.ended_at),
    title = COALESCE(NULLIF(st.title, ''), sess.title),
    category = COALESCE(NULLIF(st.category, ''), sess.category)
FROM analytics_streams st
WHERE st.stream_id = sess.canonical_stream_id;
