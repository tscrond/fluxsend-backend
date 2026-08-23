# Docker deployment

FluxSend can be run with Docker Compose or a standalone container. The repository includes a self-hosted MinIO stack that is the easiest local/private deployment path.

---

## Recommended: self-hosted MinIO stack

The project includes a Docker configuration tuned for MinIO and password auth:

```bash
git clone https://github.com/tscrond/fluxsend-backend
cd fluxsend-backend
docker compose -f compose.minio.yaml up -d --build
```

This stack includes:

| Service | Container | Port |
|---|---|---|
| `frontend` | React SPA | `8000` |
| `backend` | FluxSend API + CLI | `3000`, `8091` |
| `postgres` | PostgreSQL 16 | `5432` |
| `minio` | S3-compatible object storage | `9000`, `9001` |
| `docs` | MkDocs docs site | `8080` |

The default stack in `compose.minio.yaml` sets:

- `STORAGE_PROVIDER=minio`
- `ENABLE_PASSWORD_AUTH=true`
- `MINIO_ENDPOINT=http://minio:9000`
- `MINIO_ACCESS_KEY` / `MINIO_SECRET_KEY` from MinIO root credentials

This is the best option for a deployment with no cloud storage vendor involved.

---

## Minimal Compose example

```yaml
services:
  backend:
    image: fluxsend-backend:dev
    ports:
      - "3000:3000"
      - "8091:8091"
    environment:
      - STORAGE_PROVIDER=minio
      - MINIO_BUCKET_NAME=fluxsend
      - MINIO_ENDPOINT=http://minio:9000
      - MINIO_ACCESS_KEY=fluxsend
      - MINIO_SECRET_KEY=change-me
      - MINIO_USE_SSL=false
      - ENABLE_PASSWORD_AUTH=true
      - ENABLE_GOOGLE_AUTH=false
      - ENABLE_GITHUB_AUTH=false
      - DB_HOST=postgres
      - POSTGRES_USER=fluxsend
      - POSTGRES_PASSWORD=change-me
      - POSTGRES_DB=fluxsend
      - FRONTEND_ENDPOINT=http://localhost:8000
      - BACKEND_ENDPOINT=http://localhost:3000
      - MAIL_FROM=noreply@example.com
```

Use a `.env` file or Compose `environment:` values for the rest of the backend config.

---

## Standalone Docker container

```bash
docker build -t fluxsend-backend .
docker run -d \
  --name fluxsend \
  --env-file .env \
  -p 3000:3000 \
  -p 8091:8091 \
  fluxsend-backend
```

The backend reads the same env vars and config file settings as a standalone Linux install. Migration files are included in the image.

---

## Docker image registries

- `docker.io/bobaklabs/fluxsend-backend:latest`
- `ghcr.io/tscrond/fluxsend-backend:latest`

For self-hosted deployments, the MinIO Compose example is the most practical and least coupled to external infrastructure.

