-- Expand archive_exports for corpus manifest v2 (nullable columns; legacy rows remain valid).
ALTER TABLE archive_exports
    ADD COLUMN IF NOT EXISTS artifact_id UUID,
    ADD COLUMN IF NOT EXISTS tier TEXT,
    ADD COLUMN IF NOT EXISTS provider TEXT,
    ADD COLUMN IF NOT EXISTS channel_login TEXT,
    ADD COLUMN IF NOT EXISTS channel_id TEXT,
    ADD COLUMN IF NOT EXISTS stream_id TEXT,
    ADD COLUMN IF NOT EXISTS vod_id TEXT,
    ADD COLUMN IF NOT EXISTS source_url TEXT,
    ADD COLUMN IF NOT EXISTS content_sha256 TEXT,
    ADD COLUMN IF NOT EXISTS uncompressed_size_bytes BIGINT,
    ADD COLUMN IF NOT EXISTS failure_reason TEXT,
    ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '{}';

-- Widen status check: app maps confirmed→complete in metadata; keep confirmed for v1 compat.
ALTER TABLE archive_exports DROP CONSTRAINT IF EXISTS archive_exports_status_check;
ALTER TABLE archive_exports ADD CONSTRAINT archive_exports_status_check
    CHECK (export_status IN ('pending', 'confirmed', 'failed', 'partial', 'complete'));

CREATE INDEX IF NOT EXISTS idx_archive_exports_tier_channel
    ON archive_exports (tier, channel_login)
    WHERE channel_login IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_archive_exports_stream_tier
    ON archive_exports (stream_id, tier)
    WHERE stream_id IS NOT NULL;
