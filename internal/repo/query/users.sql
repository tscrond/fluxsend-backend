-- name: CreateUser :one
INSERT INTO users (user_email)
VALUES ($1)
RETURNING *;

-- name: GetUserById :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE user_email = $1;

-- name: GetUserBucketById :one
SELECT user_bucket FROM users WHERE id = $1;

-- name: UpdateUserBucketNameById :exec
UPDATE users SET user_bucket = $1 WHERE id = $2;

-- name: DeleteAccount :one
DELETE FROM users WHERE id = $1 RETURNING *;

-- name: ListWorkspaceObjectKeysByUploader :many
SELECT workspace_id, id AS object_id
FROM workspace_files
WHERE uploaded_by = $1
  AND file_type IS DISTINCT FROM 'inode/directory'
UNION
SELECT workspace_id, storage_mapping AS object_id
FROM file_uploads
WHERE owner_id = $1
  AND workspace_id IS NOT NULL;
