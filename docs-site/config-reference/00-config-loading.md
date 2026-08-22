# Configuration loading, env vars, and CLI flags

FluxSend loads configuration through Viper using this precedence order:

1. built-in defaults
2. optional config file
3. environment variables
4. CLI auth override flags for the main backend

In practice, the app reads a config file from `~/.fluxsend-backend.yaml` unless you pass `--config /path/to/config.yaml`.

## Default behavior

The backend sets defaults for ports and auth toggles before merging in the config file or environment variables. For example:

- `api.listen_port` defaults to `3000`
- `api.enable_google_auth` defaults to `false`
- `api.enable_github_auth` defaults to `false`
- `api.enable_password_auth` defaults to `false`
- `cli.listen_port` defaults to `8091`

## Config file example

```yaml
app:
  env: production

api:
  listen_port: "3000"
  enable_google_auth: false
  enable_github_auth: false
  enable_password_auth: true
  frontend_endpoint: "https://files.example.com"
  backend_endpoint: "https://api.example.com"
  mail_from: "noreply@example.com"

  db:
    host: "postgres"
    user: "fluxsend"
    password: "change-me"
    name: "fluxsend"

  storage_provider: "minio"

storage:
  minio_bucket_name: "fluxsend"
  minio_endpoint: "http://minio:9000"
  minio_access_key: "fluxsend"
  minio_secret_key: "super-secret"
  minio_use_ssl: false

mail:
  provider: "standard"
  smtp_host: "smtp.example.com"
  smtp_port: "587"
  smtp_username: "noreply@example.com"
  smtp_password: "smtp-secret"

cli:
  listen_port: "8091"
  route_prefix: "/api"
```

Start the app with a custom file:

```bash
./fluxsend --config /etc/fluxsend/config.yaml
```

## Environment variable mapping

The app binds every supported config key to environment variables through Viper. The full mapping is below:

| YAML / config key | Environment variable |
|---|---|
| `api.listen_port` | `FLUXSEND_LISTEN_PORT` |
| `api.google_client_id` | `GOOGLE_CLIENT_ID` |
| `api.google_client_secret` | `GOOGLE_CLIENT_SECRET` |
| `api.github_client_id` | `GITHUB_OAUTH_CLIENT_ID` |
| `api.github_client_secret` | `GITHUB_OAUTH_CLIENT_SECRET` |
| `api.token_encryption_key` | `TOKEN_ENCRYPTION_KEY` |
| `api.frontend_endpoint` | `FRONTEND_ENDPOINT` |
| `api.backend_endpoint` | `BACKEND_ENDPOINT` |
| `api.mail_from` | `MAIL_FROM` |
| `api.enable_google_auth` | `ENABLE_GOOGLE_AUTH` |
| `api.enable_github_auth` | `ENABLE_GITHUB_AUTH` |
| `api.enable_password_auth` | `ENABLE_PASSWORD_AUTH` |
| `api.db.host` | `DB_HOST` |
| `api.db.user` | `POSTGRES_USER` |
| `api.db.password` | `POSTGRES_PASSWORD` |
| `api.db.name` | `POSTGRES_DB` |
| `api.storage_provider` | `STORAGE_PROVIDER` |
| `api.aws_region` | `AWS_REGION` |
| `api.aws_access_key_id` | `AWS_ACCESS_KEY_ID` |
| `api.aws_secret_access_key` | `AWS_SECRET_ACCESS_KEY` |
| `api.cloudfront.enable_downloads` | `ENABLE_CLOUDFRONT_DOWNLOADS` |
| `api.cloudfront.domain` | `CLOUDFRONT_DOMAIN` |
| `api.cloudfront.key_pair_id` | `CLOUDFRONT_KEY_PAIR_ID` |
| `api.cloudfront.private_key_path` | `CLOUDFRONT_PRIVATE_KEY_PATH` |
| `storage.gcs_bucket_name` | `GCS_BUCKET_NAME` |
| `storage.google_application_credentials` | `GOOGLE_APPLICATION_CREDENTIALS` |
| `storage.google_project_id` | `GOOGLE_PROJECT_ID` |
| `storage.s3_bucket_name` | `S3_BUCKET_NAME` |
| `storage.aws_region` | `AWS_REGION` |
| `storage.minio_bucket_name` | `MINIO_BUCKET_NAME` |
| `storage.minio_endpoint` | `MINIO_ENDPOINT` |
| `storage.minio_access_key` | `MINIO_ACCESS_KEY` |
| `storage.minio_secret_key` | `MINIO_SECRET_KEY` |
| `storage.minio_use_ssl` | `MINIO_USE_SSL` |
| `mail.aws_region` | `AWS_REGION` |
| `mail.aws_access_key_id` | `AWS_ACCESS_KEY_ID` |
| `mail.aws_secret_access_key` | `AWS_SECRET_ACCESS_KEY` |
| `mail.provider` | `MAIL_PROVIDER` |
| `mail.smtp_host` | `SMTP_HOST` |
| `mail.smtp_port` | `SMTP_PORT` |
| `mail.smtp_username` | `SMTP_USERNAME` |
| `mail.smtp_password` | `SMTP_PASSWORD` |
| `app.env` | `APP_ENV` |
| `cli.listen_port` | `FLUXSEND_API_LISTEN_PORT` |
| `cli.backend_endpoint` | `BACKEND_ENDPOINT` |
| `cli.route_prefix` | `FLUXSEND_API_ROUTE_PREFIX` |

This is the complete set of configuration keys currently bound by the runtime; not every key is required in every deployment, but every supported key is represented here.

## CLI flags

The main backend accepts a few CLI flags to toggle auth methods at startup. These override the corresponding config keys when present:

```bash
./fluxsend --google-auth --password-auth
```

Supported flags:

- `--github-auth`
- `--google-auth`
- `--password-auth`
- `--config /path/to/config.yaml`

This means a self-hosted installation can keep its config in one file and enable only the auth methods it needs with either config values or flags.

## Recommended self-hosted setup

For a self-hosted deployment without any cloud provider, the most straightforward setup is:

- PostgreSQL for the app database
- MinIO for object storage
- password auth enabled for the UI
- SMTP for email delivery

This avoids AWS, GCS, or other external storage dependencies while still using the same FluxSend backend and CLI interfaces.
