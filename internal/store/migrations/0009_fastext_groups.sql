CREATE TABLE fastext_groups (
    id         INTEGER PRIMARY KEY,
    name       TEXT    NOT NULL UNIQUE,
    paged      INTEGER NOT NULL CHECK (paged IN (0, 1)),
    links      TEXT    NOT NULL,          -- red,green,yellow,cyan: four 3-hex links, lower case
    prev_slot  INTEGER CHECK (prev_slot BETWEEN 0 AND 3),
    next_slot  INTEGER CHECK (next_slot BETWEEN 0 AND 3),
    is_default INTEGER NOT NULL DEFAULT 0 CHECK (is_default IN (0, 1)),
    CHECK (paged = 0 OR (prev_slot IS NOT NULL AND next_slot IS NOT NULL AND prev_slot <> next_slot))
);
CREATE UNIQUE INDEX fastext_groups_default ON fastext_groups (is_default) WHERE is_default = 1;
ALTER TABLE pages ADD COLUMN fastext_group_id INTEGER NULL REFERENCES fastext_groups(id) ON DELETE SET NULL;
ALTER TABLE pages ADD COLUMN fastext_order    INTEGER NULL;
INSERT INTO settings (key, value) VALUES ('index_page_enabled', 'false'), ('index_page', '100');
