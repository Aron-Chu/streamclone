CREATE TABLE IF NOT EXISTS pulse_roster_state (
    login TEXT PRIMARY KEY,
    broadcaster_id TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'protected',
    priority INT NOT NULL DEFAULT 60,
    last_live_stream_id TEXT NOT NULL DEFAULT '',
    last_live_seen_at TIMESTAMPTZ,
    last_polled_at TIMESTAMPTZ,
    next_poll_after TIMESTAMPTZ,
    last_error_code TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_pulse_roster_state_next_poll
    ON pulse_roster_state (next_poll_after NULLS FIRST, login);
