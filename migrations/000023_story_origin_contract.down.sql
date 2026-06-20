ALTER TABLE moment_fingerprints
    DROP COLUMN IF EXISTS chat_spike_summary,
    DROP COLUMN IF EXISTS origin_confidence,
    DROP COLUMN IF EXISTS vod_id;
