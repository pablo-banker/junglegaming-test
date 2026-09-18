MIGRATIONS_PATH := migrations

ifneq (,$(wildcard ./.env))
	include .env
	export
endif

run:
	go run ./cmd/server

test:
	go test ./...

db_up:
	docker compose up -d

db_down:
	docker compose down

migrate_create:
	migrate create -ext sql -dir "$(MIGRATIONS_PATH)" -seq $(NAME)

migrate_up:
	migrate -database "$(DATABASE_URL)" -path "$(MIGRATIONS_PATH)" up

migrate_down:
	migrate -database "$(DATABASE_URL)" -path "$(MIGRATIONS_PATH)" down 1