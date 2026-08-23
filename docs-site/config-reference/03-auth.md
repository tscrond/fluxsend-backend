# Authentication

FluxSend supports three auth modes:

- OAuth 2.0 for Google or GitHub sign-in
- email/password auth for self-hosted deployments
- API keys for CLI and programmatic access

The backend allows you to enable or disable auth providers with environment variables or startup flags. Password auth is intentionally supported so self-hosted setups do not require a third-party identity provider.

---

## Auth provider flags and env vars

The app maps the following settings:

| Setting | ENV var | CLI flag |
|---|---|---|
| Google OAuth | `ENABLE_GOOGLE_AUTH` | `--google-auth` |
| GitHub OAuth | `ENABLE_GITHUB_AUTH` | `--github-auth` |
| Password auth | `ENABLE_PASSWORD_AUTH` | `--password-auth` |

Example:

```bash
./fluxsend --password-auth
# or
export ENABLE_PASSWORD_AUTH=true
```

At least one auth method must be enabled. If none are enabled, the backend refuses to start.

---

## Password authentication

Password auth is available for self-hosted deployments and is especially useful when you want the app to run with no cloud provider dependency. It supports:

- registration
- login
- email verification
- password reset
- attach password to an OAuth-linked account

### Environment variables

| Variable | Required | Purpose |
|---|---|---|
| `ENABLE_PASSWORD_AUTH` | Yes if enabled | Turns on the password flow |
| `MAIL_FROM` | Yes | Sender address for verification emails |
| `SMTP_HOST` | Yes | SMTP server host |
| `SMTP_PORT` | Yes | SMTP server port |
| `SMTP_USERNAME` | Yes | SMTP username |
| `SMTP_PASSWORD` | Yes | SMTP password |
| `TOKEN_ENCRYPTION_KEY` | Yes | Used for secure token handling |

### Example configuration

```yaml
api:
  enable_password_auth: true
  enable_google_auth: false
  enable_github_auth: false
  frontend_endpoint: "https://files.example.com"
  backend_endpoint: "https://api.example.com"
  mail_from: "noreply@example.com"

mail:
  provider: "standard"
  smtp_host: "smtp.example.com"
  smtp_port: "587"
  smtp_username: "noreply@example.com"
  smtp_password: "smtp-secret"
```

This is the recommended option for the most self-hosted-friendly deployments.

---

## OAuth 2.0 providers

### Google

**Required environment variables:**

| Variable | Description |
|---|---|
| `GOOGLE_CLIENT_ID` | OAuth client ID |
| `GOOGLE_CLIENT_SECRET` | OAuth client secret |

**Redirect URI:** `{BACKEND_ENDPOINT}/auth/google/callback`

### GitHub

**Required environment variables:**

| Variable | Description |
|---|---|
| `GITHUB_OAUTH_CLIENT_ID` | OAuth app client ID |
| `GITHUB_OAUTH_CLIENT_SECRET` | OAuth app client secret |

**Redirect URI:** `{BACKEND_ENDPOINT}/auth/github/callback`

The app creates a session after the provider callback and stores it in PostgreSQL, with the access token encrypted before saving.

---

## Session and cookie behavior

After an OAuth or password-based login, FluxSend creates a server-side session record and sets a `session_id` cookie. The access token is encrypted before it is saved in the database.

| Detail | Value |
|---|---|
| Cookie name | `session_id` |
| HttpOnly | `true` |
| Secure | `true` |
| SameSite | `Lax` |
| Expiry | 24 hours |

The application also uses an OAuth state cookie to protect the login flow from CSRF-style tampering.

---

## API key auth

API keys are used by the CLI server that listens on port `8091` under the route prefix `/api`. Authentication is done with the `X-API-Key` header.

### Notes

- Keys are generated as random values and returned only once at creation time.
- The raw value is not stored in plaintext.
- Keys are associated with a user or a workspace and are scoped to specific permissions.

This is for CLI usage and service-to-service automation, not the browser session flow.

---

## Related endpoints

| Method | Route | Notes |
|---|---|---|
| GET | `/auth/{provider}/login` | Start provider login |
| GET | `/auth/{provider}/callback` | Provider callback |
| GET | `/auth/is_valid` | Check session validity |
| POST | `/auth/logout` | Log out and revoke session |
| POST | `/auth/password/register` | Register with email/password |
| POST | `/auth/password/login` | Sign in with email/password |
| POST | `/auth/password/reset` | Request password reset |
| POST | `/auth/password/attach` | Attach a password to an existing account |

---

## Summary

For most private self-hosted installations, the easiest configuration is:

```bash
export ENABLE_PASSWORD_AUTH=true
export ENABLE_GOOGLE_AUTH=false
export ENABLE_GITHUB_AUTH=false
```

Combined with a local PostgreSQL database and MinIO storage, this creates a deployment that does not depend on any cloud provider.
