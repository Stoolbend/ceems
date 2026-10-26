CREATE TABLE templates (
    id        INTEGER PRIMARY KEY,
    name      TEXT    NOT NULL UNIQUE,
    tti       BLOB    NOT NULL,
    row_rules TEXT    NOT NULL DEFAULT '{}'    -- JSON object, "row" -> slot name
);
CREATE TABLE bindings (
    id          INTEGER PRIMARY KEY,
    name        TEXT    NOT NULL UNIQUE,
    template_id INTEGER NOT NULL REFERENCES templates(id),
    mpp         INTEGER NOT NULL UNIQUE CHECK (mpp BETWEEN 0x100 AND 0x8FF),
    source      TEXT    NOT NULL DEFAULT '{}', -- JSON; contents defined by the Sheets packet
    static      TEXT    NOT NULL DEFAULT '{}'  -- JSON object slot -> value, cells 0-127
);
