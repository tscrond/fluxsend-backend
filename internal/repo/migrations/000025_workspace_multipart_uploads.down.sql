DROP INDEX IF EXISTS file_uploads_workspace_id_idx;

ALTER TABLE file_uploads
    DROP COLUMN IF EXISTS path,
    DROP COLUMN IF EXISTS workspace_id;