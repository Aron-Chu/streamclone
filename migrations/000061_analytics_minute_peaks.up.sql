CREATE TABLE IF NOT EXISTS analytics_minute_peaks (
    stream_id TEXT NOT NULL,
    minute_ts TIMESTAMPTZ NOT NULL,
    chat_count INTEGER NOT NULL DEFAULT 0,
    total_emote_count INTEGER NOT NULL DEFAULT 0,
    seventv_emote_count INTEGER NOT NULL DEFAULT 0,
    emotes_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    chat_source TEXT NOT NULL DEFAULT '',
    source_confidence TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (stream_id, minute_ts),
    FOREIGN KEY (stream_id) REFERENCES analytics_streams(stream_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_analytics_minute_peaks_window_hot
    ON analytics_minute_peaks (minute_ts, chat_count DESC, total_emote_count DESC)
    WHERE chat_count > 0;
