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

```bash
mkdir -p /opt/fluxsend
cp fluxsend /opt/fluxsend/
cp -r internal/repo/migrations /opt/fluxsend/internal/repo/migrations/
```

Create `/opt/fluxsend/.env` with all required [environment variables](../config-reference/01-env-vars.md).

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
ExecStart=/opt/fluxsend/fluxsend
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

The frontend expects the backend at `VITE_API_BASE` (set at build time or left empty if proxied through Nginx).

## Reverse proxy

Place a reverse proxy (Nginx, Caddy, Traefik) in front of both the backend (port `3000`) and frontend (port `8000`) to handle TLS termination and domain routing. See [production setup](./01-production-setup.md) for recommendations.

The frontend repository ships a production-ready `nginx.conf` that proxies API requests to the backend automatically.

## Database

The application requires a PostgreSQL 13+ database. Connect it via the `DB_HOST` env var. Migrations run automatically on startup.
