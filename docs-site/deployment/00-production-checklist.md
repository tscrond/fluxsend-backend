# Production Deployment Checklist

Single-origin (`https://app.example.com`) is recommended: the frontend nginx proxies all backend routes, so there is no CORS and cookies stay same-site. Check items off as you go.

## 1. Decisions

- [ ] Topology: Docker Compose (VPS) / systemd / Kubernetes
- [ ] Storage: MinIO / S3 / GCS
- [ ] Auth: password / Google / GitHub — **at least one required**
- [ ] Mail: SMTP (587 STARTTLS only) / SES
- [ ] Origin: single (recommended) / split `app.` + `api.`
- [ ] Record domains, ports, providers, and who owns each credential

## 2. Provision services

**PostgreSQL**

- [ ] 13+ (16 recommended); create database + user
- [ ] User may `CREATE EXTENSION pgcrypto` (migrations need it)
- [ ] Add a persistent volume — **repo compose files have none**
- [ ] Reachable on 5432 without TLS (both hardcoded in code)

**Object storage**

- [ ] MinIO: set root user/password; bucket auto-created; keep console 9001 private; prefer a dedicated user over root creds
- [ ] S3: bucket + IAM with PutObject/GetObject/DeleteObject/ListBucket
- [ ] GCS: service account with `storage.objectAdmin` + `storage.admin`, JSON key, base bucket
- [ ] CloudFront (S3 only): distribution, key pair, RSA private key on host

**Mail**

- [ ] SMTP2Go: create account → **Sending → Verified Senders → Add Sender Domain** (e.g. `example.com`)
- [ ] SMTP2Go: add the DKIM records it shows in Cloudflare DNS (2× CNAME, usually `s1._domainkey` / `s2._domainkey`) — **DNS only (grey cloud)**
- [ ] SMTP2Go: wait for the domain to show **Verified**
- [ ] SMTP2Go: **Sending → SMTP Users → Add SMTP User**; copy username + password
- [ ] SMTP2Go: note endpoint `mail.smtp2go.com` (or regional, e.g. `mail-eu.smtp2go.com`) and port `587`
- [ ] Cloudflare: **Email → Email Routing → Enable**; accept the suggested MX records (`route1/2/3.mx.cloudflare.net`); remove conflicting MX
- [ ] Cloudflare: verify destination address(es), then create routing rules (custom address or catch-all)
- [ ] Cloudflare: keep exactly **one** SPF TXT on the root — merge both senders, never add a second record:
      `v=spf1 include:_spf.mx.cloudflare.net include:spf.smtp2go.com ~all`
- [ ] Cloudflare: add `_dmarc` TXT `v=DMARC1; p=none; rua=mailto:dmarc@example.com` (tighten after monitoring)
- [ ] Cloudflare: optional — enable Email Routing DKIM signing and add the record it provides
- [ ] Backend env: `MAIL_PROVIDER=standard`, `MAIL_FROM=noreply@example.com` (must be on the verified domain)
- [ ] Backend env: `SMTP_HOST=mail-eu.smtp2go.com`, `SMTP_PORT=587`, `SMTP_USERNAME`, `SMTP_PASSWORD`
- [ ] Note: FluxSend sends over port 587 STARTTLS only — 465 implicit TLS is unsupported; Email Routing handles inbound, SMTP2Go handles outbound

**OAuth**

- [ ] Google client → redirect `{BACKEND_ENDPOINT}/auth/google/callback`
- [ ] GitHub app → redirect `{BACKEND_ENDPOINT}/auth/github/callback`

**Host**

- [ ] Size CPU/RAM/disk for 100 MB uploads and storage throughput

## 3. DNS

- [ ] `app.example.com` → edge proxy (A/AAAA)
- [ ] `api.example.com` → backend (split-origin only)
- [ ] `docs.example.com` → docs (optional)
- [ ] `cdn.example.com` → CloudFront + ACM cert (optional)
- [ ] MX/SPF/DKIM/DMARC for the mail domain (see Mail setup in §2)
- [ ] Verify all names resolve (`dig`)

## 4. Reverse proxy + TLS

- [ ] TLS is **mandatory** — cookies are always `Secure`; login fails over HTTP
- [ ] Install Caddy/Nginx/Traefik (or k8s ingress)
- [ ] Vhost `app.example.com`, terminate TLS, redirect 80→443
- [ ] Upstream to frontend `:8000` (single-origin)
- [ ] `client_max_body_size >= 100M` + upload timeouts
- [ ] Preserve `Host`, `X-Real-IP`, `X-Forwarded-For`
- [ ] Certs issued + autorenew verified
- [ ] HSTS after HTTPS confirmed
- [ ] Firewall 3000/8091/1414/9464/9000/9001/5432

## 5. Artifacts

- [ ] Backend image, pinned tag (`bobaklabs/fluxsend-backend` / `ghcr.io/tscrond/fluxsend-backend`)
- [ ] Frontend image, pinned tag (`bobaklabs/fluxsend-frontend` / `ghcr.io/tscrond/fluxsend-frontend`)
- [ ] Docs image (optional)
- [ ] Migration dir present (`internal/repo/migrations`) — in image, or copy for standalone
- [ ] `VITE_API_BASE` is build-time; leave empty for same-origin (rebuild to change)

## 6. Configure services

**PostgreSQL service**

- [ ] `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` (applied only on first init)

**MinIO service**

- [ ] `MINIO_ROOT_USER`, `MINIO_ROOT_PASSWORD` (backend access/secret key must match)

**Backend service**

- [ ] `APP_ENV=production`
- [ ] `FLUXSEND_LISTEN_PORT=3000`
- [ ] `FLUXSEND_API_LISTEN_PORT=8091`
- [ ] `FLUXSEND_API_ROUTE_PREFIX=/api`
- [ ] `BACKEND_ENDPOINT`, `FRONTEND_ENDPOINT`
- [ ] `TOKEN_ENCRYPTION_KEY` (`openssl rand -hex 32`) — back it up
- [ ] Auth: `ENABLE_PASSWORD_AUTH` / `ENABLE_GOOGLE_AUTH` + `GOOGLE_CLIENT_ID` + `GOOGLE_CLIENT_SECRET` / `ENABLE_GITHUB_AUTH` + `GITHUB_OAUTH_CLIENT_ID` + `GITHUB_OAUTH_CLIENT_SECRET`
- [ ] `EMAIL_WHITELIST` (optional)
- [ ] DB: `DB_HOST`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`
- [ ] `STORAGE_PROVIDER` (`minio` / `s3` / `gcs`)
- [ ] MinIO: `MINIO_BUCKET_NAME`, `MINIO_ENDPOINT`, `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY`, `MINIO_USE_SSL`
- [ ] S3: `S3_BUCKET_NAME`, `AWS_REGION`, `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`
- [ ] GCS: `GCS_BUCKET_NAME`, `GOOGLE_APPLICATION_CREDENTIALS`, `GOOGLE_PROJECT_ID`
- [ ] Mail: `MAIL_FROM`, `MAIL_PROVIDER`, `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD` (SES: `AWS_REGION`, `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`)
- [ ] CDN (S3 only): `ENABLE_CLOUDFRONT_DOWNLOADS`, `CLOUDFRONT_DOMAIN`, `CLOUDFRONT_KEY_PAIR_ID`, `CLOUDFRONT_PRIVATE_KEY_PATH`
- [ ] Admin: `ADMIN_SERVER_ENABLED`, `ADMIN_USERNAME`, `ADMIN_PASSWORD`, `ADMIN_LISTEN_PORT` (or leave disabled)
- [ ] Metrics: `METRICS_ENABLED`, `METRICS_LISTEN_PORT`, `METRICS_BIND_ADDRESS`, `METRICS_PATH`
- [ ] `DEBUG_CONFIG_LOADING=false`

**Frontend service**

- [ ] `NGINX_BACKEND_HOST`, `NGINX_BACKEND_PORT=3000`, `NGINX_BACKEND_API_PORT=8091`

**Secrets**

- [ ] Store in a secret manager / Compose secrets / k8s Secrets — never commit `.env`
- [ ] Mount paths exist for GCS JSON and CloudFront PEM

## 7. First deploy

- [ ] Postgres healthcheck + `depends_on: service_healthy` (**backend has no DB retry**)
- [ ] Start order: Postgres → storage → backend → frontend
- [ ] Backend command enables the intended auth (e.g. `--password-auth`)
- [ ] Migrations succeed in logs
- [ ] Log shows the correct `selected storage provider`
- [ ] `session_id` cookie is `Secure` + `HttpOnly` + `SameSite=Lax`

## 8. Verify

- [ ] SPA loads over HTTPS
- [ ] Login works
- [ ] Upload → object appears in bucket
- [ ] Public download (signed URL redirect)
- [ ] Email share delivers (check spam)
- [ ] SPF + DKIM pass (send to Gmail → "Show original", or mail-tester.com)
- [ ] Password reset email
- [ ] OAuth round-trip returns to `FRONTEND_ENDPOINT`
- [ ] Workspace + API key; `/api` call with `X-API-Key`
- [ ] CloudFront download (if enabled)
- [ ] Metrics reachable only from private network

## 9. Operations

- [ ] Prometheus scraping (`fluxsend_db_up`, request rate, P95 latency)
- [ ] Scheduled Postgres backups + tested restore
- [ ] Bucket backup/versioning
- [ ] Back up `TOKEN_ENCRYPTION_KEY`
- [ ] Log retention + shipping off-host
- [ ] Alert on mail failures (no queue/retry in app)

## 10. Harden

- [ ] Only 80/443 public
- [ ] `DEBUG_CONFIG_LOADING` off
- [ ] Admin off, or strong creds (compose default is `admin`/`admin`)
- [ ] `METRICS_BIND_ADDRESS=127.0.0.1` or private
- [ ] Dedicated MinIO user instead of root
- [ ] OAuth redirect URIs locked to production
- [ ] TLS 1.2+, HSTS, modern ciphers
- [ ] Proxy buffering/timeouts tuned for uploads

## 11. Go-live + rollback

- [ ] Snapshot DB + bucket before cutover
- [ ] Lower DNS TTL, then switch
- [ ] Keep previous image tags for rollback
- [ ] Document rollback: migrations are forward-only → restore DB snapshot

## Non-negotiables (code-level)

- HTTPS required (Secure cookies)
- DB port 5432 + `sslmode=disable` hardcoded
- No DB retry at startup
- At least one auth method required
- CloudFront requires S3
- SMTP 587 STARTTLS only (no 465)
- Metrics unauthenticated
- Main API has no `/health` (only the admin server does)
- Frontend nginx caps uploads at 100 MB
- Repo compose Postgres has no volume
