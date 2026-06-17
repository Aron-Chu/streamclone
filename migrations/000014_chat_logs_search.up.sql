CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX IF NOT EXISTS idx_vod_chat_text_trgm
    ON analytics_vod_chat_messages USING gin (text gin_trgm_ops);

CREATE INDEX IF NOT EXISTS idx_vod_chat_stream_offset_id
    ON analytics_vod_chat_messages (stream_id, offset_seconds, id);
