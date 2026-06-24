# Getting Started

## Prerequisites

- Go 1.26 or newer.
- Docker and Docker Compose.
- A database and cloud storage configuration that matches your chosen backend mode.

## Local setup

Create a `.env` file at the repository root and set the backend, database, OAuth, and storage variables required by your environment.

Start the backend stack with Docker Compose:

```bash
docker compose up --build
```

## Useful endpoints

- Backend API: `http://localhost:3000`
- CLI/API server: `http://localhost:8091`
- Frontend: `http://localhost:8000`