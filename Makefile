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