-- StreamerBans reliability weight for Pulse Wire ban ingest.

INSERT INTO source_reliability (source_type, source_risk, confidence_weight) VALUES
    ('streamerbans_post', 'unofficial', 0.72)
ON CONFLICT (source_type) DO NOTHING;
