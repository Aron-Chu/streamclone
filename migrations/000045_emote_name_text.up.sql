-- Widen emote name columns to TEXT for long provider aliases and unicode-safe storage.
ALTER TABLE emotes
    ALTER COLUMN name TYPE TEXT;

ALTER TABLE emote_set_items
    ALTER COLUMN alias TYPE TEXT;
