# Prerequisites

The following table represents software needed to run/contribute to the project.

| Requirement | Version | For running | For contributing |
|---|---|---|---|
| Go | 1.26+ | ✓ | ✓ |
| GNU Make | — | ✓ | ✓ |
| Docker | latest | ✓ | ✓ |
| Docker Compose | v2 | ✓ | ✓ |
| PostgreSQL | 16 | ✓ (via Docker) | — |
| Cloud storage (GCS or S3) | — | ✓ | — |
| OAuth app (Google and/or GitHub) | — | ✓ | — |
| Email provider (SMTP or SES) | — | ✓ | — |
| Python 3.13+ | 3.14+ | — | ✓ (docs) |
| MkDocs + mkdocs-material + mkdocs-macros-plugin | latest in venv | — | ✓ (docs) |
| air (cosmtrek/air) | latest | — | ✓ (hot reload) |
| sqlc | v2 | — | ✓ (SQL codegen) |
| swag (swaggo/swag/cmd/swag) | v1.16.6 | — | ✓ (Swagger gen) |
| mockgen (go.uber.org/mock) | v0.6.0 | — | ✓ (mock gen) |
| govulncheck | latest | — | ✓ (security check) |
| gum (charmbracelet/gum) | latest | — | ✓ (make add-doc) |