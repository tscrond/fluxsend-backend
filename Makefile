.PHONY: db db-down

db:
	docker compose up -d dev-postgres

db-down:
	docker compose stop dev-postgres && docker compose rm -f dev-postgres

.PHONY: build
FRONTEND_DIR ?= ../fluxsend-frontend
BUILD_VERSION ?= latest
build:
	cd $(FRONTEND_DIR) &&\
	docker build -t fluxsend-frontend:dev . &&\
	cd - &&\
	docker build -t fluxsend-backend:dev .

.PHONY: deploy
deploy:
	docker compose up -d --force-recreate --remove-orphans && cd -

.PHONY: logs
logs:
	docker logs -f backend

.PHONY: bleedingedge
bleedingedge:
	cd $(FRONTEND_DIR) &&\
	docker build -t bobaklabs/fluxsend-frontend:$(BUILD_VERSION)-bleeding-edge . &&\
	docker push bobaklabs/fluxsend-frontend:$(BUILD_VERSION)-bleeding-edge &&\
	cd - &&\
	docker build -t bobaklabs/fluxsend-backend:$(BUILD_VERSION)-bleeding-edge . &&\
	docker push bobaklabs/fluxsend-backend:$(BUILD_VERSION)-bleeding-edge