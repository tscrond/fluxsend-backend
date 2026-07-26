-- name: AbortFileUpload :one
UPDATE file_uploads
SET
  status = 'aborted',
  updated_at = now()
WHERE id = $1
  AND status = 'uploading'
RETURNING *;

-- name: CompleteFileUpload :one
UPDATE file_uploads
SET
  status = 'completed',
  uploaded_size = $2,
  updated_at = now()
WHERE id = $1
  AND status = 'uploading'
RETURNING id;

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

-- name: DeleteFileUploadPartsByUploadID :exec
DELETE FROM file_upload_parts
WHERE upload_id = $1;

-- name: FailFileUpload :one
UPDATE file_uploads
SET
  status = 'failed',
  updated_at = now()
WHERE id = $1
RETURNING *;

-- name: GetFileUploadById :one
SELECT * FROM file_uploads
WHERE id = $1
ORDER BY id
LIMIT 1;

-- name: ListFileUploadPartsByUploadID :many
SELECT * FROM file_upload_parts
WHERE upload_id = $1
ORDER BY part_number;

-- name: SaveFileUploadPart :one
WITH target AS (
  SELECT fu.id AS upload_id
  FROM file_uploads fu
  WHERE fu.id = $1
    AND fu.status = 'uploading'
), upsert AS (
  INSERT INTO file_upload_parts (
    upload_id,
    part_number,
    storage_metadata,
    size
  )
  SELECT
    target.upload_id,
    $2,
    $3,
    $4
  FROM target
  ON CONFLICT (upload_id, part_number)
  DO UPDATE
  SET
    storage_metadata = EXCLUDED.storage_metadata,
    size = EXCLUDED.size,
    created_at = now()
  RETURNING upload_id
)
UPDATE file_uploads fu
SET
  uploaded_size = COALESCE((
    SELECT SUM(fup.size)
    FROM file_upload_parts fup
    WHERE fup.upload_id = fu.id
  ), 0),
  updated_at = now()
WHERE fu.id IN (SELECT upload_id FROM upsert)
RETURNING fu.uploaded_size;