-- history database migration script (a dedicated migration file, separate from schema.sql)
-- Used only for "history store compatibility": appending new columns / indexes to the
-- existing history table.
-- Table creation itself lives in schema.sql, and this file only holds ALTERs / extra indexes.
-- Note: SQLite has no `ADD COLUMN IF NOT EXISTS`, so a repeated ALTER reports a
-- "duplicate column" error. At runtime (store.go's Open) that error is ignored to stay
-- idempotent — a second launch / already-migrated store simply skips, without error.
-- When adding new columns later, append one ALTER TABLE line here — no Go code changes.
-- IMPORTANT: the loader splits this whole file on the semicolon character and runs each piece
-- as its own statement, so a comment in this file must never contain one. A piece that starts
-- mid-sentence fails to parse, and then the history store cannot be opened at all.

-- Backward compat: the history table was originally created without an engine_id column,
-- so the column is added to record the engine origin.
ALTER TABLE history ADD COLUMN engine_id INTEGER NOT NULL DEFAULT 0;
