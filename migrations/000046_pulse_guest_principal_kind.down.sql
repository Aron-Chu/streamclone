ALTER TABLE pulse_bookmarks
    DROP CONSTRAINT IF EXISTS pulse_bookmarks_principal_kind_check;
ALTER TABLE pulse_bookmarks
    ADD CONSTRAINT pulse_bookmarks_principal_kind_check CHECK (
        principal_kind IS NULL OR principal_kind IN ('beta', 'device', 'user')
    );

ALTER TABLE pulse_watchlist
    DROP CONSTRAINT IF EXISTS pulse_watchlist_principal_kind_check;
ALTER TABLE pulse_watchlist
    ADD CONSTRAINT pulse_watchlist_principal_kind_check CHECK (
        principal_kind IN ('beta', 'device', 'user')
    );
