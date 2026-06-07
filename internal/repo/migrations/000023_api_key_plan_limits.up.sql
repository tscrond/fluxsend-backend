BEGIN;

ALTER TABLE plans
	ADD COLUMN max_private_api_keys BIGINT NOT NULL DEFAULT 2,
	ADD COLUMN max_workspace_api_keys BIGINT NOT NULL DEFAULT 1;

UPDATE plans SET
	max_private_api_keys = 2,
	max_workspace_api_keys = 1
WHERE name = 'free';

UPDATE plans SET
	max_private_api_keys = 25,
	max_workspace_api_keys = 10
WHERE name = 'developer';

UPDATE plans SET
	max_private_api_keys = 500,
	max_workspace_api_keys = 100
WHERE name = 'enterprise';

COMMIT;