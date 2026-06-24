# Authentication

FluxSend uses OAuth-based login flows and authenticated API requests for user-specific operations.

## Login flow

- `GET /auth/oauth` starts the provider redirect flow.
- `GET /auth/callback` completes the OAuth round-trip.
- `GET /auth/is_valid` checks whether the current session or token is valid.
- `GET /auth/logout` clears the current session.

## Protected API usage

Most file, workspace, and user routes require authentication. API-key based access is also supported for CLI and workspace flows where applicable.

## API keys

API key endpoints live under the `api_keys` routes and support workspace-scoped and private key management.