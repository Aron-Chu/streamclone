CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX IF NOT EXISTS idx_live_chat_text_trgm
    ON live_chat_messages USING gin (text gin_trgm_ops);

CREATE INDEX IF NOT EXISTS idx_live_chat_channel_ts_id
    ON live_chat_messages (channel, ts, id);
