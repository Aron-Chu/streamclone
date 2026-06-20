CREATE TABLE IF NOT EXISTS tracked_streamers (
    twitch_user_id      TEXT NOT NULL DEFAULT '',
    login               TEXT PRIMARY KEY,
    display_name        TEXT NOT NULL DEFAULT '',
    priority_tier       TEXT NOT NULL DEFAULT 'P2',
    last_seen_live_at   TIMESTAMPTZ,
    last_rank           INT,
    is_always_tracked   BOOLEAN NOT NULL DEFAULT false,
    archive_policy      TEXT NOT NULL DEFAULT 'standard',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT tracked_streamers_tier_check
        CHECK (priority_tier IN ('P0', 'P1', 'P2', 'P3', 'P4'))
);

CREATE INDEX IF NOT EXISTS idx_tracked_streamers_live
    ON tracked_streamers (last_seen_live_at DESC NULLS LAST);

CREATE INDEX IF NOT EXISTS idx_tracked_streamers_tier
    ON tracked_streamers (priority_tier, last_rank);
