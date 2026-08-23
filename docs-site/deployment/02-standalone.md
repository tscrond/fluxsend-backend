# Standalone deployment

Run FluxSend directly on a Linux host without containerization.

---

## Build the binary

```bash
git clone https://github.com/tscrond/fluxsend-backend
cd fluxsend-backend
CGO_ENABLED=0 go build -o fluxsend ./cmd
```

## Prepare the environment

The backend reads environment variables and a config file, with config file values coming before environment variables. See [Configuration loading](../config-reference/00-config-loading.md) for details.

Create `/opt/fluxsend/.env` with a self-hosted MinIO + password-auth example:

```bash
export APP_ENV=production
export FLUXSEND_LISTEN_PORT=3000
export DB_HOST=localhost
export POSTGRES_USER=fluxsend
export POSTGRES_PASSWORD=change-me
export POSTGRES_DB=fluxsend

export BACKEND_ENDPOINT=https://api.example.com
export FRONTEND_ENDPOINT=https://files.example.com
export TOKEN_ENCRYPTION_KEY=$(openssl rand -hex 32)
export MAIL_FROM=noreply@example.com

export ENABLE_PASSWORD_AUTH=true
export ENABLE_GOOGLE_AUTH=false
export ENABLE_GITHUB_AUTH=false

export STORAGE_PROVIDER=minio
export MINIO_BUCKET_NAME=fluxsend
export MINIO_ENDPOINT=http://localhost:9000
export MINIO_ACCESS_KEY=fluxsend
export MINIO_SECRET_KEY=change-me
export MINIO_USE_SSL=false

export SMTP_HOST=smtp.example.com
export SMTP_PORT=587
export SMTP_USERNAME=noreply@example.com
export SMTP_PASSWORD=smtp-secret
```

You can also use a YAML config file instead of exporting variables individually. The app reads `--config` and defaults to `~/.fluxsend-backend.yaml` when no file is passed.

## Install and run

```bash
mkdir -p /opt/fluxsend
cp fluxsend /opt/fluxsend/
cp -r internal/repo/migrations /opt/fluxsend/internal/repo/migrations/
```

Start the service directly:

```bash
cd /opt/fluxsend
./fluxsend --config /etc/fluxsend/config.yaml
```

## Systemd service

Create `/etc/systemd/system/fluxsend.service`:

```ini
[Unit]
Description=FluxSend backend
After=network.target postgresql.service

[Service]
Type=simple
WorkingDirectory=/opt/fluxsend
EnvironmentFile=/opt/fluxsend/.env
ExecStart=/opt/fluxsend/fluxsend --password-auth
Restart=always
RestartSec=5
User=fluxsend
Group=fluxsend

[Install]
WantedBy=multi-user.target
```

```bash
systemctl daemon-reload
systemctl enable --now fluxsend
```

## Frontend

The frontend is a React SPA served by Nginx. Clone and build from [github.com/tscrond/fluxsend-frontend](https://github.com/tscrond/fluxsend-frontend):

```bash
git clone https://github.com/tscrond/fluxsend-frontend
cd fluxsend-frontend
npm ci
npm run build
```

Serve the `dist/` directory with any static file server. The Nginx config in the repo proxies API routes to the backend — use it as a reference for your own reverse proxy setup.

## Reverse proxy

Place a reverse proxy (Nginx, Caddy, Traefik) in front of both the backend (port `3000`) and frontend (port `8000`) to handle TLS termination and domain routing. See [production setup](./01-production-setup.md) for recommendations.

## Database

The application requires a PostgreSQL 13+ database. Connect it via `DB_HOST`. Migrations run automatically on startup.

For the most self-hosted-friendly setup, pair PostgreSQL with MinIO and password auth as shown above.

