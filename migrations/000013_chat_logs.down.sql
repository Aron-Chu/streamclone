DROP TABLE IF EXISTS chat_mod_events;
DROP TABLE IF EXISTS live_chat_messages;

DROP INDEX IF EXISTS idx_vod_chat_stream_sender_hash;
DROP INDEX IF EXISTS idx_vod_chat_stream_display_name;

ALTER TABLE analytics_vod_chat_messages
    DROP COLUMN IF EXISTS commenter_login;
