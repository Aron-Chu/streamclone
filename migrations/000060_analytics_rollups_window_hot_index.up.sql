CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_analytics_rollups_window_hot
    ON analytics_minute_rollups (minute_ts, chat_count DESC)
    WHERE chat_count > 0;
