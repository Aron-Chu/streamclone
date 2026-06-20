ALTER TABLE moment_fingerprints
    ADD COLUMN IF NOT EXISTS vod_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS origin_confidence REAL,
    ADD COLUMN IF NOT EXISTS chat_spike_summary TEXT NOT NULL DEFAULT '';
