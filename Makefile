MIGRATIONS_PATH := migrations

ifneq (,$(wildcard ./.env))
	include .env
	export
endif

run:
	go run ./cmd/server

test:
	go test ./...

test_integration:
	go test ./tests/integration -count=1

db_up:
	docker compose up -d postgres

db_down:
	docker compose stop postgres

db_tests_up:
	docker compose up -d postgres_test

db_tests_down:
	docker compose stop postgres_test

migrate_create:
	migrate create -ext sql -dir "$(MIGRATIONS_PATH)" -seq $(NAME)

migrate_up:
	migrate -database "$(DATABASE_URL)" -path "$(MIGRATIONS_PATH)" up

migrate_down:
	migrate -database "$(DATABASE_URL)" -path "$(MIGRATIONS_PATH)" down 1

migrate_tests_up:
	migrate -database "$(TEST_DATABASE_URL)" -path "$(MIGRATIONS_PATH)" up

migrate_tests_down:
	migrate -database "$(TEST_DATABASE_URL)" -path "$(MIGRATIONS_PATH)" down 1