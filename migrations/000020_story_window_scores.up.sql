-- Window-native Pulse Wire ranking read model.
-- Evidence is still stored once in story_evidence; this table materializes
-- today/24h/7d scores so feed semantics are stable for API consumers.

CREATE TABLE story_window_scores (
    cluster_id        BIGINT NOT NULL REFERENCES story_clusters(id) ON DELETE CASCADE,
    "window"          TEXT NOT NULL CHECK ("window" IN ('today', '24h', '7d')),
    since             TIMESTAMPTZ NOT NULL,
    evidence_count    INTEGER NOT NULL DEFAULT 0,
    source_count      INTEGER NOT NULL DEFAULT 0,
    velocity_score    REAL NOT NULL DEFAULT 0,
    credibility_score REAL NOT NULL DEFAULT 0,
    impact_score      REAL NOT NULL DEFAULT 0,
    momentum_score    REAL NOT NULL DEFAULT 0,
    freshness_score   REAL NOT NULL DEFAULT 0,
    rank_score        REAL NOT NULL DEFAULT 0,
    dominant_source   TEXT,
    computed_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (cluster_id, "window")
);

CREATE INDEX idx_story_window_scores_window_rank ON story_window_scores ("window", rank_score DESC, computed_at DESC);
CREATE INDEX idx_story_window_scores_cluster ON story_window_scores (cluster_id);
