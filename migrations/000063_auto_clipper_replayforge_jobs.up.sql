CREATE TABLE IF NOT EXISTS clip_candidate_jobs (
    id TEXT PRIMARY KEY,
    candidate_id TEXT NOT NULL REFERENCES clip_candidates(id) ON DELETE CASCADE,
    principal_id TEXT NOT NULL,
    principal_kind TEXT NOT NULL CHECK (principal_kind IN ('guest', 'beta', 'device', 'user')),
    status TEXT NOT NULL CHECK (status IN ('queued', 'ready', 'failed', 'source_unavailable')),
    replayforge_job_id TEXT NULL,
    replayforge_state TEXT NULL,
    request_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    response_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    error_code TEXT NULL,
    error_message TEXT NULL,
    submitted_at TIMESTAMPTZ NULL,
    last_checked_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (candidate_id, principal_id)
);

CREATE INDEX IF NOT EXISTS idx_clip_candidate_jobs_principal_updated
    ON clip_candidate_jobs (principal_id, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_clip_candidate_jobs_replayforge
    ON clip_candidate_jobs (replayforge_job_id)
    WHERE replayforge_job_id IS NOT NULL;
