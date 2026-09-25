-- engines.id is the primary key; history.engine_id references this id across databases (no
-- FK constraint across databases).
CREATE TABLE IF NOT EXISTS engines (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    engine   TEXT    NOT NULL UNIQUE,
    enabled  INTEGER NOT NULL DEFAULT 0,
    api_key  TEXT    NOT NULL DEFAULT '',
    secret   TEXT    NOT NULL DEFAULT '',
    extra    TEXT    NOT NULL DEFAULT '',
    endpoint TEXT    NOT NULL DEFAULT ''
);
