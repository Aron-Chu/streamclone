ALTER TABLE story_clusters
    DROP COLUMN IF EXISTS origin_checked_at,
    DROP COLUMN IF EXISTS origin_search_status;
