# Storage

FluxSend supports Google Cloud Storage and AWS S3 as object storage backends. The provider is selected via the `STORAGE_PROVIDER` env var or auto-detected at startup.

## Provider auto-detection

If `STORAGE_PROVIDER` is not set, the application inspects the environment:

- If `AWS_REGION`, `AWS_ACCESS_KEY_ID`, or `AWS_SECRET_ACCESS_KEY` is set → **S3**
- Otherwise → **GCS**

To force a specific provider, set `STORAGE_PROVIDER=gcs` or `STORAGE_PROVIDER=s3`.

---

## Google Cloud Storage

### 1. Create a service account

Create a service account in your GCP project and generate a JSON key file.

**Required IAM roles:**

| Role | Purpose |
|------|---------|
| `roles/storage.objectAdmin` | Create, read, update, delete objects |
| `roles/storage.admin` | Create and delete buckets |

The full set of IAM permissions needed:

- `storage.objects.create`
- `storage.objects.get`
- `storage.objects.list`
- `storage.objects.update`
- `storage.objects.delete`
- `storage.buckets.get`
- `storage.buckets.create`
- `storage.buckets.delete`

Signed URLs are signed locally using the private key embedded in the service account JSON file — no `iam.serviceAccounts.signBlob` permission is required.

### 2. Create a bucket

Buckets are created **per user** at runtime. The base bucket name is configured via `GCS_BUCKET_NAME`; each user gets `{GCS_BUCKET_NAME}-{userId}`.

The application creates buckets automatically with:

- **Location:** `europe-west1`
- **Uniform bucket-level access:** enabled
- **Public access prevention:** enforced

### 3. Configure environment variables

| Variable | Description |
|---|---|
| `STORAGE_PROVIDER` | Set to `gcs` (or omit for auto-detect) |
| `GCS_BUCKET_NAME` | Base bucket name |
| `GOOGLE_APPLICATION_CREDENTIALS` | Path to the service account JSON key file |
| `GOOGLE_PROJECT_ID` | GCP project ID |

### 4. Terraform

You don't need to configure Cloud Storage bucket creation for GCS provider. The code uses the cloud SDK to create them automatically upon user creation.

---

## AWS S3

### 1. Create an IAM user or role

**Minimum IAM policy:**

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:PutObject",
        "s3:GetObject",
        "s3:GetObjectAttributes",
        "s3:ListBucket",
        "s3:DeleteObject",
        "s3:DeleteObjects",
        "s3:CopyObject",
        "s3:HeadBucket",
        "s3:CreateBucket"
      ],
      "Resource": [
        "arn:aws:s3:::{BUCKET_NAME}",
        "arn:aws:s3:::{BUCKET_NAME}/*"
      ]
    }
  ]
}
```

Presigned URLs are generated using the same credentials — `s3:GetObject` on the resource is sufficient; no additional permissions are needed.

### 2. Create a bucket

Unlike GCS, **all users share a single S3 bucket**. Per-user isolation is achieved via key prefixes: objects are stored as `{userId}/{fileName}`.

The application creates the bucket automatically on first use if it does not exist.

### 3. Configure environment variables

| Variable | Description |
|---|---|
| `STORAGE_PROVIDER` | Set to `s3` (or omit for auto-detect) |
| `S3_BUCKET_NAME` | Bucket name (falls back to `GCS_BUCKET_NAME`) |
| `AWS_REGION` | e.g. `eu-north-1` |
| `AWS_ACCESS_KEY_ID` | Access key (optional — falls through to the SDK credential chain) |
| `AWS_SECRET_ACCESS_KEY` | Secret key (optional — falls through to the SDK credential chain) |

### 4. Terraform

You can use the following terraform module.

This module:

- Creates the S3 bucket
- Adjusts S3 encryption key used by default AWS CloudFront distributions
- Adds IAM policies
- Enables easy bucket lifecycle management
- Enables versioning by default

Example to define a new bucket with this module:
```hcl
# main.tf
terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "6.37.0"
    }
  }

  backend "s3" {
    bucket  = "<state_bucket_name>"
    key     = "<your_state>"
    region  = "<your_region>"
    profile = "<your_profile>"
    encrypt = true
  }
}

provider "aws" {
  region     = "<your_region>"
  access_key = var.aws_access_key_id # adjust this
  secret_key = var.aws_secret_access_key # adjust this
}


locals {
  buckets = {
    fluxsend-bucket-239123904 = {
      bucket_name = "fluxsend-bucket-239123904"
      environment = "prod"
    }
  }
}

module "s3" {
  for_each         = local.buckets
  source           = "../../modules/aws/s3"
  bucket_name      = each.value.bucket_name
  environment      = each.value.environment
  bucket_lifecycle = lookup(each.value, "bucket_lifecycle", null)
}

# required policy bindings (example)
resource "aws_iam_user" "fluxsend_prod_user" {
  name = "fluxsend_prod-user"
}

resource "aws_iam_user_policy_attachment" "fluxsend_prod_attach" {
  user       = aws_iam_user.fluxsend_prod_user.name
  policy_arn = module.s3["fluxsend-bucket-239123904"].backup_policy_arn
}

resource "aws_iam_access_key" "fluxsend_prod_key" {
  user = aws_iam_user.fluxsend_prod_user.name
}

output "fluxsend_prod_access_key_id" {
  value = aws_iam_access_key.fluxsend_prod_key.id
}

output "fluxsend_prod_secret_access_key" {
  value     = aws_iam_access_key.fluxsend_prod_key.secret
  sensitive = true
}
```

```hcl
# variables.tf
variable "bucket_name" {
  type = string
}

variable "environment" {
  type = string
  default = "prod"
}

variable "bucket_lifecycle" {
  description = "Lifecycle configuration for S3 backups. Set to null for permanent storage."

  type = object({
    expiration       = number
    noncurrent_days  = number
  })

  default = null
}

variable "force_destroy" {
  type = bool
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
