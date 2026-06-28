DROP INDEX IF EXISTS idx_analytics_streams_chat_state;

ALTER TABLE analytics_minute_rollups
    DROP COLUMN IF EXISTS chat_source_detail,
    DROP COLUMN IF EXISTS source_confidence,
    DROP COLUMN IF EXISTS chat_source;

ALTER TABLE analytics_streams
    DROP CONSTRAINT IF EXISTS analytics_streams_source_confidence_check,
    DROP CONSTRAINT IF EXISTS analytics_streams_chat_source_check,
    DROP CONSTRAINT IF EXISTS analytics_streams_chat_state_check;

ALTER TABLE analytics_streams
    DROP COLUMN IF EXISTS chat_source_detail,
    DROP COLUMN IF EXISTS last_source_upgrade_at,
    DROP COLUMN IF EXISTS source_windows_json,
    DROP COLUMN IF EXISTS missing_windows_json,
    DROP COLUMN IF EXISTS gql_coverage_pct,
    DROP COLUMN IF EXISTS live_coverage_pct,
    DROP COLUMN IF EXISTS ivr_coverage_pct,
    DROP COLUMN IF EXISTS chat_coverage_pct,
    DROP COLUMN IF EXISTS source_confidence,
    DROP COLUMN IF EXISTS chat_source,
    DROP COLUMN IF EXISTS chat_state;
