CREATE TABLE IF NOT EXISTS top500_channels (
    channel_id TEXT PRIMARY KEY,
    login TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL DEFAULT '',
    rank INTEGER NOT NULL CHECK (rank > 0),
    source TEXT NOT NULL CHECK (source IN ('operator_seed', 'configured')),
    source_version TEXT NOT NULL DEFAULT '',
    seeded_by TEXT NOT NULL DEFAULT '',
    effective_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    source_metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    enabled BOOLEAN NOT NULL DEFAULT false,
    last_seen_at TIMESTAMPTZ,
    last_sampled_at TIMESTAMPTZ,
    last_live_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (jsonb_typeof(source_metadata) = 'object')
);

CREATE INDEX IF NOT EXISTS idx_top500_channels_enabled_rank
    ON top500_channels (enabled, rank);

CREATE INDEX IF NOT EXISTS idx_top500_channels_source_rank
    ON top500_channels (source, rank);

CREATE INDEX IF NOT EXISTS idx_top500_channels_last_sampled
    ON top500_channels (last_sampled_at);

CREATE TABLE IF NOT EXISTS top500_live_snapshots (
    id BIGSERIAL NOT NULL,
    channel_id TEXT NOT NULL,
    login TEXT NOT NULL,
    stream_id TEXT,
    is_live BOOLEAN NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    category_id TEXT NOT NULL DEFAULT '',
    category_name TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ,
    viewer_count INTEGER CHECK (viewer_count IS NULL OR viewer_count >= 0),
    language TEXT NOT NULL DEFAULT '',
    tags JSONB NOT NULL DEFAULT '[]'::jsonb,
    sample_tick_at TIMESTAMPTZ NOT NULL,
    sampled_at TIMESTAMPTZ NOT NULL,
    source TEXT NOT NULL CHECK (source IN ('helix_streams', 'helix_users', 'cache', 'top500_metadata', 'configured')),
    ingested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    failure_code TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (id, sample_tick_at),
    UNIQUE (channel_id, sample_tick_at),
    CHECK (jsonb_typeof(tags) = 'array')
) PARTITION BY RANGE (sample_tick_at);

DO $$
DECLARE
    partition_day DATE;
    partition_name TEXT;
BEGIN
    FOR partition_day IN
        SELECT generate_series(current_date - 1, current_date + 14, interval '1 day')::date
    LOOP
        partition_name := format('top500_live_snapshots_%s', to_char(partition_day, 'YYYYMMDD'));
        EXECUTE format(
            'CREATE TABLE IF NOT EXISTS %I PARTITION OF top500_live_snapshots FOR VALUES FROM (%L) TO (%L)',
            partition_name,
            partition_day::timestamptz,
            (partition_day + 1)::timestamptz
        );
    END LOOP;
END $$;

CREATE TABLE IF NOT EXISTS top500_live_snapshots_default
    PARTITION OF top500_live_snapshots DEFAULT;

CREATE INDEX IF NOT EXISTS idx_top500_live_snapshots_sample_tick
    ON top500_live_snapshots (sample_tick_at);

CREATE INDEX IF NOT EXISTS idx_top500_live_snapshots_channel_tick
    ON top500_live_snapshots (channel_id, sample_tick_at DESC);

CREATE INDEX IF NOT EXISTS idx_top500_live_snapshots_stream_tick
    ON top500_live_snapshots (stream_id, sample_tick_at DESC);

CREATE INDEX IF NOT EXISTS idx_top500_live_snapshots_live_tick
    ON top500_live_snapshots (is_live, sample_tick_at DESC);

CREATE TABLE IF NOT EXISTS top500_current (
    channel_id TEXT PRIMARY KEY,
    login TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL DEFAULT '',
    rank INTEGER NOT NULL CHECK (rank > 0),
    coverage_source TEXT NOT NULL DEFAULT 'top500_metadata' CHECK (coverage_source IN ('top500_metadata', 'tier0', 'collector', 'cache', 'helix')),
    is_live BOOLEAN NOT NULL DEFAULT false,
    stream_id TEXT,
    title TEXT NOT NULL DEFAULT '',
    category_id TEXT NOT NULL DEFAULT '',
    category_name TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ,
    viewer_count INTEGER CHECK (viewer_count IS NULL OR viewer_count >= 0),
    language TEXT NOT NULL DEFAULT '',
    tags JSONB NOT NULL DEFAULT '[]'::jsonb,
    sampled_at TIMESTAMPTZ NOT NULL,
    stale_after TIMESTAMPTZ NOT NULL,
    last_success_at TIMESTAMPTZ,
    last_error_code TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (jsonb_typeof(tags) = 'array')
);

CREATE INDEX IF NOT EXISTS idx_top500_current_rank
    ON top500_current (rank);

CREATE INDEX IF NOT EXISTS idx_top500_current_live_sampled
    ON top500_current (is_live, sampled_at DESC);

CREATE INDEX IF NOT EXISTS idx_top500_current_stale_after
    ON top500_current (stale_after);
