CREATE TABLE local_follows (
    login       VARCHAR(50) PRIMARY KEY,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_local_follows_created_at ON local_follows (created_at DESC);
