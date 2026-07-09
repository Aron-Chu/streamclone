CREATE TABLE IF NOT EXISTS live_chat_messages (
    id              BIGSERIAL PRIMARY KEY,
    channel         TEXT NOT NULL,
    login           TEXT,
    display_name    TEXT NOT NULL,
    message_id      TEXT NOT NULL,
    text            TEXT NOT NULL,
    fragments       JSONB NOT NULL DEFAULT '[]'::jsonb,
    ts              TIMESTAMPTZ NOT NULL,
    synced_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (channel, message_id)
);

CREATE INDEX IF NOT EXISTS idx_live_chat_channel_ts
    ON live_chat_messages (channel, ts);

CREATE INDEX IF NOT EXISTS idx_live_chat_channel_login
    ON live_chat_messages (channel, login);

CREATE TABLE IF NOT EXISTS chat_mod_events (
    id              BIGSERIAL PRIMARY KEY,
    channel         TEXT NOT NULL,
    kind            TEXT NOT NULL,
    actor_login     TEXT,
    target_login    TEXT,
    duration_sec    INT,
    reason          TEXT,
    message_id      TEXT,
    text_preview    TEXT,
    ts              TIMESTAMPTZ NOT NULL,
    synced_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_mod_events_channel_ts
    ON chat_mod_events (channel, ts);
