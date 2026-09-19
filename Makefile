ifneq (,$(wildcard ./.env))
	include .env
	export
endif

POSTGRES_USER ?= postgres
POSTGRES_PASSWORD ?= postgres
POSTGRES_DB ?= junglegaming
POSTGRES_TEST_DB ?= junglegaming_test

# Migrations run inside the compose network, so the migrate CLI is not required on the host.
MIGRATE := docker compose run --rm --no-deps migrate -path=/migrations
OWNER_DATABASE := postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@postgres:5432/$(POSTGRES_DB)?sslmode=disable
TEST_DATABASE := postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@postgres_test:5432/$(POSTGRES_TEST_DB)?sslmode=disable
SCRATCH_DATABASE := postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@postgres_test:5432/migration_check?sslmode=disable
TEST_PSQL := docker compose exec -T postgres_test psql -v ON_ERROR_STOP=1 -U $(POSTGRES_USER) -d postgres

.PHONY: up up_multi down logs run fmt vet test test_race test_unit test_integration test_e2e test_e2e_multi test_e2e_restart \
	test_deps_up test_migrations db_app_role migrate_up migrate_down migrate_tests_up migrate_tests_down

# ==============================================================================
# RUN
# ==============================================================================

# Application, migrations, PostgreSQL, Keycloak and MiniStack.
up:
	docker compose up --build -d

# Same stack with three independent application instances (8080, 8082, 8083).
up_multi:
	docker compose --profile multi up --build -d

down:
	docker compose --profile multi --profile test down

logs:
	docker compose logs -f app

# Runs the application on the host against the compose dependencies.
run:
	go run ./cmd/server

# ==============================================================================
# QUALITY
# ==============================================================================

fmt:
	gofmt -w cmd internal tests

vet:
	go vet ./...
	go vet -tags=integration ./tests/integration
	go vet -tags="e2e multiprocess restart" ./tests/e2e

# ==============================================================================
# TESTS
# ==============================================================================

test:
	go test ./...

test_race:
	go test -race ./...

test_unit:
	go test -v -race -count=1 -coverprofile=coverage_unit.out ./internal/...

# Test database, Keycloak and MiniStack for integration tests.
test_deps_up:
	docker compose up -d --wait postgres_test keycloak ministack
	docker compose run --rm --no-deps migrate -path=/migrations -database="$(TEST_DATABASE)" up

test_integration:
	go test -v -race -tags=integration -count=1 -coverprofile=coverage_integration.out ./tests/integration

# Requires `make up`.
test_e2e:
	go test -v -race -count=1 -tags=e2e ./tests/e2e

# Requires `make up_multi`.
test_e2e_multi:
	E2E_API_URL_1=http://localhost:8080 \
	E2E_API_URL_2=http://localhost:8082 \
	E2E_API_URL_3=http://localhost:8083 \
	go test -v -race -count=1 -tags="e2e multiprocess" ./tests/e2e -run TestE2EMultiProcess

# Builds and restarts its own server process on port 8090 against the compose dependencies.
test_e2e_restart:
	E2E_RESTART_PORT=8090 go test -v -race -count=1 -tags="e2e restart" ./tests/e2e -run TestE2ERestart

# ==============================================================================
# MIGRATIONS
# ==============================================================================

# Sets the login of the application role on a volume created before 01-app-role.sh existed.
db_app_role:
	docker compose exec postgres bash /docker-entrypoint-initdb.d/01-app-role.sh

migrate_up:
	$(MIGRATE) -database="$(OWNER_DATABASE)" up

migrate_down:
	$(MIGRATE) -database="$(OWNER_DATABASE)" down 1

migrate_tests_up:
	$(MIGRATE) -database="$(TEST_DATABASE)" up

migrate_tests_down:
	$(MIGRATE) -database="$(TEST_DATABASE)" down 1

# Applies every migration, reverts all of them and applies them again on a scratch database.
# Requires `make test_deps_up`.
test_migrations:
	$(TEST_PSQL) -c "DROP DATABASE IF EXISTS migration_check" -c "CREATE DATABASE migration_check"
	$(MIGRATE) -database="$(SCRATCH_DATABASE)" up
	$(MIGRATE) -database="$(SCRATCH_DATABASE)" down -all
	$(MIGRATE) -database="$(SCRATCH_DATABASE)" up
	$(TEST_PSQL) -c "DROP DATABASE migration_check"
