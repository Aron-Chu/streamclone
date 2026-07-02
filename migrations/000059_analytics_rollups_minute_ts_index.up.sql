CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_analytics_rollups_minute_ts
    ON analytics_minute_rollups (minute_ts);
