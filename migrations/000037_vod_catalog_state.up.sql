CREATE TABLE IF NOT EXISTS vod_catalog_state (
    login TEXT NOT NULL,
    vod_id TEXT NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (login, vod_id)
);

CREATE INDEX IF NOT EXISTS idx_vod_catalog_state_login ON vod_catalog_state (login);
