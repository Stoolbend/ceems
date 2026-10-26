CREATE TABLE api_keys (
    id           TEXT    PRIMARY KEY,          -- 16 lowercase hex chars, public
    secret_hash  BLOB    NOT NULL,             -- SHA-256 of the secret
    name         TEXT    NOT NULL,
    read_only    INTEGER NOT NULL CHECK (read_only IN (0, 1)),
    created_by   TEXT    NOT NULL,             -- actor, e.g. user:1
    created_at   TEXT    NOT NULL,
    expires_at   TEXT,                         -- NULL never expires
    revoked_at   TEXT,
    last_used_at TEXT
);
CREATE TABLE grants (
    id      INTEGER PRIMARY KEY,
    user_id INTEGER REFERENCES users(id)    ON DELETE CASCADE,
    key_id  TEXT    REFERENCES api_keys(id) ON DELETE CASCADE,
    lo      INTEGER NOT NULL,
    hi      INTEGER NOT NULL,
    access  TEXT    NOT NULL CHECK (access IN ('read', 'write')),
    CHECK ((user_id IS NULL) <> (key_id IS NULL)),
    CHECK (lo BETWEEN 0x100 AND 0x8FF AND hi BETWEEN lo AND 0x8FF)
);
CREATE INDEX grants_user ON grants (user_id);
CREATE INDEX grants_key  ON grants (key_id);
