-- Optional pgvector semantic columns (pulse-wire-semantic profile only)
CREATE EXTENSION IF NOT EXISTS vector;
ALTER TABLE social_items        ADD COLUMN IF NOT EXISTS embedding vector(384);
ALTER TABLE moment_fingerprints ADD COLUMN IF NOT EXISTS embedding vector(384);
