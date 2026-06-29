# Environment variables

The following table documents existing environment variables and their purpose in the application.

| Variable | Description | Defaults | Mandatory |
|---|---|---|---|
| APP_ENV | Sets logger mode to production or development <b>(Available: "production", "development")</b> | production | false |
| AWS_ACCESS_KEY_ID | ID of AWS access key used for S3 bucket provider | "" | true |
| AWS_REGION | AWS region for the S3 bucket | "" | true |
| AWS_SECRET_ACCESS_KEY | AWS Secret Access Key for S3 bucket | "" | true |
| BACKEND_ENDPOINT | Used for OAuth2 callback endpoints and constructing sharing URLs | "" | true |
| FRONTEND_ENDPOINT | Used for frontend redirections in the OAuth2 flow | "" | false |
| CLOUDFRONT_DOMAIN | Domain served by AWS Cloudfront for files | "" | false |
| CLOUDFRONT_KEY_PAIR_ID | Cloudfront Key Pair ID | "" | false |
| CLOUDFRONT_PRIVATE_KEY_PATH | Cloudfront Private Key Path | "" | false |
| FLUXSEND_API_LISTEN_PORT | Listen port for Developer API service | "8091" | true |
| FLUXSEND_API_ROUTE_PREFIX | Prefix for Developer API routes | "/api" | true |
| FLUXSEND_LISTEN_PORT | Listen port for main backend service | "3000" | false |
| GCS_BUCKET_NAME | GCS bucket name <b>(mandatory if using the GCS backend)</b> | "" | false |
| GITHUB_OAUTH_CLIENT_ID | Github OAuth2 App client ID | "" | true |
| GITHUB_OAUTH_CLIENT_SECRET | Github OAuth2 App client secret | "" | true |
| GOOGLE_APPLICATION_CREDENTIALS | JSON credentials for service account with GCS access <b>(mandatory if using the GCS backend)</b> | "" | false |
| GOOGLE_CLIENT_ID | Google Client ID - needed for Google OAuth2 | | true |
| GOOGLE_CLIENT_SECRET | needed for Google OAuth2 | | true |
| GOOGLE_PROJECT_ID | needed for Google OAuth2 (and GCS backend if applicable) | | true |
| DB_HOST | Database host - needed for connection string | "" | true |
| POSTGRES_DB | Postgres database name | "" | true |
| POSTGRES_PASSWORD | Postgres DB password | "" | true |
| POSTGRES_USER | Postgres user | "" | true |
| MAIL_FROM | Email address permitted to send email on behalf of FluxSend | "noreply@fluxsend.win" | true |
| SMTP_HOST | SMTP Server host | "" | true |
| SMTP_PASSWORD | Password for SMTP permitted sender | "" | true |
| SMTP_PORT | Port for SMTP communication | "587" | true |
| SMTP_USERNAME | Username for SMTP user | "" | true |
| STORAGE_PROVIDER | Storage provider <b>(Available: "s3", "gcs")</b> | "s3" | true |
| TOKEN_ENCRYPTION_KEY | Encryption key for session management | "" | true |
