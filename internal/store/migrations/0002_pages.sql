CREATE TABLE pages (
    mpp         INTEGER PRIMARY KEY CHECK (mpp BETWEEN 0x100 AND 0x8FF),
    tti         BLOB    NOT NULL,
    description TEXT    NOT NULL DEFAULT '',
    subpages    INTEGER NOT NULL,
    transmit    INTEGER NOT NULL CHECK (transmit IN (0, 1)),
    binding_id  INTEGER NULL,
    updated_at  TEXT    NOT NULL,
    updated_by  TEXT    NOT NULL
);
