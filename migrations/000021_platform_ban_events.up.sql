CREATE TABLE platform_ban_events (
    id              BIGSERIAL PRIMARY KEY,
    streamer_login  VARCHAR(50) NOT NULL,
    display_name    TEXT,
    event_type      TEXT NOT NULL CHECK (event_type IN ('banned', 'unbanned', 'suspended')),
    platform        TEXT NOT NULL DEFAULT 'twitch',
    source          TEXT NOT NULL,
    source_item_id  BIGINT REFERENCES social_items(id) ON DELETE SET NULL,
    headline        TEXT NOT NULL,
    source_url      TEXT,
    occurred_at     TIMESTAMPTZ NOT NULL,
    confidence      REAL NOT NULL DEFAULT 0.7 CHECK (confidence BETWEEN 0.2 AND 1.0),
    raw             JSONB NOT NULL DEFAULT '{}'::jsonb,
    ingested_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (source, source_item_id)
);

CREATE INDEX idx_ban_events_window ON platform_ban_events (occurred_at DESC);
CREATE INDEX idx_ban_events_login ON platform_ban_events (streamer_login, occurred_at DESC);
