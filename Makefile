MIGRATIONS_PATH := migrations

ifneq (,$(wildcard ./.env))
	include .env
	export
endif

run:
	go run ./cmd/server

# ==============================================================================
# TEST COMMANDS
# ==============================================================================

test_unit:
	go test -v -race -tags=unit -count=1 -coverprofile=coverage_unit.out ./...

test_integration:
	go test -v -race -tags=integration -count=1 -coverprofile=coverage_integration.out ./tests/integration

test_e2e:
	go test -v -race -count=1 -tags=e2e ./tests/e2e

test_e2e_multi:
	E2E_API_URL_1=http://localhost:8080 \
	E2E_API_URL_2=http://localhost:8082 \
	E2E_API_URL_3=http://localhost:8083 \
	go test -v -race -count=1 -tags="e2e multiprocess" ./tests/e2e -run TestE2EMultiProcess

test_e2e_restart:
	E2E_RESTART_PORT=8090 go test -v -race -count=1 -tags="e2e restart" ./tests/e2e -run TestE2ERestart

test_cover:
	go tool cover -html=$(or $(FILE),coverage_unit.out)

# ==============================================================================
# INFRA AND DATABASE
# ==============================================================================

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

keycloak_up:
	docker compose up -d keycloak

keycloak_down:
	docker compose stop keycloak

keycloak_reset:
	docker compose rm -sf keycloak
	docker compose up -d keycloak

sqs_up:
	docker compose up -d ministack

sqs_down:
	docker compose stop ministack

sqs_logs:
	docker compose logs -f ministack