CREATE TABLE analytics_streams (
    stream_id           TEXT PRIMARY KEY,
    broadcaster_id      VARCHAR(100) NOT NULL,
    login               VARCHAR(50) NOT NULL,
    display_name        VARCHAR(100),
    profile_image_url   TEXT,
    description         TEXT,
    title               TEXT,
    category            TEXT,
    tags                JSONB NOT NULL DEFAULT '[]'::jsonb,
    language            VARCHAR(20),
    thumbnail_url       TEXT,
    started_at          TIMESTAMPTZ NOT NULL,
    ended_at            TIMESTAMPTZ,
    last_seen_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    current_viewers     INT NOT NULL DEFAULT 0,
    avg_viewers         INT NOT NULL DEFAULT 0,
    peak_viewers        INT NOT NULL DEFAULT 0,
    viewer_samples      INT NOT NULL DEFAULT 0,
    chat_messages       BIGINT NOT NULL DEFAULT 0,
    total_emote_uses    BIGINT NOT NULL DEFAULT 0,
    seventv_emote_uses  BIGINT NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE analytics_minute_rollups (
    stream_id           TEXT NOT NULL REFERENCES analytics_streams(stream_id) ON DELETE CASCADE,
    minute_ts           TIMESTAMPTZ NOT NULL,
    viewer_avg          INT NOT NULL DEFAULT 0,
    viewer_max          INT NOT NULL DEFAULT 0,
    viewer_latest       INT NOT NULL DEFAULT 0,
    viewer_samples      INT NOT NULL DEFAULT 0,
    chat_count          INT NOT NULL DEFAULT 0,
    total_emote_count   INT NOT NULL DEFAULT 0,
    seventv_emote_count INT NOT NULL DEFAULT 0,
    emotes_json         JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (stream_id, minute_ts)
);

CREATE INDEX idx_analytics_streams_login_started
    ON analytics_streams (login, started_at DESC);

CREATE INDEX idx_analytics_streams_started
    ON analytics_streams (started_at DESC);

CREATE INDEX idx_analytics_rollups_stream_minute
    ON analytics_minute_rollups (stream_id, minute_ts);
