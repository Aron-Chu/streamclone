CREATE TABLE IF NOT EXISTS pulse_bookmarks (
  id              TEXT        PRIMARY KEY,
  user_id         TEXT        NULL,
  login           TEXT        NOT NULL,
  stream_id       TEXT        NULL,
  vod_id          TEXT        NULL,
  offset_seconds  INTEGER     NOT NULL CHECK (offset_seconds >= 0),
  label           TEXT        NOT NULL,
  notes           TEXT        NOT NULL DEFAULT '',
  score           INTEGER     NULL CHECK (score IS NULL OR (score BETWEEN 0 AND 100)),
  source          TEXT        NOT NULL DEFAULT 'web' CHECK (source IN ('web','extension')),
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_pulse_bookmarks_user_created ON pulse_bookmarks (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_pulse_bookmarks_stream ON pulse_bookmarks (stream_id);
CREATE INDEX IF NOT EXISTS idx_pulse_bookmarks_login ON pulse_bookmarks (login);
