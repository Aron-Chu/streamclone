-- Emote Atlas materialization bookkeeping (retired; kept for migration chain parity).
CREATE TABLE IF NOT EXISTS emote_atlas_materialization_runs (
    run_id BIGSERIAL PRIMARY KEY,
    job_name TEXT NOT NULL,
    status TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
