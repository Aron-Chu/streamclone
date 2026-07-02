-- Emote Atlas culture section snapshots (retired; kept for migration chain parity).
CREATE TABLE IF NOT EXISTS emote_culture_sections (
    section_id BIGSERIAL PRIMARY KEY,
    section_key TEXT NOT NULL,
    captured_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    payload_json JSONB NOT NULL DEFAULT '{}'::jsonb
);
