SHELL := /bin/bash
.DEFAULT_GOAL := help

API_DIR := apps/api
WEB_DIR := apps/web

# Targets whose implementation lands in a later provisioning phase print a
# notice instead of failing confusingly.
define not_yet
	@echo "'$@' is provisioned in a later phase (see plans/). Nothing to do yet."
endef

.PHONY: help
help: ## Show available commands
	@grep -E '^[a-zA-Z0-9_.-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

.PHONY: setup
setup: ## Bootstrap dev environment (tools check, .env, git hooks)
	@./scripts/setup.sh

.PHONY: hooks
hooks: ## (Re)install git hooks
	@npx lefthook install

.PHONY: dev
dev: ## Start full stack via Docker Compose
	@if [ -f docker-compose.yml ]; then docker compose up --build; else echo "'dev' needs docker-compose.yml (provisioned in Phase 7)."; fi

.PHONY: dev-down
dev-down: ## Stop the Docker Compose stack
	@if [ -f docker-compose.yml ]; then docker compose down; else echo "'dev-down' needs docker-compose.yml (provisioned in Phase 7)."; fi

.PHONY: dev-nuke
dev-nuke: ## Stop the stack and remove volumes (destroys local DB data)
	@if [ -f docker-compose.yml ]; then docker compose down -v; else echo "'dev-nuke' needs docker-compose.yml (provisioned in Phase 7)."; fi

.PHONY: dev-logs
dev-logs: ## Tail api and web logs
	@if [ -f docker-compose.yml ]; then docker compose logs -f api web; else echo "'dev-logs' needs docker-compose.yml (provisioned in Phase 7)."; fi

.PHONY: api-dev
api-dev: ## Run the API with hot reload (host, needs local Postgres)
	$(not_yet)

.PHONY: web-dev
web-dev: ## Run the web dev server on the host
	$(not_yet)

.PHONY: test
test: test-api test-web ## Run all tests

.PHONY: test-api
test-api: ## Run backend unit + integration tests
	$(not_yet)

.PHONY: test-web
test-web: ## Run frontend tests
	$(not_yet)

.PHONY: e2e
e2e: ## Run Playwright end-to-end tests (expects 'make dev' stack)
	$(not_yet)

.PHONY: lint
lint: lint-api lint-web ## Lint both apps

.PHONY: lint-api
lint-api: ## Lint backend (golangci-lint)
	$(not_yet)

.PHONY: lint-web
lint-web: ## Lint frontend (eslint, prettier, tsc)
	$(not_yet)

.PHONY: fmt
fmt: ## Format all source
	$(not_yet)

.PHONY: migrate-up
migrate-up: ## Apply database migrations
	$(not_yet)

.PHONY: migrate-down
migrate-down: ## Roll back the last migration
	$(not_yet)

.PHONY: migrate-status
migrate-status: ## Show migration status
	$(not_yet)

.PHONY: seed
seed: ## Seed the database with development data
	$(not_yet)

.PHONY: build
build: build-api build-web ## Build production artifacts for both apps

.PHONY: build-api
build-api: ## Build the API binary / image
	$(not_yet)

.PHONY: build-web
build-web: ## Build the web production bundle / image
	$(not_yet)
