DROP INDEX IF EXISTS idx_archive_job_items_backfill_job_id;
DROP INDEX IF EXISTS idx_backfill_jobs_archive_job_id;

ALTER TABLE archive_job_items DROP COLUMN IF EXISTS backfill_job_id;
ALTER TABLE backfill_jobs DROP COLUMN IF EXISTS archive_job_id;
