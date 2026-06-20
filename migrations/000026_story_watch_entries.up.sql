CREATE TABLE story_watch_entries (
    id          BIGSERIAL PRIMARY KEY,
    user_ref    TEXT NOT NULL DEFAULT 'local',
    kind        TEXT NOT NULL CHECK (kind IN ('category', 'keyword')),
    value       TEXT NOT NULL,
    label       TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_ref, kind, value)
);

CREATE INDEX idx_story_watch_entries_user ON story_watch_entries (user_ref, created_at DESC);
