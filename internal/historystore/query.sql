-- name: InsertHistory :execlastid
INSERT INTO history (text, result, from_lang, to_lang, engine_id, from_ocr, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: QueryHistory :many
SELECT id, text, result, from_lang, to_lang, engine_id, from_ocr, created_at
FROM history
WHERE (sqlc.arg(keyword) = '' OR text LIKE '%' || sqlc.arg(keyword) || '%' OR result LIKE '%' || sqlc.arg(keyword) || '%')
  AND (sqlc.arg(from_lang) = '' OR from_lang = sqlc.arg(from_lang))
  AND (sqlc.arg(to_lang) = '' OR to_lang = sqlc.arg(to_lang))
  AND (sqlc.arg(engine_id) = 0 OR engine_id = sqlc.arg(engine_id))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountHistory :one
SELECT COUNT(*) FROM history
WHERE (sqlc.arg(keyword) = '' OR text LIKE '%' || sqlc.arg(keyword) || '%' OR result LIKE '%' || sqlc.arg(keyword) || '%')
  AND (sqlc.arg(from_lang) = '' OR from_lang = sqlc.arg(from_lang))
  AND (sqlc.arg(to_lang) = '' OR to_lang = sqlc.arg(to_lang))
  AND (sqlc.arg(engine_id) = 0 OR engine_id = sqlc.arg(engine_id));

-- name: DistinctPairs :many
SELECT DISTINCT from_lang, to_lang FROM history ORDER BY from_lang, to_lang;

-- name: DistinctEngineIDs :many
SELECT DISTINCT engine_id FROM history ORDER BY engine_id;

-- name: DeleteHistory :exec
DELETE FROM history WHERE id = ?;

-- name: ClearHistory :exec
DELETE FROM history;

-- name: FindDuplicate :one
SELECT id FROM history
WHERE text = ? AND engine_id = ? AND from_lang = ? AND to_lang = ? AND from_ocr = ?
LIMIT 1;
