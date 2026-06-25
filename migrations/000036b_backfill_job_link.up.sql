ALTER TABLE backfill_jobs
    ADD COLUMN IF NOT EXISTS archive_job_id UUID REFERENCES archive_jobs(id) ON DELETE SET NULL;

ALTER TABLE archive_job_items
    ADD COLUMN IF NOT EXISTS backfill_job_id BIGINT REFERENCES backfill_jobs(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_backfill_jobs_archive_job_id
    ON backfill_jobs (archive_job_id)
    WHERE archive_job_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_archive_job_items_backfill_job_id
    ON archive_job_items (backfill_job_id)
    WHERE backfill_job_id IS NOT NULL;
