ALTER TABLE story_clusters
    ADD COLUMN story_class TEXT;

ALTER TABLE story_clusters
    ADD CONSTRAINT story_clusters_story_class_check
    CHECK (
        story_class IS NULL OR story_class IN (
            'clip_moment',
            'drama_claim',
            'ban_event',
            'creator_news',
            'viewer_mover',
            'community_meta',
            'unlinked_evidence',
            'not_news',
            'debunked'
        )
    );

CREATE INDEX idx_story_clusters_story_class ON story_clusters (story_class)
    WHERE story_class IS NOT NULL;
