ALTER TABLE channel_emote_providers
    ADD COLUMN expected_count INT NOT NULL DEFAULT 0,
    ADD COLUMN imported_count INT NOT NULL DEFAULT 0;
