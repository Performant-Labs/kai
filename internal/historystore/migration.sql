-- history database migration script (a dedicated migration file, separate from schema.sql)
-- Used only for "history store compatibility": appending new columns / indexes to the
-- existing history table.
-- Table creation itself lives in schema.sql; this file only holds ALTERs / extra indexes.
-- Note: SQLite has no `ADD COLUMN IF NOT EXISTS`, so a repeated ALTER reports a
-- "duplicate column" error. At runtime (store.go's Open) that error is ignored to stay
-- idempotent — a second launch / already-migrated store simply skips, without error.
-- When adding new columns later, append one ALTER TABLE line here — no Go code changes.

-- Backward compat: the history table was originally created without an engine_id column;
-- the column is added so the engine origin can be recorded.
ALTER TABLE history ADD COLUMN engine_id INTEGER NOT NULL DEFAULT 0;
