-- Pulse Wire / Story Graph reliability registry, follows, and trend snapshots (design.md §4, §11).

CREATE TABLE source_reliability (                       -- R7, config-overridable
    source_type        TEXT PRIMARY KEY,                -- pulse_origin|reddit_thread|x_post|...
    source_risk        TEXT NOT NULL,                   -- official|public_api|scraper|unofficial
    confidence_weight  REAL NOT NULL CHECK (confidence_weight BETWEEN 0.2 AND 1.0)
);

-- Seed default weights (design.md §11). Overridable via config without a deploy.
INSERT INTO source_reliability (source_type, source_risk, confidence_weight) VALUES
    ('pulse_origin',    'official',   1.00),
    ('twitch_clip',     'official',   0.95),
    ('news_article',    'public_api', 0.75),
    ('reddit_thread',   'public_api', 0.70),
    ('youtube_video',   'public_api', 0.70),
    ('x_post',          'public_api', 0.60),
    ('bluesky_post',    'public_api', 0.55),
    ('manual_curation', 'unofficial', 0.40);

CREATE TABLE story_follows (                            -- R11 (see "Identity & auth model" in §13)
    cluster_id  BIGINT NOT NULL REFERENCES story_clusters(id) ON DELETE CASCADE,
    user_ref    TEXT NOT NULL DEFAULT 'local',          -- local viewer identity, NOT a streamer login
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (cluster_id, user_ref)
);

CREATE TABLE trend_snapshots (                          -- R6.1 (Postgres for moderate volume; see §10)
    cluster_id  BIGINT NOT NULL REFERENCES story_clusters(id) ON DELETE CASCADE,
    at          TIMESTAMPTZ NOT NULL,
    trend       REAL,
    volatility  REAL,
    PRIMARY KEY (cluster_id, at)
);
