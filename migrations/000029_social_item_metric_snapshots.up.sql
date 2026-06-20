-- Per-item metric history for bounded quality heuristics such as sudden Reddit comment spikes.

CREATE TABLE IF NOT EXISTS social_item_metric_snapshots (
    item_id      BIGINT NOT NULL REFERENCES social_items(id) ON DELETE CASCADE,
    at           TIMESTAMPTZ NOT NULL,
    source       TEXT NOT NULL,
    external_id  TEXT NOT NULL,
    metrics      JSONB NOT NULL DEFAULT '{}'::jsonb,
    comments     INTEGER,
    PRIMARY KEY (item_id, at)
);

CREATE INDEX IF NOT EXISTS idx_social_item_metric_snapshots_item_at
    ON social_item_metric_snapshots (item_id, at DESC);

CREATE INDEX IF NOT EXISTS idx_social_item_metric_snapshots_source_at
    ON social_item_metric_snapshots (source, at DESC);
