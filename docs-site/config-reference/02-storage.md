# Storage

FluxSend supports three storage backends: Google Cloud Storage, AWS S3, and self-hosted MinIO. The backend chooses the provider via `STORAGE_PROVIDER` or auto-detects it from the environment at startup.

## Provider selection

If `STORAGE_PROVIDER` is unset, FluxSend checks the environment in this order:

- if `AWS_REGION`, `AWS_ACCESS_KEY_ID`, or `AWS_SECRET_ACCESS_KEY` is set → `s3`
- else if `MINIO_ENDPOINT`, `MINIO_ACCESS_KEY`, or `MINIO_SECRET_KEY` is set → `minio`
- else → `gcs`

To force a specific backend, set any of the following:

```bash
export STORAGE_PROVIDER=gcs
export STORAGE_PROVIDER=s3
export STORAGE_PROVIDER=minio
```

---

## Self-hosted MinIO (recommended for self-hosting)

This is the most self-host-friendly storage mode because it runs without any cloud provider dependency. MinIO exposes an S3-compatible API, so the FluxSend backend can use it as a regular object store while keeping the data on your own infrastructure.

### Required variables

| Variable | Example | Purpose |
|---|---|---|
| `STORAGE_PROVIDER` | `minio` | Force the MinIO backend |
| `MINIO_BUCKET_NAME` | `fluxsend` | Bucket name used for object storage |
| `MINIO_ENDPOINT` | `http://minio:9000` | MinIO API endpoint |
| `MINIO_ACCESS_KEY` | `fluxsend` | Access key |
| `MINIO_SECRET_KEY` | `super-secret` | Secret key |
| `MINIO_USE_SSL` | `false` | Whether to use TLS |

### Example config

```yaml
api:
  storage_provider: "minio"

storage:
  minio_bucket_name: "fluxsend"
  minio_endpoint: "http://minio:9000"
  minio_access_key: "fluxsend"
  minio_secret_key: "super-secret"
  minio_use_ssl: false
```

This is the simplest option when you want a fully local or private deployment without AWS or GCS.

---

## Google Cloud Storage

### 1. Create a service account

Create a service account in your GCP project and generate a JSON key file.

**Required IAM roles:**

| Role | Purpose |
|------|---------|
| `roles/storage.objectAdmin` | Create, read, update, delete objects |
| `roles/storage.admin` | Create and delete buckets |

### 2. Create a bucket

Buckets are created per user at runtime. The base bucket name is configured via `GCS_BUCKET_NAME`; each user gets `{GCS_BUCKET_NAME}-{userId}`.

### 3. Configure environment variables

| Variable | Description |
|---|---|
| `STORAGE_PROVIDER` | Set to `gcs` |
| `GCS_BUCKET_NAME` | Base bucket name |
| `GOOGLE_APPLICATION_CREDENTIALS` | Path to the GCP JSON key file |
| `GOOGLE_PROJECT_ID` | GCP project ID |

---

## AWS S3

### 1. Create an IAM user or role

Use an IAM user or role with access to the bucket used by FluxSend. The app can create buckets automatically on first use, but it still needs credentials to read and write objects.

### 2. Configure environment variables

| Variable | Description |
|---|---|
| `STORAGE_PROVIDER` | Set to `s3` |
| `S3_BUCKET_NAME` | Bucket name |
| `AWS_REGION` | Example: `eu-north-1` |
| `AWS_ACCESS_KEY_ID` | Access key |
| `AWS_SECRET_ACCESS_KEY` | Secret key |

### 3. Notes

FluxSend uses S3-compatible signed URLs for downloads and object operations. This is a good option when you already rely on AWS infrastructure.

---

## Storage behavior summary

- `gcs`: per-user bucket pattern with GCP credentials
- `s3`: shared bucket pattern with AWS credentials
- `minio`: self-hosted, S3-compatible bucket pattern with local credentials

For a private deployment, `minio` is the most straightforward and least coupled to external cloud services.

  default = false
}
```

```hcl
# providers.tf
terraform {
  required_providers {
    aws = {
      source  = "aws"
      version = "6.37.0"
    }
  }
}
```

```hcl
# s3.tf
resource "aws_s3_bucket" "this" {
  bucket = var.bucket_name
  force_destroy = var.force_destroy
  object_lock_enabled = true

  tags = {
    Name        = var.bucket_name
    Environment = var.environment
  }
}

resource "aws_s3_bucket_versioning" "bucket_versioning" {
  bucket = aws_s3_bucket.this.id

  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "bucket_encryption" {
  bucket = aws_s3_bucket.this.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_s3_bucket_public_access_block" "bucket_pab" {
  bucket = aws_s3_bucket.this.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_lifecycle_configuration" "lifecycle" {
  count  = var.bucket_lifecycle != null ? 1 : 0
  bucket = aws_s3_bucket.this.id

  rule {
    id     = "retention"
    status = "Enabled"

    expiration {
      days = var.bucket_lifecycle.expiration
    }

    noncurrent_version_expiration {
      noncurrent_days = var.bucket_lifecycle.noncurrent_days
    }
  }
}
```

```hcl
# policy.tf
resource "aws_iam_policy" "bucket_policy" {
  name = "${var.bucket_name}-access-policy"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "s3:PutObject",
          "s3:GetObject",
          "s3:DeleteObject"
        ]
        Resource = "${aws_s3_bucket.this.arn}/*"
      },
      {
        Effect = "Allow"
        Action = "s3:ListBucket"
        Resource = aws_s3_bucket.this.arn
      }
    ]
  })
}
```

```hcl
# outputs.tf
output "bucket_id" {
  description = "S3 bucket ID"
  value       = aws_s3_bucket.this.id
}

output "bucket_arn" {
  description = "S3 bucket ARN"
  value       = aws_s3_bucket.this.arn
}

output "bucket_name" {
  description = "S3 bucket name"
  value       = aws_s3_bucket.this.bucket
}

output "bucket_policy_arn" {
  description = "IAM policy ARN for bucket access"
  value       = aws_iam_policy.backup_policy.arn
}

output "bucket_regional_domain_name" {
  value = aws_s3_bucket.this.bucket_regional_domain_name
}
```

---

## Signed URLs

Downloads are served via short-lived signed URLs, never directly from the bucket.

| Provider | Method | Expiry |
|---|---|---|
| GCS | V4 signing scheme, GET method | Set per-request (typically 1–60 min) |
| S3 | `PresignGetObject` | Set per-request (typically 1–60 min) |

For CloudFront-backed downloads, see the [CDN reference](05-cdn.md).

---

## Environment variables summary

```
STORAGE_PROVIDER=[gcs|s3]                  # default: auto-detect

# GCS
GCS_BUCKET_NAME=<string>
GOOGLE_APPLICATION_CREDENTIALS=<path>
GOOGLE_PROJECT_ID=<string>

# S3
S3_BUCKET_NAME=<string>                    # falls back to GCS_BUCKET_NAME
AWS_REGION=<string>
AWS_ACCESS_KEY_ID=<string>                 # optional — SDK chain fallback
AWS_SECRET_ACCESS_KEY=<string>             # optional — SDK chain fallback
```
