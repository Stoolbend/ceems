CREATE TABLE runs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    binding_id  INTEGER NOT NULL REFERENCES bindings(id) ON DELETE CASCADE,
    actor       TEXT    NOT NULL,              -- 'schedule' or the user:<id> who pressed Run now
    started_at  TEXT    NOT NULL,
    finished_at TEXT    NOT NULL,
    outcome     TEXT    NOT NULL CHECK (outcome IN ('ok', 'failed')),
    error       TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX runs_binding ON runs (binding_id, id);
