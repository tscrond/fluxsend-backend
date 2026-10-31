BEGIN;

DROP INDEX IF EXISTS idx_api_keys_key_lookup_hash;

ALTER TABLE api_keys
	DROP COLUMN IF EXISTS key_lookup_hash;

COMMIT;
