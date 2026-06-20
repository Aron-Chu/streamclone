-- Pulse Wire / Story Graph core schema (design.md §4).
-- No pgvector here: the `vector` extension is unavailable on a stock postgres:16-alpine,
-- so semantic columns live in the opt-in migrations/optional/000017 migration instead.

CREATE TABLE streamer_entities (
    id            BIGSERIAL PRIMARY KEY,
    twitch_login  VARCHAR(50) UNIQUE,
    twitch_id     VARCHAR(32),
    display_name  TEXT,
    aliases       JSONB NOT NULL DEFAULT '[]'::jsonb,   -- [{platform, handle}]
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE moment_fingerprints (
    id            BIGSERIAL PRIMARY KEY,
    entity_id     BIGINT REFERENCES streamer_entities(id) ON DELETE CASCADE,  -- forget-streamer cascades
    stream_id     VARCHAR(64) NOT NULL,
    vod_offset_s  INTEGER,                              -- VOD timestamp/offset
    window_start  TIMESTAMPTZ,
    window_end    TIMESTAMPTZ,
    transcript_kw JSONB NOT NULL DEFAULT '[]'::jsonb,   -- top quotes/keywords
    top_emotes    JSONB NOT NULL DEFAULT '[]'::jsonb,   -- provider:id:name + counts
    game          TEXT,
    phash         BYTEA,
    scene_hashes  JSONB,
    audio_fp      BYTEA,
    ocr_text      TEXT,
    fp_version    INTEGER NOT NULL DEFAULT 1,           -- determinism (R1.5)
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (stream_id, vod_offset_s, fp_version)
);

CREATE INDEX idx_moment_fingerprints_entity ON moment_fingerprints (entity_id);
CREATE INDEX idx_moment_fingerprints_stream ON moment_fingerprints (stream_id);

CREATE TABLE social_items (                             -- posts/videos/comments unified
    id              BIGSERIAL PRIMARY KEY,
    source          TEXT NOT NULL,                      -- reddit|youtube|x|bluesky|kick|news|twitch_clip|manual
    kind            TEXT NOT NULL,                      -- post|video|comment|clip|article
    external_id     TEXT NOT NULL,
    url             TEXT NOT NULL,                      -- canonical link is the durable proof
    author          TEXT,
    created_at_src  TIMESTAMPTZ,
    text            TEXT,                               -- minimized: caption/title only, not full bodies
    metrics         JSONB NOT NULL DEFAULT '{}'::jsonb, -- {score,likes,views,comments}
    entity_id       BIGINT REFERENCES streamer_entities(id) ON DELETE SET NULL, -- de-link, keep third-party post (R2.3)
    item_fp         JSONB,                              -- caption/visual/audio/transcript fingerprint
    -- Raw-data minimization (R16.4): we do NOT store the full source payload. We keep a small,
    -- bounded provenance record + a hash of the exact bytes we saw, so the link is auditable
    -- without warehousing third-party content.
    provenance      JSONB NOT NULL DEFAULT '{}'::jsonb, -- {fetched_at, source_api, request_id, http_status}
    snapshot_sha256 BYTEA,                              -- hash of the original fetched payload (proof, not the payload)
    ingested_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at      TIMESTAMPTZ NOT NULL,               -- retention TTL (R16.4); reaper deletes past this
    UNIQUE (source, external_id)                        -- dedup (R13.4)
);

CREATE INDEX idx_social_items_expires_at ON social_items (expires_at);
CREATE INDEX idx_social_items_entity ON social_items (entity_id);

CREATE TABLE story_clusters (
    id            BIGSERIAL PRIMARY KEY,
    entity_id     BIGINT REFERENCES streamer_entities(id) ON DELETE CASCADE,    -- forget-streamer cascades
    moment_fp_id  BIGINT REFERENCES moment_fingerprints(id) ON DELETE SET NULL, -- NULL = unlinked (R5.2)
    title         TEXT,
    summary       TEXT,
    category      TEXT,                                 -- drama|funny|bans|records|esports|...
    category_conf REAL,
    state         TEXT NOT NULL DEFAULT 'developing',   -- developing|published|unverified|settled
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_story_clusters_entity ON story_clusters (entity_id);
CREATE INDEX idx_story_clusters_state ON story_clusters (state);

CREATE TABLE story_evidence (
    id            BIGSERIAL PRIMARY KEY,
    cluster_id    BIGINT NOT NULL REFERENCES story_clusters(id) ON DELETE CASCADE,
    item_id       BIGINT REFERENCES social_items(id) ON DELETE SET NULL,        -- survives item retention purge
    moment_fp_id  BIGINT REFERENCES moment_fingerprints(id) ON DELETE CASCADE,
    source_type   TEXT NOT NULL,                        -- registry key (R7.1)
    source_url    TEXT,                                 -- copied link so a receipt stays auditable after purge
    match_conf    REAL,                                 -- same_story_confidence (R4)
    weight        REAL,                                 -- reliability weight applied (R7)
    occurred_at   TIMESTAMPTZ,                          -- for the spread timeline (R8.4)
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_story_evidence_cluster ON story_evidence (cluster_id);
CREATE INDEX idx_story_evidence_occurred_at ON story_evidence (occurred_at);

CREATE TABLE story_scores (
    cluster_id    BIGINT PRIMARY KEY REFERENCES story_clusters(id) ON DELETE CASCADE,
    trend         REAL,                                 -- 0..100, NULL until trend history exists (Phase 2)
    volatility    REAL,                                 -- NULL until >=2 trend snapshots exist (Phase 2)
    confidence    TEXT,                                 -- canonical enum: single_source|corroborated|widely_reported (§10)
    sentiment     REAL,                                 -- -1..1, NULL until Phase 2 sentiment model
    factors       JSONB,                                -- explainability (R6.6)
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
