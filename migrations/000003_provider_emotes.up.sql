ALTER TABLE emotes
    ADD COLUMN provider VARCHAR(20),
    ADD COLUMN provider_emote_id VARCHAR(100),
    ADD COLUMN provider_set_id VARCHAR(100),
    ADD COLUMN source_url TEXT;

CREATE UNIQUE INDEX idx_emotes_provider_identity
    ON emotes (provider, provider_emote_id)
    WHERE provider IS NOT NULL AND provider_emote_id IS NOT NULL;

CREATE INDEX idx_emotes_provider_set
    ON emotes (provider, provider_set_id)
    WHERE provider IS NOT NULL AND provider_set_id IS NOT NULL;