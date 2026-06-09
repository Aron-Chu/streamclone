CREATE TABLE channel_emote_providers (
    twitch_id  VARCHAR(100) NOT NULL REFERENCES channels(twitch_id) ON DELETE CASCADE,
    provider   VARCHAR(20) NOT NULL,
    state      VARCHAR(20) NOT NULL DEFAULT 'ready',
    count      INT NOT NULL DEFAULT 0,
    last_error TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (twitch_id, provider)
);

