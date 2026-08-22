# Environment variables

The application reads its configuration through Viper with a layered precedence of defaults, config file, environment variables, and a few auth override flags. See [Configuration loading](./00-config-loading.md) for the full precedence and examples.

The following table documents the environment variables supported by the backend.

| Variable | Description | Default | Required |
|---|---|---|---|
| APP_ENV | Runtime environment (`production` or `development`) | `production` | No |
| BACKEND_ENDPOINT | Base URL for callbacks and public links | `""` | Yes |
| FRONTEND_ENDPOINT | Frontend origin used for redirects and CORS | `""` | Yes |
| FLUXSEND_LISTEN_PORT | Main backend listen port | `3000` | No |
| FLUXSEND_API_LISTEN_PORT | Developer CLI API listen port | `8091` | No |
| FLUXSEND_API_ROUTE_PREFIX | CLI API route prefix | `/api` | No |
| ENABLE_GOOGLE_AUTH | Enables Google OAuth login | `false` | No |
| ENABLE_GITHUB_AUTH | Enables GitHub OAuth login | `false` | No |
| ENABLE_PASSWORD_AUTH | Enables email/password auth | `false` | No |
| GOOGLE_CLIENT_ID | Google OAuth client ID | `""` | If Google auth enabled |
| GOOGLE_CLIENT_SECRET | Google OAuth client secret | `""` | If Google auth enabled |
| GITHUB_OAUTH_CLIENT_ID | GitHub OAuth app client ID | `""` | If GitHub auth enabled |
| GITHUB_OAUTH_CLIENT_SECRET | GitHub OAuth app client secret | `""` | If GitHub auth enabled |
| TOKEN_ENCRYPTION_KEY | Key used to encrypt provider access tokens at rest | `""` | Yes |
| DB_HOST | PostgreSQL host | `""` | Yes |
| POSTGRES_DB | PostgreSQL database name | `""` | Yes |
| POSTGRES_USER | PostgreSQL database user | `""` | Yes |
| POSTGRES_PASSWORD | PostgreSQL database password | `""` | Yes |
| STORAGE_PROVIDER | Storage backend: `gcs`, `s3`, or `minio` | Auto-detected | No |
| GCS_BUCKET_NAME | Base GCS bucket name | `""` | If `gcs` |
| GOOGLE_APPLICATION_CREDENTIALS | GCS service account JSON path | `""` | If `gcs` |
| GOOGLE_PROJECT_ID | GCP project ID for GCS | `""` | If `gcs` |
| S3_BUCKET_NAME | Base S3 bucket name | `""` | If `s3` |
| AWS_REGION | AWS region for S3/SES | `""` | If `s3` or SES |
| AWS_ACCESS_KEY_ID | AWS access key for S3/SES | `""` | If using AWS credentials |
| AWS_SECRET_ACCESS_KEY | AWS secret key for S3/SES | `""` | If using AWS credentials |
| MINIO_BUCKET_NAME | Base MinIO bucket name | `""` | If `minio` |
| MINIO_ENDPOINT | MinIO endpoint, such as `http://minio:9000` | `""` | If `minio` |
| MINIO_ACCESS_KEY | MinIO access key / login | `""` | If `minio` |
| MINIO_SECRET_KEY | MinIO secret key / password | `""` | If `minio` |
| MINIO_USE_SSL | Use TLS when connecting to MinIO | `false` | No |
| MAIL_FROM | Default sender email for password reset and mail notifications | `noreply@fluxsend.invalid` | Yes |
| MAIL_PROVIDER | Mail provider (`standard` or `ses`) | `standard` | No |
| SMTP_HOST | SMTP server hostname | `""` | If standard SMTP |
| SMTP_PORT | SMTP server port | `587` | If standard SMTP |
| SMTP_USERNAME | SMTP username | `""` | If standard SMTP |
| SMTP_PASSWORD | SMTP password | `""` | If standard SMTP |
| CLOUDFRONT_DOMAIN | CloudFront domain for signed downloads | `""` | If CDN downloads enabled |
| CLOUDFRONT_KEY_PAIR_ID | CloudFront key pair ID | `""` | If CDN downloads enabled |
| CLOUDFRONT_PRIVATE_KEY_PATH | CloudFront private key path | `""` | If CDN downloads enabled |
| ENABLE_CLOUDFRONT_DOWNLOADS | Use signed CloudFront URLs instead of storage URLs | `false` | No |

## Provider auto-detection

If `STORAGE_PROVIDER` is not set, the backend auto-detects it from the environment:

- `AWS_REGION`, `AWS_ACCESS_KEY_ID`, or `AWS_SECRET_ACCESS_KEY` set → `s3`
- `MINIO_ENDPOINT`, `MINIO_ACCESS_KEY`, or `MINIO_SECRET_KEY` set → `minio`
- otherwise → `gcs`

This is useful for self-hosted deployments where MinIO values are present without explicitly setting the storage provider.
