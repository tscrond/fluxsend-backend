BEGIN;

ALTER TABLE api_keys
	ADD COLUMN IF NOT EXISTS key_lookup_hash TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS idx_api_keys_key_lookup_hash
	ON api_keys (key_lookup_hash);

COMMIT;
