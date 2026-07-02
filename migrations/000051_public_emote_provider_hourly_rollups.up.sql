-- Public emote provider hourly rollups (aggregate-only; no per-stream identity fields).
CREATE TABLE IF NOT EXISTS public_emote_provider_hourly_rollups (
    bucket_hour TIMESTAMPTZ NOT NULL,
    corpus_key TEXT NOT NULL DEFAULT '__all__',
    provider TEXT NOT NULL,
    total_uses BIGINT NOT NULL CHECK (total_uses >= 0),
    tracked_minutes BIGINT NOT NULL CHECK (tracked_minutes >= 0),
    emote_minutes BIGINT NOT NULL CHECK (emote_minutes >= 0),
    coverage_pct DOUBLE PRECISION NOT NULL CHECK (coverage_pct >= 0 AND coverage_pct <= 100),
    confidence DOUBLE PRECISION NOT NULL CHECK (confidence >= 0 AND confidence <= 100),
    PRIMARY KEY (bucket_hour, corpus_key, provider)
);

CREATE INDEX IF NOT EXISTS idx_public_emote_provider_hourly_rollups_bucket
    ON public_emote_provider_hourly_rollups (bucket_hour DESC);
