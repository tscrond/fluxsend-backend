BEGIN;
ALTER TABLE files
DROP CONSTRAINT IF EXISTS files_owner_id_fkey;

ALTER TABLE files
ADD CONSTRAINT files_owner_id_fkey
FOREIGN KEY (owner_id)
REFERENCES users(id)
ON DELETE CASCADE;

ALTER TABLE notes
DROP CONSTRAINT IF EXISTS notes_user_id_fkey;

ALTER TABLE notes
ADD CONSTRAINT notes_user_id_fkey
FOREIGN KEY (user_id)
REFERENCES users(id)
ON DELETE CASCADE;

ALTER TABLE shares
DROP CONSTRAINT IF EXISTS shares_shared_by_fkey;

ALTER TABLE shares
ADD CONSTRAINT shares_shared_by_fkey
FOREIGN KEY (shared_by)
REFERENCES users(user_email)
ON DELETE CASCADE;
COMMIT;