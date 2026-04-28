.PHONY: db db-down

db:
	docker compose up -d dev-postgres

db-down:
	docker compose stop dev-postgres && docker compose rm -f dev-postgres

.PHONY: build
build:
	cd /home/tskr/projects/fluxsend-frontend &&\
	docker build -t fluxsend-frontend:dev . &&\
	cd - &&\
	docker build -t fluxsend-backend:dev .

.PHONY: deploy
deploy:
	docker compose up -d --force-recreate --remove-orphans && cd -