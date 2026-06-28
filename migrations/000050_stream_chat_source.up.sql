-- Stream-level chat source metadata for IVR Gold Lite + GQL canonical tracking.
ALTER TABLE analytics_streams
    ADD COLUMN IF NOT EXISTS chat_state TEXT NOT NULL DEFAULT 'none',
    ADD COLUMN IF NOT EXISTS chat_source TEXT NOT NULL DEFAULT 'none',
    ADD COLUMN IF NOT EXISTS source_confidence TEXT NOT NULL DEFAULT 'none',
    ADD COLUMN IF NOT EXISTS chat_coverage_pct REAL NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS ivr_coverage_pct REAL NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS live_coverage_pct REAL NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS gql_coverage_pct REAL NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS missing_windows_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS source_windows_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS last_source_upgrade_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS chat_source_detail TEXT NOT NULL DEFAULT '';

ALTER TABLE analytics_streams
    DROP CONSTRAINT IF EXISTS analytics_streams_chat_state_check;
ALTER TABLE analytics_streams
    ADD CONSTRAINT analytics_streams_chat_state_check CHECK (
        chat_state IN ('none', 'live_partial', 'ivr_lite', 'mixed_lite', 'gql_gold', 'failed')
    );

ALTER TABLE analytics_streams
    DROP CONSTRAINT IF EXISTS analytics_streams_chat_source_check;
ALTER TABLE analytics_streams
    ADD CONSTRAINT analytics_streams_chat_source_check CHECK (
        chat_source IN ('none', 'live', 'ivr', 'gql', 'mixed')
    );

ALTER TABLE analytics_streams
    DROP CONSTRAINT IF EXISTS analytics_streams_source_confidence_check;
ALTER TABLE analytics_streams
    ADD CONSTRAINT analytics_streams_source_confidence_check CHECK (
        source_confidence IN ('none', 'provisional', 'verified', 'canonical')
    );

-- Per-minute rollup source for write-priority rules (GQL canonical > live > IVR provisional).
ALTER TABLE analytics_minute_rollups
    ADD COLUMN IF NOT EXISTS chat_source TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS source_confidence TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS chat_source_detail TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_analytics_streams_chat_state
    ON analytics_streams (chat_state)
    WHERE chat_state NOT IN ('none', 'gql_gold');
