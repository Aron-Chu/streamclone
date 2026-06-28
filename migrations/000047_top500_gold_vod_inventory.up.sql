-- Durable Top 500 VOD inventory for Gold direct-enqueue and archive tracking.
CREATE TABLE IF NOT EXISTS top500_vod_inventory (
    vod_id                TEXT        PRIMARY KEY,
    stream_id             TEXT        NOT NULL DEFAULT '',
    login                 TEXT        NOT NULL,
    channel_id            TEXT        NOT NULL DEFAULT '',
    top500_rank           INT         CHECK (top500_rank IS NULL OR top500_rank > 0),
    title                 TEXT        NOT NULL DEFAULT '',
    category_name         TEXT        NOT NULL DEFAULT '',
    started_at            TIMESTAMPTZ,
    ended_at              TIMESTAMPTZ,
    duration_minutes      INT         CHECK (duration_minutes IS NULL OR duration_minutes >= 0),
    availability_state    TEXT        NOT NULL DEFAULT 'discovered'
        CHECK (availability_state IN (
            'discovered', 'eligible', 'queued', 'loaded', 'expired', 'deleted',
            'private_or_sub_only', 'no_chat', 'region_blocked', 'gql_forbidden',
            'gql_throttled', 'unknown_unavailable', 'failed'
        )),
    source                TEXT        NOT NULL DEFAULT 'bronze_vod_index',
    source_metadata       JSONB       NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(source_metadata) = 'object'),
    last_checked_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    gold_status           TEXT        NOT NULL DEFAULT 'not_queued'
        CHECK (gold_status IN ('not_queued', 'queued', 'running', 'done', 'failed', 'skipped')),
    gold_backfill_job_id  BIGINT      REFERENCES backfill_jobs(id) ON DELETE SET NULL,
    gold_queued_at        TIMESTAMPTZ,
    gold_completed_at     TIMESTAMPTZ,
    archive_export_status TEXT        NOT NULL DEFAULT '',
    error                 TEXT        NOT NULL DEFAULT '',
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_top500_vod_inventory_availability
    ON top500_vod_inventory (availability_state, last_checked_at DESC);
CREATE INDEX IF NOT EXISTS idx_top500_vod_inventory_gold_status
    ON top500_vod_inventory (gold_status, last_checked_at DESC);
CREATE INDEX IF NOT EXISTS idx_top500_vod_inventory_login_started
    ON top500_vod_inventory (login, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_top500_vod_inventory_rank_started
    ON top500_vod_inventory (top500_rank, started_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_top500_vod_inventory_stream
    ON top500_vod_inventory (stream_id) WHERE stream_id <> '';
