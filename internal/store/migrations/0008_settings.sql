CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
INSERT INTO settings (key, value) VALUES
    ('system_name', 'CeeMS'),
    ('vbit_conf', 'true'),
    -- Cell 0x03 (cyan) before the clock: vbit.conf writes it as ESC C, as in testdata/sample/vbit.conf.
    ('header_template', 'CeeMS   %%# %%a %e %%b ' || char(3) || '%H:%M:%S'),
    ('initial_teletext_page', '100'),
    ('row_adaptive_mode', 'false'),
    ('lines_per_field', '16'),
    ('magazine_priority', '9,1,3,3,3,9,9,9'),
    ('status_display', 'CeeMS');
