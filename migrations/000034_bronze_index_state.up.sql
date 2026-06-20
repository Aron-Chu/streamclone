CREATE TABLE IF NOT EXISTS bronze_index_state (
    login              TEXT PRIMARY KEY,
    last_helix_at      TIMESTAMPTZ,
    last_summary_at    TIMESTAMPTZ,
    helix_blob_uri     TEXT,
    summary_blob_uri   TEXT,
    helix_row_count    INT NOT NULL DEFAULT 0,
    error              TEXT,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_bronze_index_state_stale
    ON bronze_index_state (last_helix_at NULLS FIRST, last_summary_at NULLS FIRST);
