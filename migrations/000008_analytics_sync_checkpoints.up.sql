CREATE TABLE IF NOT EXISTS analytics_sync_checkpoints (
    stream_id         TEXT NOT NULL,
    video_id          TEXT NOT NULL,
    cursor            TEXT NOT NULL DEFAULT '',
    offset_seconds    INT NOT NULL DEFAULT 0,
    comments_fetched  INT NOT NULL DEFAULT 0,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (stream_id, video_id)
);

CREATE INDEX IF NOT EXISTS idx_analytics_sync_checkpoints_updated
    ON analytics_sync_checkpoints (updated_at DESC);
