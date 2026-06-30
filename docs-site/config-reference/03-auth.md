# Authentication

FluxSend supports two authentication modes: **OAuth 2.0 session-based auth** (for the web UI) and **API key auth** (for programmatic/CLI access).

---

## OAuth providers

### Google

**Required environment variables:**

| Variable | Description |
|---|---|
| `GOOGLE_CLIENT_ID` | OAuth 2.0 client ID |
| `GOOGLE_CLIENT_SECRET` | OAuth 2.0 client secret |

**OAuth scopes requested:**

- `https://www.googleapis.com/auth/userinfo.email`
- `https://www.googleapis.com/auth/userinfo.profile`

**Setup in Google Cloud Console:**

1. Go to **APIs & Services > Credentials**
2. Create an OAuth 2.0 Client ID (Web application type)
3. Add **Authorized JavaScript origins**: `{FRONTEND_ENDPOINT}`
4. Add **Authorized redirect URIs**: `{BACKEND_ENDPOINT}/auth/google/callback`
5. Copy the Client ID and Client Secret into `GOOGLE_CLIENT_ID` and `GOOGLE_CLIENT_SECRET`

**Userinfo endpoint:** `https://www.googleapis.com/oauth2/v2/userinfo`

**Token revocation:** POST to `https://oauth2.googleapis.com/revoke` on logout.

### GitHub

**Required environment variables:**

| Variable | Description |
|---|---|
| `GITHUB_OAUTH_CLIENT_ID` | OAuth app client ID |
| `GITHUB_OAUTH_CLIENT_SECRET` | OAuth app client secret |

**OAuth scopes requested:**

- `user:email` — read email addresses
- `read:user` — read profile data

**Setup in GitHub:**

1. Go to **Settings > Developer settings > OAuth Apps > New OAuth App**
2. Set **Homepage URL**: `{FRONTEND_ENDPOINT}`
3. Set **Authorization callback URL**: `{BACKEND_ENDPOINT}/auth/github/callback`
4. Copy the Client ID and generate a Client Secret

**Userinfo endpoint:** `https://api.github.com/user`

**Email fallback:** GitHub's `/user` endpoint may return a null email. The provider falls back to `https://api.github.com/user/emails` and picks the primary verified email.

**Token revocation:** Not supported — GitHub OAuth access tokens cannot be revoked via API. Logout is a no-op on the provider side.

---

## Session management

Once authenticated via OAuth, the server creates a session and stores it in PostgreSQL.

**Session cookie (`session_id`):**

| Property | Value |
|---|---|
| Name | `session_id` |
| HttpOnly | true |
| Secure | true |
| SameSite | Lax |
| Path | `/` |
| Expiry | 24 hours |

**OAuth state cookie (`oauth_state`)** — used for CSRF protection during login:

| Property | Value |
|---|---|
| Name | `oauth_state` |
| HttpOnly | true |
| Secure | true |
| SameSite | Lax |
| MaxAge | 5 minutes |

**Session storage:**

```sql
sessions (
    id                    UUID PRIMARY KEY,             -- the session_id cookie value
    user_id               UUID NOT NULL,                -- references users(id)
    provider              TEXT NOT NULL,                 -- "google" or "github"
    provider_access_token TEXT,                          -- AES-256-GCM encrypted
    expires_at            TIMESTAMPTZ NOT NULL,
    created_at            TIMESTAMPTZ DEFAULT now()
)
```

Session queries enforce `expires_at > now()` — expired sessions automatically return no rows and the user gets a 403.

### Token encryption

OAuth provider access tokens are encrypted at rest in the database.

| Detail | Value |
|---|---|
| Algorithm | AES-256-GCM |
| Key derivation | `SHA-256(TOKEN_ENCRYPTION_KEY)` → 32 bytes |
| Output format | `base64url(nonce \|\| ciphertext \|\| tag)` |
| Nonce size | 12 bytes (GCM standard) |

The `TOKEN_ENCRYPTION_KEY` env var can be any non-empty string. For production, generate a secure key:

```bash
openssl rand -hex 32
```

---

## Login flow

```
User clicks "Login with Google/GitHub"
         │
         ▼
GET /auth/{provider}/login
  ├── Generate 16 random bytes → hex state string
  ├── Set `oauth_state` cookie (5 min expiry)
  └── Redirect (307) to provider's OAuth URL

User authenticates on provider site
         │
         ▼
GET /auth/{provider}/callback?code=...&state=...
  ├── Validate `state` against `oauth_state` cookie
  ├── Exchange code for access token
  ├── Fetch user info from provider userinfo endpoint
  ├── findOrCreateUserFromResult():
  │   ├── Look up identity by (provider, provider_user_id)
  │   ├── If found → return existing user
  │   ├── If new + verified email matches existing user
  │   │   └── Link identity to existing user (cross-provider merge)
  │   └── Otherwise → create new user + identity
  ├── Encrypt access token (AES-256-GCM)
  ├── Create session row in DB
  ├── Set `session_id` cookie (24h expiry)
  └── Redirect (307) to FRONTEND_ENDPOINT
```

### Cross-provider identity linking

If a user authenticates with Google (email `user@example.com`) and later signs in with GitHub using the same verified email, the identities are **automatically merged** into a single account. The merge happens during `findOrCreateUserFromResult` — if the email is verified and a user with that email already exists, a new identity row is linked to the existing user.

### Logout flow

```
POST /auth/logout
  ├── Read `session_id` cookie
  ├── Decrypt provider_access_token
  ├── Google: POST to revoke endpoint
  ├── GitHub: no-op
  ├── DELETE session from DB
  ├── Clear `session_id` cookie (MaxAge=-1)
  └── 200 {logout_successful: true}
```

---

## API key auth

API keys allow programmatic access via the CLI server (port 8091, route prefix `/api`). Keys are sent via the `X-API-Key` header.

### Key generation

Keys are 32 random bytes, base64url-encoded (43 characters). The raw key is returned **only once** at creation.

### Key hashing

Keys are bcrypt-hashed at cost 10 before storage. Key lookup uses PostgreSQL's `pgcrypto` extension:

```sql
WHERE ak.key_hash = crypt($1, ak.key_hash)
```

The raw key is never stored in plaintext.

### Key types

| Type | Binding | Scopes available |
|---|---|---|
| Personal | Bound to a single user via `api_key_user_assignments` | `private_files:*` |
| Workspace | Bound to a workspace via `api_key_workspaces` | `workspaces:*` |

### Scopes

| Scope | Category |
|---|---|
| `private_files:read` | Files |
| `private_files:write` | Files |
| `private_files:delete` | Files |
| `private_files:share` | Files |
| `workspaces:read` | Workspace |
| `workspaces:write` | Workspace |
| `workspaces:delete` | Workspace |
| `workspaces:members:read` | Workspace members |
| `workspaces:members:manage` | Workspace members |
| `workspaces:invites:manage` | Workspace invites |
| `workspaces:files:read` | Workspace files |
| `workspaces:files:write` | Workspace files |
| `workspaces:files:delete` | Workspace files |

Personal keys can only use `private_files:*` scopes; workspace keys can only use `workspaces:*` scopes.

### CLI auth flow

```
Request with X-API-Key header
         │
         ▼
CLI auth middleware
  ├── GetAuthorizedCLIUserInfoByAPIKey SQL query (bcrypt via crypt())
  ├── Parse binding type (private vs workspace)
  ├── Load key scopes (validated against known scopes)
  ├── Load user's plan
  ├── Enforce plan limits (max API keys)
  └── Store AuthorizedUserWithPlan + scopes in context
```

---

## Auth endpoints

| Method | Route | Description |
|---|---|---|
| GET | `/auth/{provider}/login` | Initiate OAuth login |
| GET | `/auth/{provider}/callback` | OAuth callback |
| GET | `/auth/is_valid` | Check session validity |
| POST | `/auth/logout` | Logout and revoke session |

The CLI server at port 8091 does **not** expose auth routes — it authenticates exclusively via `X-API-Key` header.

---

## CORS

Configured on the main API server:

| Parameter | Value |
|---|---|
| `AllowedOrigins` | `{FRONTEND_ENDPOINT}` (single origin) |
| `AllowedMethods` | `GET, POST, PUT, PATCH, DELETE, OPTIONS` |
| `AllowedHeaders` | `Content-Type, Authorization` |
| `AllowCredentials` | `true` |

`AllowCredentials: true` is required for HttpOnly session cookies to be sent cross-origin.

---

## Environment variables summary

```
# OAuth
GOOGLE_CLIENT_ID=<string>
GOOGLE_CLIENT_SECRET=<string>
GITHUB_OAUTH_CLIENT_ID=<string>
GITHUB_OAUTH_CLIENT_SECRET=<string>

# Encryption
TOKEN_ENCRYPTION_KEY=<string>       # any non-empty string; recommended: openssl rand -hex 32

# Endpoints
FRONTEND_ENDPOINT=<url>             # e.g. http://localhost:8000
BACKEND_ENDPOINT=<url>              # e.g. http://localhost:3000
```
