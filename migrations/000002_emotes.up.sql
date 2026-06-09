CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE emotes (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(64) NOT NULL,
    owner_id    VARCHAR(100),
    is_global   BOOLEAN NOT NULL DEFAULT false,
    flags       INT NOT NULL DEFAULT 0,
    animated    BOOLEAN NOT NULL DEFAULT false,
    mime_type   VARCHAR(20) NOT NULL DEFAULT 'image/webp',
    source_hash CHAR(64),
    status      SMALLINT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE emote_sets (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       VARCHAR(100) NOT NULL,
    owner_id   VARCHAR(100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE emote_set_items (
    id           BIGSERIAL PRIMARY KEY,
    emote_set_id UUID NOT NULL REFERENCES emote_sets(id) ON DELETE CASCADE,
    emote_id     UUID NOT NULL REFERENCES emotes(id) ON DELETE CASCADE,
    alias        VARCHAR(64),
    status       SMALLINT NOT NULL DEFAULT 1,
    added_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (emote_set_id, emote_id)
);

CREATE TABLE channels (
    twitch_id           VARCHAR(100) PRIMARY KEY,
    login               VARCHAR(50) NOT NULL UNIQUE,
    display_name        VARCHAR(100),
    active_emote_set_id UUID REFERENCES emote_sets(id) ON DELETE SET NULL,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE processing_jobs (
    id         BIGSERIAL PRIMARY KEY,
    emote_id   UUID NOT NULL REFERENCES emotes(id) ON DELETE CASCADE,
    source_key TEXT NOT NULL,
    state      SMALLINT NOT NULL DEFAULT 0,
    attempts   INT NOT NULL DEFAULT 0,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_emotes_name ON emotes (name);
CREATE INDEX idx_emotes_global ON emotes (is_global) WHERE is_global = true;
CREATE INDEX idx_set_items_set ON emote_set_items (emote_set_id);
CREATE INDEX idx_jobs_claimable ON processing_jobs (state, id) WHERE state IN (0, 2);
CREATE UNIQUE INDEX idx_jobs_idempotent ON processing_jobs (emote_id, source_key);
