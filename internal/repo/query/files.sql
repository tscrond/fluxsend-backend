-- name: InsertFile :one
INSERT INTO files (owner_google_id, file_name, file_type, size, md5_checksum, private_download_token)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- -- name: InsertFileReturningID :one
-- INSERT INTO files (owner_google_id, file_name, file_type, size, md5_checksum, private_download_token)
-- VALUES ($1, $2, $3, $4, $5, $6)
-- ON CONFLICT (owner_google_id, md5_checksum) DO NOTHING
-- RETURNING id;

-- name: GetFilesByOwner :many
SELECT * FROM files WHERE owner_google_id = $1;

-- name: GetFileByOwnerAndName :one
SELECT id, md5_checksum
FROM files
WHERE owner_google_id = $1 AND file_name = $2;

-- name: GetFileById :one
SELECT * FROM files WHERE id = $1;

-- name: GetFileIdFromToken :one
SELECT id FROM files WHERE private_download_token = $1;

-- name: DeleteFileByNameAndId :exec
DELETE FROM files WHERE owner_google_id = $1 AND file_name = $2;

-- name: GetFileFromChecksum :one
SELECT id FROM files WHERE md5_checksum = $1;

-- name: UpdateFileNameByOwnerAndName :exec
UPDATE files
SET file_name = $1
WHERE owner_google_id = $2 AND file_name = $3;
