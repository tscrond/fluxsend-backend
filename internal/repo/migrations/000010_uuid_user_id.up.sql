-- Migration: Switch primary user identifier from google_id to UUID
-- Non-destructive: google_id is kept as a UNIQUE column for OAuth lookups

-- 1. Add UUID column to users and populate for existing rows
ALTER TABLE users ADD COLUMN id UUID DEFAULT gen_random_uuid();
UPDATE users SET id = gen_random_uuid() WHERE id IS NULL;
ALTER TABLE users ALTER COLUMN id SET NOT NULL;

-- 2. Drop old FK constraints (must happen before dropping columns)
ALTER TABLE files DROP CONSTRAINT IF EXISTS files_owner_google_id_fkey;
ALTER TABLE notes DROP CONSTRAINT IF EXISTS notes_user_id_fkey;

-- 3. Drop old unique constraints that reference owner_google_id
ALTER TABLE files DROP CONSTRAINT IF EXISTS files_unique_per_owner;
ALTER TABLE notes DROP CONSTRAINT IF EXISTS unique_user_file_note;

-- 4. Add new UUID-based owner column to files, populate, drop old
ALTER TABLE files ADD COLUMN owner_id UUID;
UPDATE files SET owner_id = u.id FROM users u WHERE files.owner_google_id = u.google_id;
ALTER TABLE files DROP COLUMN owner_google_id;

-- 5. Add new UUID-based user column to notes, populate, drop old, rename
ALTER TABLE notes ADD COLUMN uuid_user_id UUID;
UPDATE notes SET uuid_user_id = u.id FROM users u WHERE notes.user_id = u.google_id;
ALTER TABLE notes DROP COLUMN user_id;
ALTER TABLE notes RENAME COLUMN uuid_user_id TO user_id;

-- 6. Switch primary key on users from google_id to id
ALTER TABLE users DROP CONSTRAINT users_pkey;
ALTER TABLE users ADD PRIMARY KEY (id);
ALTER TABLE users ADD CONSTRAINT users_google_id_unique UNIQUE (google_id);

-- 7. Add new FK constraints using UUID
ALTER TABLE files ADD CONSTRAINT files_owner_id_fkey
    FOREIGN KEY (owner_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE notes ADD CONSTRAINT notes_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

-- 8. Re-create unique constraints
ALTER TABLE files ADD CONSTRAINT files_unique_per_owner UNIQUE (owner_id, file_name);
ALTER TABLE notes ADD CONSTRAINT unique_user_file_note UNIQUE (user_id, file_id);

-- 9. Update user_bucket values from '{base}-{google_id}' to '{base}-{uuid}'
UPDATE users SET user_bucket =
    substring(user_bucket FROM 1 FOR length(user_bucket) - length(google_id)) || id::text
WHERE user_bucket IS NOT NULL AND google_id != '';
