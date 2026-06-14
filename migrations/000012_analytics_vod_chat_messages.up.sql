CREATE TABLE IF NOT EXISTS analytics_vod_chat_messages (
    id              BIGSERIAL PRIMARY KEY,
    stream_id       TEXT NOT NULL,
    minute_ts       TIMESTAMPTZ NOT NULL,
    message_id      TEXT NOT NULL,
    display_name    TEXT NOT NULL,
    sender_hash     TEXT NOT NULL,
    text            TEXT NOT NULL,
    emote_frags     JSONB NOT NULL DEFAULT '[]'::jsonb,
    offset_seconds  INT NOT NULL,
    synced_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (stream_id, message_id)
);

CREATE INDEX IF NOT EXISTS idx_vod_chat_stream_offset
    ON analytics_vod_chat_messages (stream_id, offset_seconds);

CREATE INDEX IF NOT EXISTS idx_vod_chat_synced_at
    ON analytics_vod_chat_messages (synced_at);
