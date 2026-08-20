SHELL := /bin/bash
.PHONY: db
db: ## create db
	docker compose up -d dev-postgres

.PHONY: db-down
db-down: ## remove db
	docker compose stop dev-postgres && docker compose rm -f dev-postgres

.PHONY: build
FRONTEND_DIR ?= ../fluxsend-frontend
BUILD_VERSION ?= latest
build: ## build app
	cd $(FRONTEND_DIR) &&\
	docker build -t fluxsend-frontend:dev . &&\
	cd - &&\
	docker build -t fluxsend-backend:dev .

.PHONY: deploy
deploy: ## deploy backend and frontend (Test)
	docker compose up -d --force-recreate --remove-orphans

.PHONY: logs
logs: ## view logs
	docker logs -f backend

.PHONY: swagger
swagger: ## generate swagger docs
	go run github.com/swaggo/swag/cmd/swag@v1.16.6 init -g cmd/main.go -o docs --generatedTime=false && \
	go run ./cmd/docsgen

.PHONY: docs-deps
docs-deps: ## install docs Python dependencies into .venv
	python3 -m venv .venv
	.venv/bin/python3 -m pip install -r requirements.txt

.PHONY: docs-build
docs-build: swagger ## build mkdocs
	.venv/bin/python3 -m mkdocs build --config-file mkdocs.yml

.PHONY: docs-serve
docs-serve: swagger ## serve docs
	.venv/bin/python3 -m mkdocs serve --config-file mkdocs.yml -a 0.0.0.0:1234

.PHONY: docs-image
docs-image: ## build docs docker image
	docker build -t fluxsend-docs:dev -f Dockerfile.docs .

.PHONY: add-doc
add-doc: ## prompt to add a new doc page with gum
	@gum style --border double --padding "1 2" "Create a new doc page"; \
	DOC_TITLE=$$(gum input --placeholder "e.g. Troubleshooting"); \
	DOC_FILE=$$(gum input --placeholder "e.g. troubleshooting (no .md)"); \
	[ -n "$$DOC_TITLE" ] && [ -n "$$DOC_FILE" ] || { echo "Aborted — both fields required"; exit 1; }; \
	touch "docs-site/$$DOC_FILE.md"; \
	sed -i "/^plugins:/i\  - $$DOC_TITLE: $$DOC_FILE.md" mkdocs.yml; \
	gum style --foreground 212 "Created docs-site/$$DOC_FILE.md and added nav entry"

.PHONY: bleedingedge
bleedingedge: ## build & push bleeding edge version of containers
	cd $(FRONTEND_DIR) &&\
	docker build -t bobaklabs/fluxsend-frontend:$(BUILD_VERSION)-bleeding-edge . &&\
	docker push bobaklabs/fluxsend-frontend:$(BUILD_VERSION)-bleeding-edge &&\
	cd - &&\
	docker build -t bobaklabs/fluxsend-backend:$(BUILD_VERSION)-bleeding-edge . &&\
	docker push bobaklabs/fluxsend-backend:$(BUILD_VERSION)-bleeding-edge

.PHONY: docs-all
docs-all: swagger docs-build docs-image ## execute full docs build
	docker compose up -d --build
# ---------------------------
# Help
# ---------------------------
.PHONY: help
help: ## View help
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
	awk 'BEGIN {FS = ":.*?## "}; {printf "\t\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
