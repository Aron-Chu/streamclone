CREATE TABLE IF NOT EXISTS archive_jobs (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_type                TEXT NOT NULL,
    tier                    TEXT,
    status                  TEXT NOT NULL DEFAULT 'queued',
    trigger_source          TEXT,
    requested_by_user_id    UUID,
    requested_by_role       TEXT,
    total_items             INT NOT NULL DEFAULT 0,
    completed_items         INT NOT NULL DEFAULT 0,
    failed_items            INT NOT NULL DEFAULT 0,
    skipped_items           INT NOT NULL DEFAULT 0,
    retried_items           INT NOT NULL DEFAULT 0,
    current_channel         TEXT,
    current_stream_id       TEXT,
    current_vod_id          TEXT,
    current_artifact_type   TEXT,
    priority                INT NOT NULL DEFAULT 0,
    started_at              TIMESTAMPTZ,
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at             TIMESTAMPTZ,
    heartbeat_at            TIMESTAMPTZ,
    error                   TEXT,
    metadata                JSONB NOT NULL DEFAULT '{}',
    CONSTRAINT archive_jobs_status_check
        CHECK (status IN ('queued', 'running', 'succeeded', 'partial', 'failed', 'cancelled', 'paused', 'stale'))
);

CREATE TABLE IF NOT EXISTS archive_job_items (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id          UUID NOT NULL REFERENCES archive_jobs(id) ON DELETE CASCADE,
    item_key        TEXT NOT NULL,
    channel_login   TEXT,
    channel_id      TEXT,
    stream_id       TEXT,
    vod_id          TEXT,
    provider        TEXT,
    artifact_type   TEXT,
    status          TEXT NOT NULL DEFAULT 'queued',
    attempts        INT NOT NULL DEFAULT 0,
    started_at      TIMESTAMPTZ,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at     TIMESTAMPTZ,
    error           TEXT,
    output_uri      TEXT,
    output_sha256   TEXT,
    metadata        JSONB NOT NULL DEFAULT '{}',
    CONSTRAINT archive_job_items_status_check
        CHECK (status IN ('queued', 'running', 'succeeded', 'skipped', 'failed', 'retrying', 'cancelled')),
    CONSTRAINT archive_job_items_job_item_key UNIQUE (job_id, item_key)
);

CREATE TABLE IF NOT EXISTS archive_job_events (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id      UUID NOT NULL REFERENCES archive_jobs(id) ON DELETE CASCADE,
    item_id     UUID REFERENCES archive_job_items(id) ON DELETE SET NULL,
    level       TEXT NOT NULL DEFAULT 'info',
    event_type  TEXT NOT NULL,
    message     TEXT,
    metadata    JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_archive_jobs_status_updated
    ON archive_jobs (status, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_archive_jobs_type_tier_started
    ON archive_jobs (job_type, tier, started_at DESC);

CREATE INDEX IF NOT EXISTS idx_archive_jobs_heartbeat
    ON archive_jobs (heartbeat_at)
    WHERE status = 'running';

CREATE INDEX IF NOT EXISTS idx_archive_job_items_job_status
    ON archive_job_items (job_id, status);

CREATE INDEX IF NOT EXISTS idx_archive_job_items_channel
    ON archive_job_items (channel_login)
    WHERE channel_login IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_archive_job_items_stream
    ON archive_job_items (stream_id)
    WHERE stream_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_archive_job_items_artifact_status
    ON archive_job_items (artifact_type, status);

CREATE INDEX IF NOT EXISTS idx_archive_job_events_job_created
    ON archive_job_events (job_id, created_at DESC);
