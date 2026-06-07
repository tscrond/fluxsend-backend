BEGIN;

ALTER TABLE plans
	DROP COLUMN IF EXISTS max_workspace_api_keys,
	DROP COLUMN IF EXISTS max_private_api_keys;

COMMIT;