# Kubernetes deployment

Deploy FluxSend on a Kubernetes cluster.

---

## Prerequisites

- A Kubernetes cluster (v1.24+)
- Ingress controller (Nginx, Traefik, or similar)
- PostgreSQL instance (can be self-hosted via operator or managed like RDS, Cloud SQL)

---

## Helm chart

A community-maintained Helm chart is available at [github.com/tscrond/fluxsend-charts](https://github.com/tscrond/fluxsend-charts).

```bash
helm repo add fluxsend https://tscrond.github.io/fluxsend-charts
helm install fluxsend fluxsend/fluxsend-backend \
  --set postgresql.enabled=true \
  --set secrets.storage.type=s3
```

### Default values

```yaml
images:
  backend: bobaklabs/fluxsend-backend:latest
  frontend: bobaklabs/fluxsend-frontend:latest
  docs: bobaklabs/fluxsend-docs:latest

namespace: "default"

appName: "fluxsend"
replicas: 1
mailFrom: "noreply@fluxsend.com"
labels: {}
# here specify:
# podDisruptionBudget:
#   minAvailable: <your_number>
# or
# podDisruptionBudget:
#   maxUnavailable: <your_number>
podDisruptionBudget:
  maxUnavailable: null
  minAvailable: null

cloudFront:
  enabled: false
  domain: "changeme"
  keyPairId: "changeme"
  secretName: "changeme"

ingress:
  ingressClassName: haproxy
  annotations: {}
  tls:
    secretName: "changeme"
  hosts:
    - hostname: "changeme.changeme"
      servicePort: "3000"
    - hostname: "docs.changeme.changeme"
      servicePort: "4321"

params:
  frontendEndpoint: "__CHANGE_ME__"
  backendEndpoint: "__CHANGE_ME__"
  fluxsendApiPort: "__CHANGE_ME__"
  logLevel: "development"

secrets:
  storage:
    # s3 or gcs
    type: "s3"
    storageSecretName: "changeme"

  dbSecretName: "changeme"
  smtpSecretName: "changeme"
  oauthSecretName: "changeme"
  sessionEncryptionSecretName: "changeme"

migrate:
  enabled: false
  legacyS3PrefixMigration: false
  storageMappingMigration: false
```

### Secrets reference

The chart expects pre-created Kubernetes Secrets referenced by name. Each secret must contain specific keys:

| `values.yaml` path | Secret name field | Expected keys | Maps to env vars |
|---|---|---|---|
| `secrets.oauthSecretName` | `oauthSecretName` | `google_client_id`, `google_client_secret`, `github_oauth_client_id`, `github_oauth_client_secret` | `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `GITHUB_OAUTH_CLIENT_ID`, `GITHUB_OAUTH_CLIENT_SECRET` |
| `secrets.dbSecretName` | `dbSecretName` | `postgres_user`, `postgres_password`, `postgres_db`, `db_host` | `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`, `DB_HOST` |
| `secrets.smtpSecretName` | `smtpSecretName` | `smtp_host`, `smtp_port`, `smtp_username`, `smtp_password` | `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD` |
| `secrets.sessionEncryptionSecretName` | `sessionEncryptionSecretName` | `token_encryption_key` | `TOKEN_ENCRYPTION_KEY` |
| `secrets.storage.storageSecretName` | `storageSecretName` | `gcs_bucket_name`, `google_application_credentials`, `google_project_id` (GCS) or `s3_bucket_name`, `aws_region`, `aws_access_key_id`, `aws_secret_access_key` (S3) | `GCS_BUCKET_NAME`, `GOOGLE_APPLICATION_CREDENTIALS`, ... or `S3_BUCKET_NAME`, `AWS_REGION`, ... |

### Secrets management

**Never commit raw secret values to git.** Use [Sealed Secrets](https://github.com/bitnami-labs/sealed-secrets) (kubeseal) to encrypt secrets and store them safely in your repository:

```bash
# Install the Sealed Secrets controller in your cluster
kubectl apply -f https://github.com/bitnami-labs/sealed-secrets/releases/.../controller.yaml

# Create a sealed secret from a local manifest
kubeseal --format=yaml < secret.yaml > sealed-secret.yaml

# Commit sealed-secret.yaml — it is safe to store in git
```

Alternatively, use external secrets operators (e.g. External Secrets Operator with AWS Secrets Manager, GCP Secret Manager) to pull secrets from your cloud provider.

## Resources (manual)

| Resource | Purpose |
|---|---|
| `Deployment` | Runs the backend container (port `3000` API, `8091` CLI) |
| `Service` | Exposes the deployment internally |
| `Ingress` | Routes external traffic to the service with TLS |
| `ConfigMap` / `Secret` | Environment variables and sensitive config |
| `PersistentVolumeClaim` | Only if running PostgreSQL in-cluster |

---

## Environment

Configure the backend via environment variables on the `Deployment`:

```yaml
env:
  - name: DB_HOST
    value: "postgres-service.namespace.svc.cluster.local"
  - name: POSTGRES_USER
    valueFrom:
      secretKeyRef:
        name: fluxsend-db
        key: user
  - name: POSTGRES_PASSWORD
    valueFrom:
      secretKeyRef:
        name: fluxsend-db
        key: password
  - name: POSTGRES_DB
    value: fluxsend
  - name: STORAGE_PROVIDER
    value: s3
  # ... additional vars from config reference
```

For GCS, mount the service account JSON as a volume:

```yaml
volumes:
  - name: gcs-creds
    secret:
      secretName: fluxsend-gcs
volumeMounts:
  - name: gcs-creds
    mountPath: /config
    readOnly: true
```

---

## Images

### Backend

```yaml
image: bobaklabs/fluxsend-backend:latest
```

### Frontend

```yaml
image: bobaklabs/fluxsend-frontend:latest
```

See the [Docker guide](./03-docker.md) for available registries.

---

## Frontend

The frontend is a React SPA served by Nginx. It requires the `NGINX_BACKEND_HOST`, `NGINX_BACKEND_PORT`, and `NGINX_BACKEND_API_PORT` environment variables to proxy API requests to the backend service.

```yaml
env:
  - name: NGINX_BACKEND_HOST
    value: "fluxsend-backend.svc.cluster.local"
  - name: NGINX_BACKEND_PORT
    value: "3000"
  - name: NGINX_BACKEND_API_PORT
    value: "8091"
```

In the Helm chart, the frontend is deployed as part of the release alongside the backend.

---

## Database migration

Migrations run automatically on pod startup. Ensure the database is reachable before the pod starts (use `initContainers` or a startup probe to wait for PostgreSQL).
