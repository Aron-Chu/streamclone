DROP INDEX IF EXISTS idx_emotes_provider_set;
DROP INDEX IF EXISTS idx_emotes_provider_identity;

ALTER TABLE emotes
    DROP COLUMN IF EXISTS source_url,
    DROP COLUMN IF EXISTS provider_set_id,
    DROP COLUMN IF EXISTS provider_emote_id,
    DROP COLUMN IF EXISTS provider;