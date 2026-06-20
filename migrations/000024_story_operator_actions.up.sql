CREATE TABLE story_operator_actions (
    id          BIGSERIAL PRIMARY KEY,
    cluster_id  BIGINT NOT NULL REFERENCES story_clusters(id) ON DELETE CASCADE,
    action      TEXT NOT NULL,
    operator    TEXT NOT NULL DEFAULT 'operator',
    note        TEXT,
    before_data JSONB NOT NULL DEFAULT '{}'::jsonb,
    after_data  JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_story_operator_actions_cluster_created
    ON story_operator_actions (cluster_id, created_at DESC);
