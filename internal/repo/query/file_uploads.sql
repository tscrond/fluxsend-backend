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