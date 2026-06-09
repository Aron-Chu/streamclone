CREATE TABLE stream_game_segments (
    id                  SERIAL PRIMARY KEY,
    stream_id           TEXT NOT NULL REFERENCES analytics_streams(stream_id) ON DELETE CASCADE,
    game_name           VARCHAR(255) NOT NULL,
    box_art_url         TEXT,
    offset_seconds      INT NOT NULL,
    duration_seconds    INT NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_game_segments_stream ON stream_game_segments(stream_id);
