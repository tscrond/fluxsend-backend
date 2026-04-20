BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

ALTER TABLE files
ADD COLUMN IF NOT EXISTS storage_mapping UUID DEFAULT gen_random_uuid();

UPDATE files
SET storage_mapping = gen_random_uuid()
WHERE storage_mapping IS NULL;

ALTER TABLE files
ALTER COLUMN storage_mapping SET NOT NULL;

DO $$
BEGIN
	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'files_storage_mapping_unique'
	) THEN
		ALTER TABLE files
		ADD CONSTRAINT files_storage_mapping_unique UNIQUE (storage_mapping);
	END IF;
END $$;

COMMIT;