-- name: CreateFileUpload :one
INSERT INTO file_uploads
(
  owner_id,
  storage_backend,
  storage_upload_id,
  storage_mapping,
  file_name,
  file_type,
  expected_size,
  status
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: UpdateFileUploadParts :exec
INSERT INTO file_upload_parts (
    upload_id,
    part_number,
    storage_metadata,
    size
)
VALUES ($1, $2, $3, $4)
ON CONFLICT (upload_id, part_number)
DO UPDATE
SET
    storage_metadata = EXCLUDED.storage_metadata,
    size = EXCLUDED.size,
    created_at = now();

-- name: GetFileUploadById :one
SELECT * FROM file_uploads
WHERE id = $1
ORDER BY id
LIMIT 1;

-- name: ListFileUploadPartsByUploadID :many
SELECT * FROM file_upload_parts
WHERE upload_id = $1
ORDER BY part_number;

-- name: CompleteFileUpload :exec
UPDATE file_uploads
SET
  status = 'completed',
  uploaded_size = $2,
  updated_at = now()
WHERE id = $1;