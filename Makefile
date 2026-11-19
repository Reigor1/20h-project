# =====================================================
# Project Configuration
# =====================================================

# Load environment variables from the project .env file.
include .env
# Export all variables loaded from .env to child processes.
export

# Define the absolute path to the project root.
export PROJECT_ROOT := $(CURDIR)


# =====================================================
# Environment Management
# =====================================================

# ----------------------------
# PostgreSQL
# ----------------------------

# Start the PostgreSQL development environment.
env-postgres-up:
	@docker compose up -d todoapp-postgres

# Stop the PostgreSQL development environment.
env-postgres-down:
	@docker compose down todoapp-postgres

# Stop and remove environment containers and PostgreSQL data.
# Requires explicit user confirmation before deleting data.
env-postgres-cleanup:
	@read -p "Are you sure you want to clean up the environment? (y/n): " ans; \
	if [ "$$ans" = "y" ]; then \
		docker-compose down todoapp-postgres port-forwarder && \
		rm -rf ${PROJECT_ROOT}/out/pgdata && \
		echo "Files cleaned up!"; \
	else \
		echo "Environment cleanup aborted."; \
	fi

# Start the port forwarding service.
env-port-forward:
	@docker compose up -d port-forwarder

# Stop the port forwarding service.
env-port-close:
	@docker compose down port-forwarder

# ----------------------------
# Redis
# ----------------------------

# Start the Redis development environment.
env-redis-up:
	@docker compose up -d redis

# Stop the Redis development environment.
env-redis-down:
	@docker compose stop redis

# =====================================================
# Database Migrations
# =====================================================

# Create a new sequential SQL migration.
# Usage: make migrate-create seq=init
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

# Apply all available database migrations.
migrate-up:
	@make migrate-action action=up

# Roll back the last database migration.
migrate-down:
	@make migrate-action action=down

# Execute a database migration action.
# Usage: make migrate-action action=up
# Optional: make migrate-action action=down version=1
migrate-action:
	@if [ -z "$(action)" ]; then \
		echo "Required parameter missing action. Example: make migrate-action action=up"; \
		exit 1; \
	fi
	docker compose run --rm todoapp-postgres-migrate \
		-path //migrations \
		-database postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@todoapp-postgres:5432/${POSTGRES_DB}?sslmode=disable "$(action)" $(version) \


# =====================================================
# Kafka
# =====================================================

# -----------------------------------------------------
# Kafka Cluster
# -----------------------------------------------------

# Start all three Kafka brokers and wait until they are healthy.
kafka-up:
	@docker compose up -d --wait --wait-timeout 180 kafka-1 kafka-2 kafka-3

# Stop Kafka brokers without deleting their persistent volumes.
kafka-down:
	@docker compose stop kafka-1 kafka-2 kafka-3

# Display broker status, KRaft quorum and broker API versions.
kafka-status:
	@docker compose ps kafka-1 kafka-2 kafka-3
	@docker compose exec -T kafka-1 \
		/opt/kafka/bin/kafka-metadata-quorum.sh \
		--bootstrap-server kafka-1:19092 describe --status
	@docker compose exec -T kafka-1 \
		/opt/kafka/bin/kafka-cluster.sh \
		api-versions --bootstrap-server kafka-1:19092

# -----------------------------------------------------
# Kafka Topics
# -----------------------------------------------------

KAFKA_BOOTSTRAP_SERVER ?= kafka-1:19092
KAFKA_CLI_SERVICE ?= kafka-1

KAFKA_TOPICS_CLI = docker compose exec -T $(KAFKA_CLI_SERVICE) \
	/opt/kafka/bin/kafka-topics.sh \
	--bootstrap-server $(KAFKA_BOOTSTRAP_SERVER)

KAFKA_CONFIGS_CLI = docker compose exec -T $(KAFKA_CLI_SERVICE) \
	/opt/kafka/bin/kafka-configs.sh \
	--bootstrap-server $(KAFKA_BOOTSTRAP_SERVER)

.PHONY: kafka-topics-create kafka-topics-configure kafka-topics-setup
.PHONY: kafka-topics kafka-topics-describe kafka-topics-config

# List all Kafka topics.
kafka-topics:
	@$(KAFKA_TOPICS_CLI) --list

# Create the application event topics.
# Existing topics are not recreated.
kafka-topics-create:
	@$(KAFKA_TOPICS_CLI) \
		--create \
		--if-not-exists \
		--topic task-events \
		--partitions 3 \
		--replication-factor 3 \
		--config cleanup.policy=delete \
		--config retention.ms=604800000 \
		--config min.insync.replicas=2 \
		--config unclean.leader.election.enable=false

	@$(KAFKA_TOPICS_CLI) \
		--create \
		--if-not-exists \
		--topic task-events.dlq \
		--partitions 3 \
		--replication-factor 3 \
		--config cleanup.policy=delete \
		--config retention.ms=2592000000 \
		--config min.insync.replicas=2 \
		--config unclean.leader.election.enable=false

# Apply the required configuration to existing topics.
kafka-topics-configure:
	@$(KAFKA_CONFIGS_CLI) \
		--entity-type topics \
		--entity-name task-events \
		--alter \
		--add-config cleanup.policy=delete,retention.ms=604800000,min.insync.replicas=2,unclean.leader.election.enable=false

	@$(KAFKA_CONFIGS_CLI) \
		--entity-type topics \
		--entity-name task-events.dlq \
		--alter \
		--add-config cleanup.policy=delete,retention.ms=2592000000,min.insync.replicas=2,unclean.leader.election.enable=false

# Create topics and enforce their configuration.
kafka-topics-setup:
	@$(MAKE) kafka-topics-create
	@$(MAKE) kafka-topics-configure

# Display partition, leader, replica and ISR information.
kafka-topics-describe:
	@$(KAFKA_TOPICS_CLI) --describe --topic task-events
	@$(KAFKA_TOPICS_CLI) --describe --topic task-events.dlq

# Display topic-level configuration.
kafka-topics-config:
	@$(KAFKA_CONFIGS_CLI) \
		--entity-type topics \
		--entity-name task-events \
		--describe

	@$(KAFKA_CONFIGS_CLI) \
		--entity-type topics \
		--entity-name task-events.dlq \
		--describe

# -----------------------------------------------------
# Kafka UI
# -----------------------------------------------------

# Start the Kafka UI container.
kafka-ui-up:
	@docker compose up -d kafka-ui

# Stop the Kafka UI container without affecting other services.
kafka-ui-down:
	@docker compose stop kafka-ui


# =====================================================
# Application
# =====================================================

ACTIVITY_WORKER_REPLICAS ?= 3

# -----------------------------------------------------
# Local
# -----------------------------------------------------

# Start infrastructure and Activity Workers in Docker,
# then run the application locally.
todoapp-run-locally:
	@export LOGGER_FOLDER=${PROJECT_ROOT}/out/logs && \
	export POSTGRES_HOST=localhost && \
	$(MAKE) env-redis-up && \
	$(MAKE) env-postgres-up && \
	$(MAKE) env-port-forward && \
	$(MAKE) observability-up && \
	$(MAKE) kafka-up && \
	$(MAKE) kafka-topics-setup && \
	$(MAKE) kafka-ui-up && \
	go mod tidy && \
	$(MAKE) activity-workers-up && \
	go run ${PROJECT_ROOT}/cmd/todoapp/main.go

# -----------------------------------------------------
# Docker
# -----------------------------------------------------

# Start infrastructure in Docker and run the application in Docker.
todoapp-run-in-docker:
	@export LOGGER_FOLDER=${PROJECT_ROOT}/out/logs && \
	export POSTGRES_HOST=localhost && \
	$(MAKE) env-redis-up && \
	$(MAKE) env-postgres-up && \
	$(MAKE) env-port-forward && \
	$(MAKE) observability-up && \
	$(MAKE) kafka-up && \
	$(MAKE) kafka-topics-setup && \
	$(MAKE) kafka-ui-up && \
	$(MAKE) activity-workers-up && \
	go mod tidy && \
	$(MAKE) activity-workers-up && \
	$(MAKE) todoapp-deploy

# -----------------------------------------------------
# Todoapp
# -----------------------------------------------------

# Build the application Docker image and start the container.
todoapp-deploy:
	@docker compose up -d --build todoapp

# Stop the application container without removing it.
todoapp-undeploy:
	@docker compose down todoapp

# -----------------------------------------------------
# Activity Worker
# -----------------------------------------------------

# Start three Activity Worker instances in one Consumer Group.
activity-workers-up:
	@docker compose up -d --build \
		--scale activity-worker=$(ACTIVITY_WORKER_REPLICAS) \
		activity-worker

# Stop all Activity Worker instances.
activity-workers-down:
	@docker compose stop activity-worker


# -----------------------------------------------------
# Activity DLQ Replay
# -----------------------------------------------------

# Replay one message from the Activity DLQ using its partition and offset.
# Usage: make activity-dlq-replay DLQ_PARTITION=1 DLQ_OFFSET=125
activity-dlq-replay:
	@if [ -z "$(DLQ_PARTITION)" ] || [ -z "$(DLQ_OFFSET)" ]; then \
		echo "Usage: make activity-dlq-replay DLQ_PARTITION=1 DLQ_OFFSET=125"; \
		exit 1; \
	fi
	@go run ./cmd/activity-dlq-replay \
		-partition "$(DLQ_PARTITION)" \
		-offset "$(DLQ_OFFSET)"


# =====================================================
# API Documentation
# =====================================================

# Generate Swagger documentation from application source code.
swagger-gen:
	@docker compose run --rm swagger \
		init \
		-g cmd/todoapp/main.go \
		-o docs \
		--parseInternal \
		--parseDependency


# =====================================================
# Testing
# =====================================================

# -----------------------------------------------------
# Integration Testing
# -----------------------------------------------------

# Run all integration test suites sequentially.
tests-integration: 
	@$(MAKE) test-integration-pgx
	@$(MAKE) test-integration-http
	@$(MAKE) test-integration-users-postgres
	@$(MAKE) test-integration-tasks-postgres
	@$(MAKE) test-integration-statistics-postgres
	@$(MAKE) test-integration-events-migrations

# Run PostgreSQL connection pool integration tests.
# Start the test database, load test environment variables,
# execute tests and remove the test container.
test-integration-pgx:
	@docker compose -p todoapp-test --env-file .env.test -f docker-compose.test.yaml up -d --wait postgres-test
	@set -a; . ${PROJECT_ROOT}/.env.test && set +a && \
		go test -count=1 -v -run \
			'^(TestPostgresPoolIntegration|TestNewPoolRejectsInvalidConfiguration)$$' \
			${PROJECT_ROOT}/tests/integration/infrastructure; \
		test_status=$$?; \
		docker compose -p todoapp-test --env-file .env.test -f docker-compose.test.yaml rm -sf postgres-test; \
		exit $$test_status

# Run HTTP server integration tests without external dependencies.
test-integration-http:
	@go test -count=1 -v -run \
		'^TestRun(ServesRequestsAndShutsDownGracefully|ReturnsListenError|WaitsForInFlightRequestBeforeReturning)$$' \
		${PROJECT_ROOT}/tests/integration/infrastructure

# Run PostgreSQL users repository integration tests.
test-integration-users-postgres:
	@docker compose -p todoapp-test --env-file .env.test -f docker-compose.test.yaml up -d --wait postgres-test
	@set -a; . ${PROJECT_ROOT}/.env.test && set +a && \
		go test -count=1 -v -run \
			'^TestUsersRepositoryIntegration$$' \
			${PROJECT_ROOT}/tests/integration/features/users/repository/; \
		test_status=$$?; \
		docker compose -p todoapp-test --env-file .env.test -f docker-compose.test.yaml rm -sf postgres-test; \
		exit $$test_status

# Run PostgreSQL tasks repository integration tests.
test-integration-tasks-postgres:
	@docker compose -p todoapp-test --env-file .env.test -f docker-compose.test.yaml up -d --wait postgres-test
	@set -a; . ${PROJECT_ROOT}/.env.test && set +a && \
		go test -count=1 -v -run \
			'^TestTasksRepositoryIntegration$$' \
			${PROJECT_ROOT}/tests/integration/features/tasks/repository/; \
		test_status=$$?; \
		docker compose -p todoapp-test --env-file .env.test -f docker-compose.test.yaml rm -sf postgres-test; \
		exit $$test_status

# Run PostgreSQL statistics repository integration tests.
test-integration-statistics-postgres:
	@docker compose -p todoapp-test --env-file .env.test -f docker-compose.test.yaml up -d --wait postgres-test
	@set -a; . ${PROJECT_ROOT}/.env.test && set +a && \
		go test -count=1 -v -run \
			'^TestStatisticsRepositoryIntegration$$' \
			${PROJECT_ROOT}/tests/integration/features/statistics/repository/; \
		test_status=$$?; \
		docker compose -p todoapp-test --env-file .env.test -f docker-compose.test.yaml rm -sf postgres-test; \
		exit $$test_status

# Run event storage migration integration tests.
# Apply the real migration sequence and verify unique event constraints.
test-integration-events-migrations:
	@docker compose -p todoapp-test --env-file .env.test -f docker-compose.test.yaml up -d --wait postgres-test
	@set -a; . ${PROJECT_ROOT}/.env.test && set +a && \
		go test -count=1 -v \
			${PROJECT_ROOT}/tests/integration/features/events/migrations; \
		test_status=$$?; \
		docker compose -p todoapp-test --env-file .env.test -f docker-compose.test.yaml rm -sf postgres-test; \
		exit $$test_status

# -----------------------------------------------------
# End-to-End Testing
# -----------------------------------------------------

# Run all API end-to-end test suites.
tests-e2e:
	@$(MAKE) test-e2e-api

# Run API end-to-end tests against a dedicated PostgreSQL test database.
# Start the test database, apply the initial migration,
# execute E2E tests and remove the test container.
test-e2e-api:
	@docker compose -p todoapp-test --env-file .env.test -f docker-compose.test.yaml up -d --wait postgres-test
	@set -a; . ${PROJECT_ROOT}/.env.test && set +a && \
		docker compose -p todoapp-test --env-file .env.test -f docker-compose.test.yaml exec -T postgres-test \
			sh -c 'psql -v ON_ERROR_STOP=1 -U "$$POSTGRES_USER" -d "$$POSTGRES_DB"' \
			< "${PROJECT_ROOT}/migrations/000001_init.up.sql" && \
		docker compose -p todoapp-test --env-file .env.test -f docker-compose.test.yaml exec -T postgres-test \
			sh -c 'psql -v ON_ERROR_STOP=1 -U "$$POSTGRES_USER" -d "$$POSTGRES_DB"' \
			< "${PROJECT_ROOT}/migrations/000002_create_outbox.up.sql" && \
		docker compose -p todoapp-test --env-file .env.test -f docker-compose.test.yaml exec -T postgres-test \
			sh -c 'psql -v ON_ERROR_STOP=1 -U "$$POSTGRES_USER" -d "$$POSTGRES_DB"' \
			< "${PROJECT_ROOT}/migrations/000003_create_task_activity.up.sql" && \
		export LOGGER_FOLDER=${PROJECT_ROOT}/out/logs && \
		go test -count=1 -v -run \
			'^TestApiE2E$$' \
			${PROJECT_ROOT}/tests/e2e/api/; \
		test_status=$$?; \
		docker compose -p todoapp-test --env-file .env.test -f docker-compose.test.yaml rm -sf postgres-test; \
		exit $$test_status

# -----------------------------------------------------
# Observability Testing
# -----------------------------------------------------

test-observability:
	@go run ${PROJECT_ROOT}/tests/e2e/observability

# Run load-testing profiles against an already running application.
# Usage: make test-load [PROFILE=standard|ramp-up|steady-state|stress|all]
# Optional: make test-load PROFILE=stress STRESS_WORKERS=75,100,150
PROFILE ?= standard
STRESS_WORKERS ?= 75,100,150
STAGE_DURATION ?= 30s
STEADY_WORKERS ?= 50
STEADY_DURATION ?= 5m
USERS ?= 10
TASKS_PER_USER ?= 2

test-load:
	@go run ${PROJECT_ROOT}/tests/e2e/observability \
		-scenario=load \
		-load-profile=$(PROFILE) \
		-stage-duration=$(STAGE_DURATION) \
		-steady-workers=$(STEADY_WORKERS) \
		-steady-duration=$(STEADY_DURATION) \
		-stress-workers=$(STRESS_WORKERS) \
		-users=$(USERS) \
		-tasks-per-user=$(TASKS_PER_USER)

# =====================================================
# Observability
# =====================================================

# Start all observability services in dependency order.
observability-up:
	@$(MAKE) tempo-up
	@$(MAKE) otel-collector-up
	@$(MAKE) prometheus-up
	@$(MAKE) loki-up
	@$(MAKE) alloy-up
	@$(MAKE) grafana-up

# Stop all observability services in reverse startup order.
observability-down:
	@$(MAKE) grafana-down
	@$(MAKE) alloy-down
	@$(MAKE) loki-down
	@$(MAKE) prometheus-down
	@$(MAKE) otel-collector-down
	@$(MAKE) tempo-down

# Start Tempo.
tempo-up:
	@docker compose up -d tempo

# Stop Tempo.
tempo-down:
	@docker compose stop tempo

# Start OpenTelemetry Collector.
otel-collector-up:
	@docker compose up -d otel-collector

# Stop OpenTelemetry Collector.
otel-collector-down:
	@docker compose stop otel-collector

# Start Prometheus.
prometheus-up:
	@docker compose up -d --wait prometheus kafka-exporter

# Stop Prometheus.
prometheus-down:
	@docker compose stop prometheus kafka-exporter

# Start Loki.
loki-up:
	@docker compose up -d loki

# Stop Loki.
loki-down:
	@docker compose stop loki

# Start Grafana Alloy.
alloy-up:
	@docker compose up -d alloy

# Stop Grafana Alloy.
alloy-down:
	@docker compose stop alloy

# Start Grafana.
grafana-up:
	@docker compose up -d grafana

# Stop Grafana.
grafana-down:
	@docker compose stop grafana


# =====================================================
# Diagnostics & Maintenance
# =====================================================

# Display the status of all Docker Compose services.
ps:
	@ps

# Remove all application log files.
# Requires explicit user confirmation before deleting logs.
logs-cleanup:
	@read -p "Are you sure you want to clean up the all logs? (y/n): " ans; \
	if [ "$$ans" = "y" ]; then \
		rm -rf ${PROJECT_ROOT}/out/logs && \
		echo "Files cleaned up!"; \
	else \
		echo "Logs cleanup aborted."; \
	fi

# Restart one or more Docker Compose services.
# Usage: make docker-restart SERVICES="alloy grafana"
.PHONY: docker-restart
docker-restart:
	@if [ -z "$(strip $(SERVICES))" ]; then \
		echo 'Usage: make docker-restart SERVICES="alloy grafana"'; \
		exit 1; \
	fi
	@PROJECT_ROOT="$(PROJECT_ROOT)" docker compose restart $(SERVICES)