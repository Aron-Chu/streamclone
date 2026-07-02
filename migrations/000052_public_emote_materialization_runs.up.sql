-- Audit trail for public emote provider materialization jobs.
CREATE TABLE IF NOT EXISTS public_emote_materialization_runs (
    run_id BIGSERIAL PRIMARY KEY,
    job_name TEXT NOT NULL,
    schema_version TEXT NOT NULL,
    range_value TEXT NOT NULL,
    status TEXT NOT NULL,
    range_start TIMESTAMPTZ NOT NULL,
    range_end TIMESTAMPTZ NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ,
    rows_upserted BIGINT NOT NULL DEFAULT 0 CHECK (rows_upserted >= 0),
    error_code TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_public_emote_materialization_runs_job_schema
    ON public_emote_materialization_runs (job_name, schema_version, updated_at DESC);
