ALTER TABLE pulse_bookmarks
    ADD COLUMN IF NOT EXISTS principal_id   TEXT NULL,
    ADD COLUMN IF NOT EXISTS principal_kind TEXT NULL CHECK (principal_kind IS NULL OR principal_kind IN ('beta', 'device', 'user'));

CREATE INDEX IF NOT EXISTS idx_pulse_bookmarks_principal_created
    ON pulse_bookmarks (principal_id, created_at DESC)
    WHERE principal_id IS NOT NULL;
