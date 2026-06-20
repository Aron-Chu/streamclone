-- Pulse Wire evidence previews and source embeds.
-- Stores safe, bounded metadata for source URLs shown on story detail pages.

CREATE TABLE evidence_previews (
    id              BIGSERIAL PRIMARY KEY,
    canonical_url   TEXT NOT NULL UNIQUE,
    platform        TEXT NOT NULL,
    provider_name   TEXT,
    title           TEXT,
    author          TEXT,
    thumbnail_url   TEXT,
    embed_url       TEXT,
    embed_html      TEXT,
    created_at_src  TIMESTAMPTZ,
    fetched_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    http_status     INTEGER,
    error           TEXT,
    retry_count     INTEGER NOT NULL DEFAULT 0,
    next_fetch_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at      TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_evidence_previews_platform ON evidence_previews (platform);
CREATE INDEX idx_evidence_previews_expires_at ON evidence_previews (expires_at);
CREATE INDEX idx_evidence_previews_next_fetch_at ON evidence_previews (next_fetch_at);

CREATE TABLE story_evidence_previews (
    cluster_id   BIGINT NOT NULL REFERENCES story_clusters(id) ON DELETE CASCADE,
    evidence_id  BIGINT REFERENCES story_evidence(id) ON DELETE CASCADE,
    preview_id   BIGINT NOT NULL REFERENCES evidence_previews(id) ON DELETE CASCADE,
    match_kind   TEXT NOT NULL DEFAULT 'url',
    note         TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (cluster_id, preview_id)
);

CREATE INDEX idx_story_evidence_previews_evidence ON story_evidence_previews (evidence_id);
