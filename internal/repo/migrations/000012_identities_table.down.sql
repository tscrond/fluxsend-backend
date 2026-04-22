BEGIN;

DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS identities;

-- restore users table from backup
ALTER TABLE users RENAME TO users_new;
ALTER TABLE users_old RENAME TO users;

-- restore shares fk
ALTER TABLE shares DROP CONSTRAINT IF EXISTS fk_shared_by_user;
ALTER TABLE shares DROP COLUMN IF EXISTS shared_by_user_id;

DROP TABLE IF EXISTS users_new;

COMMIT;