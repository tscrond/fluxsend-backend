BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- =========================
-- 1. identities
-- =========================
CREATE TABLE identities (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  provider TEXT NOT NULL,
  provider_user_id TEXT NOT NULL,
  email TEXT,
  email_verified BOOLEAN DEFAULT FALSE,
  name TEXT,
  avatar_url TEXT,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT now()
);

ALTER TABLE identities 
ADD CONSTRAINT unique_identity 
UNIQUE (provider, provider_user_id);

CREATE INDEX idx_identities_user_id ON identities(user_id);

-- migrate existing Google users
INSERT INTO identities (user_id, provider, provider_user_id, email, email_verified, name, avatar_url)
SELECT id, 'google', google_id, user_email, TRUE, user_name, NULL
FROM users
WHERE google_id IS NOT NULL;

-- =========================
-- 2. users_new (clean model)
-- =========================
CREATE TABLE users_new (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_bucket TEXT UNIQUE,
  user_email TEXT NOT NULL UNIQUE,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL
);

INSERT INTO users_new (id, user_bucket, user_email)
SELECT id, user_bucket, user_email
FROM users;

-- =========================
-- 3. rename users (safe swap)
-- =========================
ALTER TABLE users RENAME TO users_old;
ALTER TABLE users_new RENAME TO users;

CREATE INDEX idx_users_email ON users(user_email);

-- =========================
-- 4. shares migration (email → user_id)
-- =========================
ALTER TABLE shares ADD COLUMN shared_by_user_id UUID;

UPDATE shares s
SET shared_by_user_id = u.id
FROM users_old u
WHERE s.shared_by = u.user_email;

ALTER TABLE shares
ADD CONSTRAINT fk_shared_by_user
FOREIGN KEY (shared_by_user_id) REFERENCES users(id) ON DELETE SET NULL;

CREATE INDEX idx_shares_shared_by_user_id ON shares(shared_by_user_id);

-- =========================
-- 5. sessions (NEW auth system)
-- =========================
CREATE TABLE sessions (
  id UUID PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  provider TEXT NOT NULL,
  provider_access_token TEXT,
  expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL
);

CREATE INDEX idx_sessions_user_id ON sessions(user_id);

-- =========================
-- 6. FIX FK (point to new users)
-- =========================
-- identities FK fix
ALTER TABLE identities DROP CONSTRAINT identities_user_id_fkey;
ALTER TABLE identities
ADD CONSTRAINT identities_user_id_fkey
FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

-- sessions FK fix
ALTER TABLE sessions DROP CONSTRAINT sessions_user_id_fkey;
ALTER TABLE sessions
ADD CONSTRAINT sessions_user_id_fkey
FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

-- =========================
-- 7. (optional) cleanup later
-- =========================
-- DROP TABLE users_old;

COMMIT;