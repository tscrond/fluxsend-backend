# Installing Tools

Below are installation commands for each tool needed to develop FluxSend. Skip any you already have.

## Go

Download from [go.dev](https://go.dev/dl/) or use a package manager:

```bash
# Ubuntu / Debian
sudo apt install golang-go

# macOS (Homebrew)
brew install go

# Verify
go version
```

The project requires Go 1.26 or newer.

## GNU Make

```bash
# Ubuntu / Debian
sudo apt install make

# macOS
xcode-select --install

# Verify
make --version
```

## Docker & Docker Compose

Install [Docker Desktop](https://docs.docker.com/get-docker/) or the [Docker Engine](https://docs.docker.com/engine/install/):

```bash
# Ubuntu / Debian
sudo apt install docker-ce docker-ce-cli containerd.io docker-compose-plugin

# Verify
docker --version
docker compose version
```

Compose v2 is bundled with Docker Desktop and the Docker engine package.

## Python 3.13+ & MkDocs

```bash
# Ubuntu / Debian
sudo apt install python3 python3-venv python3-pip

# macOS (Homebrew)
brew install python@3.14
```

The project venv is at `.venv/`. Create or update it:

```bash
python3 -m venv .venv
.venv/bin/pip install mkdocs mkdocs-material mkdocs-macros-plugin
```

Verify:

```bash
.venv/bin/mkdocs --version
```

## air (hot reload)

```bash
go install github.com/air-verse/air@latest

# Verify
air -v
```

## sqlc (SQL code generation)

```bash
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest

# Verify
sqlc version
```

## swag (Swagger doc generator)

No manual install needed — `make swagger` runs it via `go run`. The Makefile pins version `v1.16.6`.

## mockgen (mock generation)

```bash
go install go.uber.org/mock/mockgen@latest

# Verify
mockgen -version
```

## govulncheck (security scanner)

```bash
go install golang.org/x/vuln/cmd/govulncheck@latest

# Verify
govulncheck -version
```

## gum (interactive CLI helper for `make add-doc`)

```bash
# macOS (Homebrew)
brew install gum

# Linux — download from github.com/charmbracelet/gum/releases
# or use the Go install approach:
go install github.com/charmbracelet/gum@latest

# Verify
gum --version
```

## Verify all at once

```bash
go version && make --version && docker --version && docker compose version && \
python3 --version && sqlc version && air -v && mockgen -version && govulncheck -version
```
