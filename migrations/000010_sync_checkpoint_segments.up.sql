ALTER TABLE analytics_sync_checkpoints
    ADD COLUMN IF NOT EXISTS segments_json TEXT NOT NULL DEFAULT '';

ALTER TABLE analytics_sync_checkpoints
    ADD COLUMN IF NOT EXISTS fetch_mode TEXT NOT NULL DEFAULT '';
