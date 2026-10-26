CREATE TABLE page_versions (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    mpp        INTEGER NOT NULL,
    tti        BLOB    NOT NULL,
    updated_at TEXT    NOT NULL,   -- the retired row's own updated_at
    updated_by TEXT    NOT NULL    -- and updated_by
);
CREATE INDEX page_versions_mpp ON page_versions (mpp, id);
