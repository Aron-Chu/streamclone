CREATE TABLE pulse_device_tokens (
    device_id TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    beta_principal_id TEXT NOT NULL,
    label TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ
);

CREATE INDEX idx_pulse_device_tokens_hash
    ON pulse_device_tokens (token_hash)
    WHERE revoked_at IS NULL;

CREATE INDEX idx_pulse_device_tokens_beta
    ON pulse_device_tokens (beta_principal_id);
