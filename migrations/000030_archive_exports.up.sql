CREATE TABLE IF NOT EXISTS archive_exports (
    artifact_type     TEXT NOT NULL,
    natural_key       TEXT NOT NULL,
    gcs_uri           TEXT NOT NULL,
    object_generation TEXT,
    etag              TEXT,
    row_count         BIGINT,
    byte_size         BIGINT,
    export_status     TEXT NOT NULL DEFAULT 'pending',
    error             TEXT,
    exported_at       TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (artifact_type, natural_key),
    CONSTRAINT archive_exports_status_check
        CHECK (export_status IN ('pending', 'confirmed', 'failed'))
);

CREATE INDEX IF NOT EXISTS idx_archive_exports_status
    ON archive_exports (artifact_type, export_status, exported_at DESC);

CREATE INDEX IF NOT EXISTS idx_archive_exports_updated
    ON archive_exports (updated_at DESC);
