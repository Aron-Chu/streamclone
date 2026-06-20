DROP INDEX IF EXISTS idx_stream_aliases_canonical;
DROP INDEX IF EXISTS idx_analytics_streams_canonical;
DROP INDEX IF EXISTS idx_stream_sessions_tt_id;
DROP INDEX IF EXISTS idx_stream_sessions_twitch_id;
DROP INDEX IF EXISTS idx_stream_sessions_vod;
DROP INDEX IF EXISTS idx_stream_sessions_login_window;
DROP INDEX IF EXISTS idx_stream_sessions_login_started;

ALTER TABLE analytics_streams DROP CONSTRAINT IF EXISTS analytics_streams_viewer_source_check;
ALTER TABLE analytics_streams DROP COLUMN IF EXISTS viewer_source;
ALTER TABLE analytics_streams DROP COLUMN IF EXISTS canonical_stream_id;

DROP TABLE IF EXISTS analytics_stream_aliases;
DROP TABLE IF EXISTS analytics_stream_sessions;
