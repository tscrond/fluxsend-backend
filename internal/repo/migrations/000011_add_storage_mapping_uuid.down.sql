BEGIN;

ALTER TABLE files
DROP CONSTRAINT IF EXISTS files_storage_mapping_unique;

ALTER TABLE files
DROP COLUMN IF EXISTS storage_mapping;

COMMIT;