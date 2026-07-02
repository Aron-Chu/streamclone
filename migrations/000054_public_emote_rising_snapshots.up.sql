-- Emote Atlas rising emote snapshots (retired; kept for migration chain parity).
CREATE TABLE IF NOT EXISTS public_emote_rising_snapshots (
    snapshot_id BIGSERIAL PRIMARY KEY,
    range_value TEXT NOT NULL,
    captured_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    payload_json JSONB NOT NULL DEFAULT '{}'::jsonb
);
