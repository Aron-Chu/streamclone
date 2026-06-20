-- Extend Pulse Wire source reliability rows for current link-only and context sources.

INSERT INTO source_reliability (source_type, source_risk, confidence_weight) VALUES
    ('tiktok_video',   'public_api', 0.60),
    ('instagram_post', 'public_api', 0.55),
    ('kick_clip',      'public_api', 0.55)
ON CONFLICT (source_type) DO UPDATE SET
    source_risk = EXCLUDED.source_risk,
    confidence_weight = EXCLUDED.confidence_weight;
