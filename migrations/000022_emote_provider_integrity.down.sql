ALTER TABLE channel_emote_providers
    DROP COLUMN IF EXISTS expected_count,
    DROP COLUMN IF EXISTS imported_count;
