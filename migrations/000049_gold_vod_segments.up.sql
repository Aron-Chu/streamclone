-- Gold VOD segment queue for parallel chat backfill windows.
CREATE TABLE IF NOT EXISTS gold_vod_segments (
    id                   BIGSERIAL   PRIMARY KEY,
    segment_key          TEXT        NOT NULL UNIQUE,
    vod_id               TEXT        NOT NULL,
    stream_id            TEXT        NOT NULL DEFAULT '',
    login                TEXT        NOT NULL DEFAULT '',
    backfill_job_id      BIGINT      REFERENCES backfill_jobs(id) ON DELETE SET NULL,
    strategy_version     TEXT        NOT NULL,
    start_offset_seconds INT         NOT NULL,
    end_offset_seconds   INT         NOT NULL,
    status               TEXT        NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'running', 'done', 'failed', 'dead_letter', 'skipped')),
    attempt              INT         NOT NULL DEFAULT 0,
    max_attempts         INT         NOT NULL DEFAULT 3,
    lease_owner          TEXT        NOT NULL DEFAULT '',
    lease_expires_at     TIMESTAMPTZ,
    heartbeat_at         TIMESTAMPTZ,
    next_run_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    comments_fetched     INT         NOT NULL DEFAULT 0,
    cursor               TEXT        NOT NULL DEFAULT '',
    error                TEXT        NOT NULL DEFAULT '',
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (attempt >= 0 AND max_attempts > 0),
    CHECK (start_offset_seconds >= 0 AND end_offset_seconds > start_offset_seconds)
);

CREATE INDEX IF NOT EXISTS idx_gold_vod_segments_claim
    ON gold_vod_segments (status, next_run_at, vod_id, start_offset_seconds)
    WHERE status IN ('queued', 'failed');
CREATE INDEX IF NOT EXISTS idx_gold_vod_segments_job
    ON gold_vod_segments (backfill_job_id) WHERE backfill_job_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_gold_vod_segments_lease
    ON gold_vod_segments (lease_expires_at) WHERE status = 'running';
CREATE INDEX IF NOT EXISTS idx_gold_vod_segments_vod_status
    ON gold_vod_segments (vod_id, status, start_offset_seconds);
CREATE UNIQUE INDEX IF NOT EXISTS idx_gold_vod_segments_window
    ON gold_vod_segments (vod_id, start_offset_seconds, end_offset_seconds, strategy_version);
