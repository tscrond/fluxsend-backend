BEGIN;
-- CREATE TABLE IF NOT EXISTS api_key_identities (

-- )

CREATE TABLE IF NOT EXISTS api_keys (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	created_by_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	whitelisted_user_ids UUID[] NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,
	expires_at TIMESTAMP WITH TIME ZONE,
	name TEXT NOT NULL,
	key TEXT NOT NULL,
	description TEXT,
	permissions TEXT ('read', 'write', 'admin') NOT NULL,
	scope TEXT ('private', 'workspaces') NOT NULL
);

COMMIT;