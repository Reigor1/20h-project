include .env
export

export PROJECT_ROOT := $(CURDIR)

env-up:
	@docker compose up -d todoapp-postgres

env-down:
	@docker compose down todoapp-postgres

env-cleanup:
	@read -p "Are you sure you want to clean up the environment? (y/n): " ans; \
	if [ "$$ans" = "y" ]; then \
		docker-compose down todoapp-postgres port-forwarder && \
		rm -rf ${PROJECT_ROOT}/out/pgdata && \
		echo "Files cleaned up!"; \
	else \
		echo "Environment cleanup aborted."; \
	fi

env-port-forward:
	@docker compose up -d port-forwarder

env-port-close:
	@docker compose down port-forwarder

migrate-create:
	@if [ -z "$(seq)" ]; then \
		echo "Required parameter missing seq. Example: make migrate-create seq=init"; \
		exit 1; \
	fi
	@docker compose run --rm todoapp-postgres-migrate \
		create \
		-ext sql \
		-dir //migrations \
		-seq "$(seq)"

migrate-up:
	@make migrate-action action=up

migrate-down:
	@make migrate-action action=down

migrate-action:
	@if [ -z "$(action)" ]; then \
		echo "Required parameter missing action. Example: make migrate-action action=up"; \
		exit 1; \
	fi
	docker compose run --rm todoapp-postgres-migrate \
		-path //migrations \
		-database postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@todoapp-postgres:5432/${POSTGRES_DB}?sslmode=disable "$(action)" $(version) \

logs-cleanup:
	@read -p "Are you sure you want to clean up the all logs? (y/n): " ans; \
	if [ "$$ans" = "y" ]; then \
		rm -rf ${PROJECT_ROOT}/out/logs && \
		echo "Files cleaned up!"; \
	else \
		echo "Logs cleanup aborted."; \
	fi

todoapp-run:
	@export LOGGER_FOLDER=${PROJECT_ROOT}/out/logs && \
	export POSTGRES_HOST=localhost && \
	go mod tidy && \
	go run ${PROJECT_ROOT}/cmd/todoapp/main.go

todoapp-deploy:
	@docker compose up -d --build todoapp

todoapp-undeploy:
	@docker compose down todoapp

swagger-gen:
	@docker compose run --rm swagger \
		init \
		-g cmd/todoapp/main.go \
		-o docs \
		--parseInternal \
		--parseDependency
ps: 
	@docker compose ps

# =====================================================
# Testing
# =====================================================

tests-integration: 
	@$(MAKE) test-integration-pgx
	@$(MAKE) test-integration-http
	@$(MAKE) test-integration-users-postgres
	@$(MAKE) test-integration-tasks-postgres
	@$(MAKE) test-integration-statistics-postgres

test-integration-pgx:
	@docker compose -p todoapp-test --env-file .env.test -f docker-compose.test.yaml up -d --wait postgres-test
	@set -a; . ${PROJECT_ROOT}/.env.test && set +a && \
		go test -count=1 -v -run \
			'^(TestPostgresPoolIntegration|TestNewPoolRejectsInvalidConfiguration)$$' \
			${PROJECT_ROOT}/tests/integration/infrastructure; \
		test_status=$$?; \
		docker compose -p todoapp-test --env-file .env.test -f docker-compose.test.yaml rm -sf postgres-test; \
		exit $$test_status

test-integration-http:
	@go test -count=1 -v -run \
		'^TestRun(ServesRequestsAndShutsDownGracefully|ReturnsListenError)$$' \
		${PROJECT_ROOT}/tests/integration/infrastructure

test-integration-users-postgres:
	@docker compose -p todoapp-test --env-file .env.test -f docker-compose.test.yaml up -d --wait postgres-test
	@set -a; . ${PROJECT_ROOT}/.env.test && set +a && \
		go test -count=1 -v -run \
			'^TestUsersRepositoryIntegration$$' \
			${PROJECT_ROOT}/tests/integration/features/users/repository/; \
		test_status=$$?; \
		docker compose -p todoapp-test --env-file .env.test -f docker-compose.test.yaml rm -sf postgres-test; \
		exit $$test_status

test-integration-tasks-postgres:
	@docker compose -p todoapp-test --env-file .env.test -f docker-compose.test.yaml up -d --wait postgres-test
	@set -a; . ${PROJECT_ROOT}/.env.test && set +a && \
		go test -count=1 -v -run \
			'^TestTasksRepositoryIntegration$$' \
			${PROJECT_ROOT}/tests/integration/features/tasks/repository/; \
		test_status=$$?; \
		docker compose -p todoapp-test --env-file .env.test -f docker-compose.test.yaml rm -sf postgres-test; \
		exit $$test_status

test-integration-statistics-postgres:
	@docker compose -p todoapp-test --env-file .env.test -f docker-compose.test.yaml up -d --wait postgres-test
	@set -a; . ${PROJECT_ROOT}/.env.test && set +a && \
		go test -count=1 -v -run \
			'^TestStatisticsRepositoryIntegration$$' \
			${PROJECT_ROOT}/tests/integration/features/statistics/repository/; \
		test_status=$$?; \
		docker compose -p todoapp-test --env-file .env.test -f docker-compose.test.yaml rm -sf postgres-test; \
		exit $$test_status

tests-e2e:
	@$(MAKE) test-e2e-api

test-e2e-api:
	@docker compose -p todoapp-test --env-file .env.test -f docker-compose.test.yaml up -d --wait postgres-test
	@set -a; . ${PROJECT_ROOT}/.env.test && set +a && \
		docker compose -p todoapp-test --env-file .env.test -f docker-compose.test.yaml exec -T postgres-test \
			sh -c 'psql -v ON_ERROR_STOP=1 -U "$$POSTGRES_USER" -d "$$POSTGRES_DB"' \
			< "${PROJECT_ROOT}/migrations/000001_init.up.sql" && \
		export LOGGER_FOLDER=${PROJECT_ROOT}/out/logs && \
		go test -count=1 -v -run \
			'^TestApiE2E$$' \
			${PROJECT_ROOT}/tests/e2e/api/; \
		test_status=$$?; \
		docker compose -p todoapp-test --env-file .env.test -f docker-compose.test.yaml rm -sf postgres-test; \
		exit $$test_status

# =====================================================
# Observability
# =====================================================

# Start all observability services in order.
observability-up:
	@$(MAKE) otel-collector-up
	@$(MAKE) prometheus-up
	@$(MAKE) grafana-up

# Stop all observability services in reverse order.
observability-down:
	@$(MAKE) grafana-down
	@$(MAKE) prometheus-down
	@$(MAKE) otel-collector-down

# Start OpenTelemetry Collector.
otel-collector-up:
	docker compose up -d otel-collector

# Stop OpenTelemetry Collector.
otel-collector-down:
	docker compose stop otel-collector

# Start Prometheus.
prometheus-up:
	docker compose up -d prometheus

# Stop Prometheus.
prometheus-down:
	docker compose stop prometheus

# Start Grafana.
grafana-up:
	docker compose up -d grafana

# Stop Grafana.
grafana-down:
	docker compose stop grafana