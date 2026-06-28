-- Emote history phase 1a: provider snapshots, alias history, and usage rollups.
CREATE TABLE IF NOT EXISTS channel_emote_set_snapshots (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    twitch_id       TEXT        NOT NULL,
    login           TEXT        NOT NULL DEFAULT '',
    provider        TEXT        NOT NULL CHECK (provider = lower(provider)),
    provider_set_id TEXT        NOT NULL DEFAULT '',
    fetched_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    effective_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    item_count      INT         NOT NULL DEFAULT 0,
    snapshot_hash   TEXT        NOT NULL,
    state           TEXT        NOT NULL DEFAULT 'complete'
        CHECK (state IN ('complete', 'unchanged', 'failed', 'partial', 'rate_limited')),
    source          TEXT        NOT NULL DEFAULT 'snapshot_poll',
    schema_version  INT         NOT NULL DEFAULT 1,
    http_status     INT,
    error           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_emote_snapshots_channel_provider_time
    ON channel_emote_set_snapshots (twitch_id, provider, fetched_at DESC);
CREATE INDEX IF NOT EXISTS idx_emote_snapshots_provider_hash
    ON channel_emote_set_snapshots (provider, snapshot_hash);

CREATE TABLE IF NOT EXISTS channel_emote_set_snapshot_items (
    snapshot_id       UUID        NOT NULL REFERENCES channel_emote_set_snapshots(id) ON DELETE CASCADE,
    twitch_id         TEXT        NOT NULL,
    provider          TEXT        NOT NULL,
    provider_emote_id TEXT        NOT NULL,
    provider_set_id   TEXT        NOT NULL DEFAULT '',
    alias             TEXT        NOT NULL,
    canonical_name    TEXT        NOT NULL DEFAULT '',
    source_url        TEXT        NOT NULL DEFAULT '',
    asset_hash        TEXT        NOT NULL DEFAULT '',
    flags             INT         NOT NULL DEFAULT 0,
    animated          BOOLEAN     NOT NULL DEFAULT false,
    zero_width        BOOLEAN     NOT NULL DEFAULT false,
    sort_key          TEXT        NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (snapshot_id, provider, provider_emote_id, alias)
);

CREATE INDEX IF NOT EXISTS idx_emote_snapshot_items_channel_provider
    ON channel_emote_set_snapshot_items (twitch_id, provider, provider_emote_id);
CREATE INDEX IF NOT EXISTS idx_emote_snapshot_items_provider_identity
    ON channel_emote_set_snapshot_items (provider, provider_emote_id);

CREATE TABLE IF NOT EXISTS emote_alias_history (
    id                 BIGSERIAL   PRIMARY KEY,
    twitch_id          TEXT        NOT NULL,
    login              TEXT        NOT NULL DEFAULT '',
    provider           TEXT        NOT NULL,
    provider_emote_id  TEXT        NOT NULL,
    alias              TEXT        NOT NULL,
    valid_from         TIMESTAMPTZ NOT NULL,
    valid_to           TIMESTAMPTZ,
    first_seen_by_us   TIMESTAMPTZ NOT NULL,
    last_seen_by_us    TIMESTAMPTZ NOT NULL,
    opened_snapshot_id UUID        REFERENCES channel_emote_set_snapshots(id) ON DELETE SET NULL,
    closed_snapshot_id UUID        REFERENCES channel_emote_set_snapshots(id) ON DELETE SET NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (valid_to IS NULL OR valid_to >= valid_from)
);

CREATE INDEX IF NOT EXISTS idx_emote_alias_channel_provider_period
    ON emote_alias_history (twitch_id, provider, valid_from DESC, valid_to);
CREATE INDEX IF NOT EXISTS idx_emote_alias_lookup_asof
    ON emote_alias_history (twitch_id, provider, alias, valid_from DESC, valid_to);
CREATE UNIQUE INDEX IF NOT EXISTS idx_emote_alias_one_open
    ON emote_alias_history (twitch_id, provider, provider_emote_id, alias)
    WHERE valid_to IS NULL;
CREATE INDEX IF NOT EXISTS idx_emote_alias_provider_identity
    ON emote_alias_history (provider, provider_emote_id);

CREATE TABLE IF NOT EXISTS emote_usage_minute_rollups (
    stream_id           TEXT           NOT NULL REFERENCES analytics_streams(stream_id) ON DELETE CASCADE,
    minute_ts           TIMESTAMPTZ    NOT NULL,
    twitch_id           TEXT           NOT NULL,
    login               TEXT           NOT NULL DEFAULT '',
    provider            TEXT           NOT NULL DEFAULT '',
    provider_emote_id   TEXT           NOT NULL DEFAULT '',
    emote_name          TEXT           NOT NULL DEFAULT '',
    local_emote_id      UUID           REFERENCES emotes(id) ON DELETE SET NULL,
    use_count           INT            NOT NULL DEFAULT 0,
    identity_resolution TEXT           NOT NULL
        CHECK (identity_resolution IN ('provider_id', 'alias_fallback', 'ambiguous', 'unresolved')),
    confidence          NUMERIC(5,4)   NOT NULL DEFAULT 0,
    source_key          TEXT           NOT NULL,
    created_at          TIMESTAMPTZ    NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ    NOT NULL DEFAULT now(),
    PRIMARY KEY (stream_id, minute_ts, source_key)
);

CREATE INDEX IF NOT EXISTS idx_emote_usage_minute_channel_day
    ON emote_usage_minute_rollups (login, minute_ts DESC);
CREATE INDEX IF NOT EXISTS idx_emote_usage_minute_provider_identity
    ON emote_usage_minute_rollups (provider, provider_emote_id);
CREATE INDEX IF NOT EXISTS idx_emote_usage_minute_stream_time
    ON emote_usage_minute_rollups (stream_id, minute_ts);

CREATE TABLE IF NOT EXISTS emote_usage_stream_rollups (
    stream_id           TEXT           NOT NULL REFERENCES analytics_streams(stream_id) ON DELETE CASCADE,
    twitch_id           TEXT           NOT NULL,
    login               TEXT           NOT NULL DEFAULT '',
    provider            TEXT           NOT NULL DEFAULT '',
    provider_emote_id   TEXT           NOT NULL DEFAULT '',
    emote_name          TEXT           NOT NULL DEFAULT '',
    local_emote_id      UUID           REFERENCES emotes(id) ON DELETE SET NULL,
    use_count           BIGINT         NOT NULL DEFAULT 0,
    minutes_seen        INT            NOT NULL DEFAULT 0,
    first_minute_ts     TIMESTAMPTZ,
    last_minute_ts      TIMESTAMPTZ,
    identity_resolution TEXT           NOT NULL
        CHECK (identity_resolution IN ('provider_id', 'alias_fallback', 'ambiguous', 'unresolved')),
    confidence          NUMERIC(5,4)   NOT NULL DEFAULT 0,
    updated_at          TIMESTAMPTZ    NOT NULL DEFAULT now(),
    PRIMARY KEY (stream_id, provider, provider_emote_id, emote_name, identity_resolution)
);

CREATE INDEX IF NOT EXISTS idx_emote_usage_stream_channel_provider
    ON emote_usage_stream_rollups (login, provider, provider_emote_id);
CREATE INDEX IF NOT EXISTS idx_emote_usage_stream_channel_updated
    ON emote_usage_stream_rollups (login, updated_at DESC);
