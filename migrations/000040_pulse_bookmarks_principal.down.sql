DROP INDEX IF EXISTS idx_pulse_bookmarks_principal_created;

ALTER TABLE pulse_bookmarks
    DROP COLUMN IF EXISTS principal_kind,
    DROP COLUMN IF EXISTS principal_id;
