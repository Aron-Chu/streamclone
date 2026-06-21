DROP INDEX IF EXISTS idx_archive_exports_stream_tier;
DROP INDEX IF EXISTS idx_archive_exports_tier_channel;

ALTER TABLE archive_exports DROP CONSTRAINT IF EXISTS archive_exports_status_check;
ALTER TABLE archive_exports ADD CONSTRAINT archive_exports_status_check
    CHECK (export_status IN ('pending', 'confirmed', 'failed'));

ALTER TABLE archive_exports
    DROP COLUMN IF EXISTS metadata,
    DROP COLUMN IF EXISTS failure_reason,
    DROP COLUMN IF EXISTS uncompressed_size_bytes,
    DROP COLUMN IF EXISTS content_sha256,
    DROP COLUMN IF EXISTS source_url,
    DROP COLUMN IF EXISTS vod_id,
    DROP COLUMN IF EXISTS stream_id,
    DROP COLUMN IF EXISTS channel_id,
    DROP COLUMN IF EXISTS channel_login,
    DROP COLUMN IF EXISTS provider,
    DROP COLUMN IF EXISTS tier,
    DROP COLUMN IF EXISTS artifact_id;
