ALTER TABLE emote_set_items
    ALTER COLUMN alias TYPE VARCHAR(64) USING left(alias, 64);

ALTER TABLE emotes
    ALTER COLUMN name TYPE VARCHAR(64) USING left(name, 64);
