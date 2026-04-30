-- name: CreateWorkspaceFile :one
INSERT INTO workspace_files (id, workspace_id, uploaded_by, file_name, file_type, size, md5_checksum, path)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, workspace_id, uploaded_by, file_name, file_type, size, md5_checksum, path, created_at;

-- name: CreateWorkspaceFolder :one
INSERT INTO workspace_files (id, workspace_id, uploaded_by, file_name, file_type, size, path)
VALUES ($1, $2, $3, $4, 'inode/directory', 0, $5)
RETURNING id, workspace_id, uploaded_by, file_name, file_type, size, md5_checksum, path, created_at;

-- name: GetWorkspaceFiles :many
SELECT id, workspace_id, uploaded_by, file_name, file_type, size, md5_checksum, path, created_at
FROM workspace_files WHERE workspace_id = $1;

-- name: GetWorkspaceFilesByPath :many
SELECT id, workspace_id, uploaded_by, file_name, file_type, size, md5_checksum, path, created_at
FROM workspace_files WHERE workspace_id = $1 AND path = $2;

-- name: GetWorkspaceFileByName :one
SELECT id, file_name, md5_checksum, path
FROM workspace_files
WHERE workspace_id = $1 AND path = $2 AND file_name = $3;

-- name: GetWorkspaceFileById :one
SELECT id, workspace_id, uploaded_by, file_name, file_type, size, md5_checksum, path, created_at
FROM workspace_files WHERE id = $1 AND workspace_id = $2;

-- name: GetWorkspaceFoldersByPath :many
SELECT id, workspace_id, uploaded_by, file_name, file_type, size, md5_checksum, path, created_at
FROM workspace_files
WHERE workspace_id = $1 AND path = $2 AND file_type = 'inode/directory';

-- name: GetWorkspaceFolderByPathAndName :one
SELECT id, workspace_id, uploaded_by, file_name, file_type, size, md5_checksum, path, created_at
FROM workspace_files
WHERE workspace_id = $1 AND path = $2 AND file_name = $3 AND file_type = 'inode/directory';

-- name: DeleteWorkspaceFileById :exec
DELETE FROM workspace_files WHERE id = $1 AND workspace_id = $2;

-- name: DeleteWorkspaceFilesByPath :exec
DELETE FROM workspace_files WHERE workspace_id = $1 AND path = $2;

-- name: DeleteWorkspaceFilesByPathPrefix :exec
DELETE FROM workspace_files WHERE workspace_id = $1 AND path LIKE $2;

-- name: UpdateWorkspaceFileName :exec
UPDATE workspace_files
SET file_name = $1
WHERE workspace_id = $2 AND path = $3 AND file_name = $4;

-- name: MoveWorkspaceFile :exec
UPDATE workspace_files
SET path = $1
WHERE id = $2 AND workspace_id = $3;

-- name: UpdateWorkspaceFolderLocation :exec
UPDATE workspace_files
SET path = $1, file_name = $2
WHERE id = $3 AND workspace_id = $4;

-- name: MoveWorkspaceFilesByPathPrefix :exec
UPDATE workspace_files
SET path = regexp_replace(path, $1, $2)
WHERE workspace_id = $3 AND path LIKE $4;

