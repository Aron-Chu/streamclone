CREATE TABLE IF NOT EXISTS backfill_jobs (
    id              BIGSERIAL PRIMARY KEY,
    tier            TEXT NOT NULL DEFAULT 'silver',
    stream_id       TEXT NOT NULL,
    login           TEXT NOT NULL,
    egress_slot     INT NOT NULL DEFAULT 0,
    attempt         INT NOT NULL DEFAULT 0,
    export_status   TEXT NOT NULL DEFAULT 'pending',
    status          TEXT NOT NULL DEFAULT 'queued',
    next_run_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    error           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT backfill_jobs_status_check
        CHECK (status IN ('queued', 'running', 'done', 'skipped', 'failed')),
    CONSTRAINT backfill_jobs_export_status_check
        CHECK (export_status IN ('pending', 'confirmed', 'failed', 'skipped'))
);

CREATE INDEX IF NOT EXISTS idx_backfill_jobs_next_run
    ON backfill_jobs (status, next_run_at)
    WHERE status IN ('queued', 'running');

CREATE UNIQUE INDEX IF NOT EXISTS idx_backfill_jobs_active_stream
    ON backfill_jobs (stream_id)
    WHERE status IN ('queued', 'running');
