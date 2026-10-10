# Self-hosted MinIO deployment

This is the most self-host-friendly deployment path for FluxSend because it keeps the application, database, and object storage inside your own infrastructure and does not require a managed cloud storage provider.

## Why this setup is recommended

- No AWS or GCS dependency for object storage
- Easy to run locally or on a small VPS
- MinIO exposes the same S3-compatible API the app expects
- Password authentication can be enabled without any third-party OAuth provider
- Works well with Docker Compose or a Linux systemd deployment

## Example Compose stack

The repository includes a MinIO-focused stack in `compose.minio.yaml`.

```bash
docker compose -f compose.minio.yaml up -d --build
```

The stack runs:

- the FluxSend backend
- PostgreSQL
- MinIO
- the frontend
- documentation

## Example config for password-only auth + MinIO

```yaml
app:
  env: production

api:
  listen_port: "3000"
  enable_google_auth: false
  enable_github_auth: false
  enable_password_auth: true
  frontend_endpoint: "http://localhost:8000"
  backend_endpoint: "http://localhost:3000"
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
  minio_secret_key: "change-me"
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

## Environment-variable version

```bash
export STORAGE_PROVIDER=minio
export MINIO_BUCKET_NAME=fluxsend
export MINIO_ENDPOINT=http://minio:9000
export MINIO_ACCESS_KEY=fluxsend
export MINIO_SECRET_KEY=change-me
export MINIO_USE_SSL=false

export ENABLE_PASSWORD_AUTH=true
export ENABLE_GITHUB_AUTH=false
export ENABLE_GOOGLE_AUTH=false

export DB_HOST=postgres
export POSTGRES_USER=fluxsend
export POSTGRES_PASSWORD=change-me
export POSTGRES_DB=fluxsend

export FRONTEND_ENDPOINT=http://localhost:8000
export BACKEND_ENDPOINT=http://localhost:3000
export MAIL_FROM=noreply@example.com
```

## MinIO setup checklist

1. Start the MinIO container.
2. Create the root user and password via `MINIO_ROOT_USER` and `MINIO_ROOT_PASSWORD`.
3. Set `MINIO_ACCESS_KEY` and `MINIO_SECRET_KEY` to the credentials the backend uses.
4. Ensure the bucket `fluxsend` exists or let the app create it automatically on first use.
5. Set `STORAGE_PROVIDER=minio` or rely on auto-detection when MinIO values are present.

## How downloads reach the browser

`MINIO_ENDPOINT` (`http://minio:9000`) is the internal address the backend uses
and is not resolvable from the public internet. Downloads therefore need one of:

- **Backend proxy (default).** With no `MINIO_PUBLIC_ENDPOINT` set, the backend
  streams objects itself over the app's HTTPS origin. MinIO is never exposed and
  no extra DNS, TLS, or firewall work is needed. Password-protected shares and
  direct-to-object-store range requests are limited in this mode.
- **Public download endpoint.** Set `MINIO_PUBLIC_ENDPOINT=https://s3.example.com`
  (HTTPS only) and route that host to `minio:9000` through your reverse proxy.
  The backend then returns short-lived presigned URLs so browsers download
  directly from MinIO. Keep the console port private, forward the original
  `Host` header, and restrict the route to `GET`/`HEAD`/`OPTIONS`.

The presign region defaults to `us-east-1`; change `MINIO_REGION` if MinIO was
started with a different region.

## Password auth checklist

1. Set `ENABLE_PASSWORD_AUTH=true` or pass `--password-auth` at startup.
2. Make sure at least one auth method is enabled:
   - password
   - Google OAuth
   - GitHub OAuth
3. Configure SMTP so registration and password reset emails can be sent.

This is the simplest production-ready self-hosted option if you want to avoid cloud-managed object storage and don’t need OAuth immediately.
