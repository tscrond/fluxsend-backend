# Deployment examples

Two compose files that work together: a minimal Traefik edge proxy and a
self-hosted MinIO stack for FluxSend.

| File | Purpose |
|---|---|
| `traefik/docker-compose.yaml` | Publishes 80/443, terminates TLS, creates the shared `proxy` network |
| `minio/docker-compose.yaml` | FluxSend frontend/backend + PostgreSQL + MinIO + docs, routed by Traefik |

## Run

1. Create DNS records: `app.example.com` (and `docs.example.com`) → this host's IP.
2. Build the images from the repo root (or swap the `:dev` images for published registry tags):

   ```bash
   make build
   ```

3. Start Traefik (creates the `proxy` network):

   ```bash
   cp examples/traefik/.env.example examples/traefik/.env
   docker compose -f examples/traefik/docker-compose.yaml up -d
   ```

4. Start the MinIO stack:

   ```bash
   cp examples/minio/.env.example examples/minio/.env
   docker compose -f examples/minio/docker-compose.yaml up -d
   ```

Certificates are issued on first request; follow progress with:

```bash
docker compose -f examples/traefik/docker-compose.yaml logs -f traefik
```

## Notes

- Only Traefik publishes ports. PostgreSQL, MinIO, the API, and the CLI stay on the internal network.
- Single origin (`https://app.example.com`): the frontend nginx proxies the API, so no CORS setup is needed.
- Downloads are streamed through the backend by default, so object storage is never exposed and URLs stay on the app origin. To hand downloads to MinIO directly, create a DNS record for `s3.<APP_DOMAIN>`, set `MINIO_PUBLIC_ENDPOINT=https://s3.<APP_DOMAIN>` in `.env`, and uncomment the MinIO Traefik labels in `minio/docker-compose.yaml` (the route is limited to `GET`/`HEAD`/`OPTIONS`).
- The MinIO console and the metrics listener are private by default; enable the commented labels to publish the console.
- MinIO no longer ships an official community image: `minio/minio` was removed
  from Docker Hub, dl.min.io returns 410, and `quay.io/minio/aistor/minio`
  requires a licence. The stack uses `pgsty/minio`, the maintained AGPLv3
  continuation ("Silo"). `minio/Dockerfile` is an alternative that builds the
  last official upstream release (`RELEASE.2025-09-07T16-13-09Z`) from the
  GitHub release assets; build it with
  `docker build -t fluxsend-minio:oss examples/minio` and point the service at
  that tag. Both are drop-in for the S3 API the backend uses.
- Change every default secret in `.env` before going live, and back up `TOKEN_ENCRYPTION_KEY`.
