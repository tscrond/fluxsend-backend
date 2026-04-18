-- Rollback: Restore google_id as primary user identifier

BEGIN;

-- 1. Add back the old columns
ALTER TABLE files ADD COLUMN owner_google_id TEXT;
ALTER TABLE notes ADD COLUMN old_user_id TEXT;

-- 2. Populate from UUID join
UPDATE files SET owner_google_id = u.google_id
FROM users u WHERE files.owner_id = u.id;

UPDATE notes SET old_user_id = u.google_id
FROM users u WHERE notes.user_id = u.id;

-- 3. Drop UUID FK constraints
ALTER TABLE files DROP CONSTRAINT IF EXISTS files_owner_id_fkey;
ALTER TABLE notes DROP CONSTRAINT IF EXISTS notes_user_id_fkey;
ALTER TABLE files DROP CONSTRAINT IF EXISTS files_unique_per_owner;
ALTER TABLE notes DROP CONSTRAINT IF EXISTS unique_user_file_note;

-- 4. Drop UUID columns
ALTER TABLE files DROP COLUMN owner_id;
ALTER TABLE notes DROP COLUMN user_id;

-- 5. Rename old columns back
ALTER TABLE notes RENAME COLUMN old_user_id TO user_id;

-- 6. Restore primary key on users
ALTER TABLE users DROP CONSTRAINT users_pkey;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_google_id_unique;
ALTER TABLE users ADD PRIMARY KEY (google_id);

-- 7. Drop UUID column
ALTER TABLE users DROP COLUMN id;

-- 8. Restore old FK constraints
ALTER TABLE files ADD CONSTRAINT files_owner_google_id_fkey
    FOREIGN KEY (owner_google_id) REFERENCES users(google_id) ON DELETE CASCADE;

ALTER TABLE notes ADD CONSTRAINT notes_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(google_id) ON DELETE CASCADE;

-- 9. Restore unique constraints
ALTER TABLE files ADD CONSTRAINT files_unique_per_owner UNIQUE (owner_google_id, file_name);
ALTER TABLE notes ADD CONSTRAINT unique_user_file_note UNIQUE (user_id, file_id);

-- 10. Revert user_bucket values from '{base}-{uuid}' back to '{base}-{google_id}'
-- This would need to be done manually as we can't reverse the UUID mapping

COMMIT;
