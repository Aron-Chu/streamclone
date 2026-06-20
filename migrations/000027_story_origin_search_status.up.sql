ALTER TABLE story_clusters
    ADD COLUMN IF NOT EXISTS origin_search_status TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS origin_checked_at TIMESTAMPTZ;
