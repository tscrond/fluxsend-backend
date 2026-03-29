.PHONY: db db-down

db:
	docker compose up -d dev-postgres

db-down:
	docker compose stop dev-postgres && docker compose rm -f dev-postgres
