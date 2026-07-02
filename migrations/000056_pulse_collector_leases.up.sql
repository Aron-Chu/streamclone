-- Exclusive IRC collector ownership per login (hosted multi-collector coordination).
CREATE TABLE IF NOT EXISTS pulse_collector_leases (
    login TEXT PRIMARY KEY,
    stream_id TEXT NOT NULL DEFAULT '',
    collector_instance_id TEXT NOT NULL,
    priority INT NOT NULL DEFAULT 0,
    shard_index INT NOT NULL DEFAULT 0,
    shard_count INT NOT NULL DEFAULT 1,
    state TEXT NOT NULL DEFAULT 'claimed',
    claimed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    heartbeat_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_pulse_collector_leases_expires
    ON pulse_collector_leases (expires_at);
