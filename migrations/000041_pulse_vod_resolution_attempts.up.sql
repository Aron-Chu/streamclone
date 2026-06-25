CREATE TABLE IF NOT EXISTS pulse_vod_resolution_attempts (
    id BIGSERIAL PRIMARY KEY,
    stream_id TEXT NOT NULL,
    login TEXT NOT NULL DEFAULT '',
    twitch_stream_id TEXT NOT NULL DEFAULT '',
    broadcaster_id TEXT NOT NULL DEFAULT '',
    candidate_vod_id TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('resolving', 'available', 'waiting', 'unavailable', 'deleted', 'private', 'error')),
    attempts INTEGER NOT NULL DEFAULT 0,
    last_attempt_at TIMESTAMPTZ NULL,
    next_auto_retry_at TIMESTAMPTZ NULL,
    final_after_at TIMESTAMPTZ NULL,
    finalized_at TIMESTAMPTZ NULL,
    manual_retry_allowed BOOLEAN NOT NULL DEFAULT false,
    error_code TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_pulse_vod_resolution_stream_updated
    ON pulse_vod_resolution_attempts (stream_id, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_pulse_vod_resolution_retry
    ON pulse_vod_resolution_attempts (status, next_auto_retry_at)
    WHERE next_auto_retry_at IS NOT NULL;
