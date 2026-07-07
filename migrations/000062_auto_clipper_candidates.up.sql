CREATE TABLE IF NOT EXISTS clip_candidates (
    id TEXT PRIMARY KEY,
    login TEXT NOT NULL,
    stream_id TEXT NOT NULL,
    vod_id TEXT NULL,
    minute_ts TIMESTAMPTZ NULL,
    offset_seconds INTEGER NOT NULL CHECK (offset_seconds >= 0),
    start_seconds INTEGER NOT NULL CHECK (start_seconds >= 0),
    end_seconds INTEGER NOT NULL CHECK (end_seconds > start_seconds),
    score INTEGER NOT NULL CHECK (score BETWEEN 0 AND 100),
    confidence DOUBLE PRECISION NOT NULL DEFAULT 0,
    reason TEXT NOT NULL,
    source_kind TEXT NOT NULL CHECK (source_kind IN ('live', 'recap', 'manual_import', 'historical')),
    coverage_state TEXT NULL,
    signals_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    top_emotes_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    source_status TEXT NOT NULL DEFAULT 'unknown' CHECK (source_status IN ('unknown', 'available', 'missing', 'restricted')),
    source_checked_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (stream_id, offset_seconds, reason),
    FOREIGN KEY (stream_id) REFERENCES analytics_streams(stream_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_clip_candidates_login_created
    ON clip_candidates (login, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_clip_candidates_stream_score
    ON clip_candidates (stream_id, score DESC);

CREATE INDEX IF NOT EXISTS idx_clip_candidates_source_status
    ON clip_candidates (source_status)
    WHERE source_status <> 'available';

CREATE TABLE IF NOT EXISTS clip_candidate_states (
    id TEXT PRIMARY KEY,
    candidate_id TEXT NOT NULL REFERENCES clip_candidates(id) ON DELETE CASCADE,
    principal_id TEXT NOT NULL,
    principal_kind TEXT NOT NULL CHECK (principal_kind IN ('guest', 'beta', 'device', 'user')),
    status TEXT NOT NULL DEFAULT 'new' CHECK (status IN ('new', 'saved', 'dismissed')),
    title_override TEXT NULL,
    start_seconds_override INTEGER NULL CHECK (start_seconds_override IS NULL OR start_seconds_override >= 0),
    end_seconds_override INTEGER NULL CHECK (end_seconds_override IS NULL OR end_seconds_override >= 0),
    notes TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (candidate_id, principal_id),
    CHECK (
        start_seconds_override IS NULL
        OR end_seconds_override IS NULL
        OR end_seconds_override > start_seconds_override
    )
);

CREATE INDEX IF NOT EXISTS idx_clip_candidate_states_principal_updated
    ON clip_candidate_states (principal_id, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_clip_candidate_states_status
    ON clip_candidate_states (principal_id, status, updated_at DESC);
