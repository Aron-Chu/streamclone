DROP INDEX IF EXISTS idx_story_clusters_story_class;

ALTER TABLE story_clusters
    DROP CONSTRAINT IF EXISTS story_clusters_story_class_check;

ALTER TABLE story_clusters
    DROP COLUMN IF EXISTS story_class;
