-- name: ListFileIDsWithoutPrivateToken :many
SELECT id FROM files
WHERE private_download_token IS NULL;

-- name: UpdatePrivateDownloadToken :exec
UPDATE files
SET private_download_token = $1
WHERE id = $2;

-- name: GetPrivateDownloadTokenByFileName :one 
SELECT private_download_token FROM files WHERE file_name = $1 AND owner_id = $2;
