-- Pulse Wire directory stats backbone (Phase 1).
-- Historizes the Twitch directory (GQL TopStreams via metadata /v1/streams) so a real
-- rising leaderboard and per-streamer viewer/rank series can be computed. Everything here
-- is opt-in behind PULSE_WIRE_ENABLED; Core Watch never touches these tables.

-- Periodic snapshots of the live directory; one row per streamer per sample run.
CREATE TABLE directory_samples (
    id            BIGSERIAL PRIMARY KEY,
    twitch_login  VARCHAR(50) NOT NULL,
    twitch_id     VARCHAR(32),
    display_name  TEXT,
    category      TEXT,
    viewers       INTEGER NOT NULL DEFAULT 0,
    rank          INTEGER NOT NULL,                     -- 1-based position within the sampled top-N
    is_live       BOOLEAN NOT NULL DEFAULT true,
    sample_run_id TEXT NOT NULL,                        -- groups all rows written by one sampler tick
    sampled_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_directory_samples_login_sampled ON directory_samples (twitch_login, sampled_at);
CREATE INDEX idx_directory_samples_sampled ON directory_samples (sampled_at);

-- Computed rising signals per streamer per window (today|24h|7d). Upserted by the Phase 2 job.
-- "window" is a reserved word in PostgreSQL, so it must always be double-quoted in DDL/DML.
CREATE TABLE streamer_rising (
    twitch_login     VARCHAR(50) NOT NULL,
    "window"         TEXT NOT NULL,                     -- today|24h|7d
    viewers_now      INTEGER,
    viewers_prev     INTEGER,
    viewer_delta_pct REAL,
    rank_now         INTEGER,
    rank_prev        INTEGER,
    rank_delta       INTEGER,
    new_entrant      BOOLEAN NOT NULL DEFAULT false,
    clip_velocity    REAL,
    rising_score     REAL,
    computed_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (twitch_login, "window")
);

CREATE INDEX idx_streamer_rising_window_score ON streamer_rising ("window", rising_score DESC);

-- Optional bounded follower history for TwitchTracker enrichment of the rising set only.
CREATE TABLE streamer_follower_snapshots (
    id           BIGSERIAL PRIMARY KEY,
    twitch_login VARCHAR(50) NOT NULL,
    followers    BIGINT NOT NULL,
    sampled_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_streamer_follower_snapshots_login_sampled ON streamer_follower_snapshots (twitch_login, sampled_at);
