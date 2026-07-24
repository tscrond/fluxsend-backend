DROP TABLE IF EXISTS file_upload_parts;

DROP INDEX IF EXISTS file_uploads_status_idx;
DROP INDEX IF EXISTS file_uploads_owner_id_idx;
DROP INDEX IF EXISTS file_uploads_storage_mapping_unique;

DROP TABLE IF EXISTS file_uploads;