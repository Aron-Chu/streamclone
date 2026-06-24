CREATE TABLE IF NOT EXISTS pulse_watchlist (
    id              TEXT        PRIMARY KEY,
    principal_id    TEXT        NOT NULL,
    principal_kind  TEXT        NOT NULL DEFAULT 'beta' CHECK (principal_kind IN ('beta', 'device', 'user')),
    login           TEXT        NOT NULL,
    always_track    BOOLEAN     NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (principal_id, login)
);

CREATE INDEX IF NOT EXISTS idx_pulse_watchlist_principal ON pulse_watchlist (principal_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_pulse_watchlist_always ON pulse_watchlist (always_track) WHERE always_track;
