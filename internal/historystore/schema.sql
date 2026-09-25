-- engine_id references config.db's engines.id (cross-database, no FK constraint).
CREATE TABLE IF NOT EXISTS history (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    text       TEXT    NOT NULL,
    result     TEXT    NOT NULL,
    from_lang  TEXT    NOT NULL DEFAULT '',
    to_lang    TEXT    NOT NULL DEFAULT '',
    engine_id  INTEGER NOT NULL DEFAULT 0,
    from_ocr   INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_history_created ON history (created_at DESC);
