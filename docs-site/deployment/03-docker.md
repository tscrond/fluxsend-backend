# Docker deployment

Run FluxSend using Docker or Docker Compose.

---

## Docker Compose (recommended)

### Full stack (backend + frontend + docs)

```bash
git clone https://github.com/tscrond/fluxsend-backend
cd fluxsend-backend
git clone https://github.com/tscrond/fluxsend-frontend ../fluxsend-frontend
cp .env.example .env    # fill in your config
docker compose up -d --build
```

This starts:

| Service | Container | Port |
|---|---|---|
| `frontend` | React SPA (Nginx) | `8000` |
| `backend` | Go API server | `3000`, `8091` |
| `postgres` | PostgreSQL 16 | `5432` |
| `docs` | MkDocs documentation | `8080` |

The frontend Nginx config proxies API requests (`/auth/`, `/files/`, `/workspaces/`, etc.) to the backend automatically.

### Backend only

```bash
docker compose up -d backend dev-postgres
```

### Environment

The backend container reads its config from your host `.env` file (referenced via `${VAR}` in `docker-compose.yaml`). Pass additional config as environment variables or mount files as volumes:

```yaml
volumes:
  - /path/to/bucket-auth.json:/config/bucket-auth.json:ro
  - /path/to/cloudfront-key.pem:/config/cloudfront-key.pem:ro
```

---

## Standalone Docker

### Backend

```bash
docker build -t fluxsend-backend .
docker run -d \
  --name fluxsend \
  --env-file .env \
  -p 3000:3000 \
  -p 8091:8091 \
  fluxsend-backend
```

Note that migrations require the `internal/repo/migrations/` directory — the Docker image includes it automatically.

### Frontend

```bash
git clone https://github.com/tscrond/fluxsend-frontend
cd fluxsend-frontend
docker build -t fluxsend-frontend .
docker run -d \
  --name fluxsend-frontend \
  -p 8000:8000 \
  -e NGINX_BACKEND_HOST=localhost \
  -e NGINX_BACKEND_PORT=3000 \
  -e NGINX_BACKEND_API_PORT=8091 \
  fluxsend-frontend
```

The frontend container uses `nginx.conf` with environment variable substitution to locate the backend. On Linux, replace `host.docker.internal` with your host's IP or container network name.

---

## Docker image registries

### Backend

- `docker.io/bobaklabs/fluxsend-backend:latest`
- `ghcr.io/tscrond/fluxsend-backend:latest`

### Frontend

- `docker.io/bobaklabs/fluxsend-frontend:latest`
- `ghcr.io/tscrond/fluxsend-frontend:latest`

Multi-arch images are built via CI on pushes to `main`.
