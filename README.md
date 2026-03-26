# Dropper Backend

This is the backend application for the Dropper frontend.
It handles file uploads to a Google Cloud Storage (GCS) bucket.

## Features

- Uploading files to the cloud storage (GCS)
- Displaying metadata about stored and received objects
- Sharing files in the store via email or private/public links
- Previewing supported file types (image, video, audio, pdf)
- Adding notes attached to stored files

## Prerequisites

- Go installed on your system.
- Docker and Docker Compose for containerization.
- A Google Cloud project with a GCS bucket and appropriate credentials.

## Getting Started

### Clone the Repository

```bash
git clone https://github.com/tscrond/dropper.git
cd dropper
```

### Set Up Environment Variables

Create a `.env` file in the root directory and add the following variables:

.env:

```env
#!/bin/bash

DROPPER_LISTEN_PORT=3000
GCS_BUCKET_NAME="dropper-app"
GOOGLE_APPLICATION_CREDENTIALS=<redacted>

GOOGLE_PROJECT_ID=<redacted>
GOOGLE_COOKIE_SECRET=<redacted>
GOOGLE_CLIENT_ID=<redacted>
GOOGLE_CLIENT_SECRET=<redacted>

# for SES email provider
AWS_ACCESS_KEY_ID=<redacted>
AWS_SECRET_ACCESS_KEY=<redacted>
AWS_REGION=<redacted>

# for "standard" email provider
SMTP_HOST=<redacted>
SMTP_PORT="587"
SMTP_USERNAME=<redacted>
SMTP_PASSWORD=<redacted>
MAIL_FROM="noreply@fluxsend.com"

POSTGRES_USER="devuser"
POSTGRES_PASSWORD="devpass" 
POSTGRES_DB="devdb"
DB_HOST="localhost"

FRONTEND_ENDPOINT="http://localhost:5173"
BACKEND_ENDPOINT="http://localhost:3000"
```

.envs:

```env
#!/bin/bash

CURRENT_IP="localhost"

export DROPPER_LISTEN_PORT=3000
export GCS_BUCKET_NAME="dropper-app"
export GOOGLE_APPLICATION_CREDENTIALS=<redacted>

export GOOGLE_PROJECT_ID=<redacted>
export GOOGLE_COOKIE_SECRET=<redacted>
export GOOGLE_CLIENT_ID=<redacted>
export GOOGLE_CLIENT_SECRET=<redacted>

# for SES email provider
export AWS_ACCESS_KEY_ID=<redacted>
export AWS_SECRET_ACCESS_KEY=<redacted>
export AWS_REGION=<redacted>

# for "standard" email provider
export SMTP_HOST=<redacted>
export SMTP_PORT="587"
export SMTP_USERNAME=<redacted>
export SMTP_PASSWORD=<redacted>
export MAIL_FROM="noreply@fluxsend.com"

export POSTGRES_USER="devuser"
export POSTGRES_PASSWORD="devpass" 
export POSTGRES_DB="devdb"
export DB_HOST="localhost"

export FRONTEND_ENDPOINT="http://${CURRENT_IP}:5173"
export BACKEND_ENDPOINT="http://${CURRENT_IP}:3000"
```

### Build and Run with Docker

```bash
docker-compose up --build
```

This will build the Docker image and start the backend service.

# API Endpoints

## 🔐 Authentication Endpoints

| Method | Endpoint              | Description |
|--------|-----------------------|-------------|
| ANY    | `/auth/callback`      | Callback endpoint for OAuth authentication. Handles the redirect after a user successfully logs in via the OAuth provider. |
| ANY    | `/auth/oauth`         | Initiates the OAuth login flow. Redirects the user to the OAuth provider's authentication page. |
| ANY    | `/auth/is_valid`      | Validates the current session or token to confirm if the user is authenticated. |
| ANY    | `/auth/logout`        | Logs the user out by clearing session data or revoking tokens. |

## 📁 File Handling Endpoints

| Method | Endpoint              | Description |
|--------|-----------------------|-------------|
| POST   | `/files/upload`       | Authenticated. Uploads a file to the user's cloud storage (likely GCS). |
| POST   | `/files/share`        | Authenticated. Shares a file with another user. |
| GET    | `/files/received`     | Authenticated. Retrieves files that have been shared with the current user. |

## 📥 Download Endpoints

| Method | Endpoint                  | Description |
|--------|---------------------------|-------------|
| GET    | `/d/private/{token}`      | Authenticated. Allows users to download their private files via a secure token. |
| GET    | `/d/{token}`              | Public. Proxy for downloading shared files using a token (possibly with time-limited access). |

## 👤 User Info Endpoints

| Method | Endpoint                         | Description |
|--------|----------------------------------|-------------|
| GET    | `/user/data`                     | Authenticated. Returns profile or account details of the current user. |
| GET    | `/user/bucket`                   | Authenticated. Provides details about the user’s GCS bucket (e.g., usage, files, etc.). |
| POST   | `/user/private/download_token`   | Authenticated. Generates a download token for a private file (used with `/d/private/{token}`). |

## Project Structure

- `cmd/`: Entry point of the application.
- `internal/`: Internal packages for business logic.
- `pkg/`: Shared packages across the application.
- `Dockerfile`: Docker configuration for the backend service.
- `docker-compose.yaml`: Docker Compose configuration for multi-container setup.

## License

This project is licensed under the MIT License.
