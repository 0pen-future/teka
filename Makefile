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

# No .env sourcing here: the API loads the repo-root .env itself in development.
.PHONY: api-dev
api-dev: ## Run the API with hot reload (host, needs local Postgres)
	@cd $(API_DIR) && if command -v air >/dev/null 2>&1; then air; else \
		echo "air not found; running without hot reload (go install github.com/air-verse/air@latest)"; \
		go run ./cmd/api serve; fi

.PHONY: web-dev
web-dev: ## Run the web dev server on the host
	$(not_yet)

.PHONY: test
test: test-api test-web ## Run all tests

# Coverage runs only over packages that contain tests (go list filter) while
# -coverpkg still attributes coverage across the whole module. Passing ./...
# directly would make `go test` synthesize empty profiles for test-less
# packages via the covdata tool, which auto-downloaded Go toolchains lack.
API_COVERAGE_FLOOR := 60
.PHONY: test-api
test-api: ## Run backend unit + integration tests; fails under the coverage floor (needs Docker)
	@cd $(API_DIR) && go test -tags=integration -coverpkg=./... -coverprofile=coverage.out \
		$$(go list -tags=integration -f '{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}' ./...) \
		&& go tool cover -func=coverage.out | tail -1 \
		| awk -v floor=$(API_COVERAGE_FLOOR) '{c=$$3+0; printf "total coverage: %.1f%% (floor %d%%)\n", c, floor; exit c<floor}'

.PHONY: test-api-unit
test-api-unit: ## Run backend unit + HTTP tests only (fast, no Docker)
	@cd $(API_DIR) && go test -short ./...

.PHONY: coverage-api
coverage-api: ## Open the HTML coverage report from the last test-api run
	@cd $(API_DIR) && go tool cover -html=coverage.out

.PHONY: api-docs
api-docs: ## Regenerate the OpenAPI spec from swag annotations
	@cd $(API_DIR) && go tool swag init -g cmd/api/main.go -o docs --parseInternal

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
	@cd $(API_DIR) && golangci-lint run

.PHONY: lint-web
lint-web: ## Lint frontend (eslint, prettier, tsc)
	$(not_yet)

.PHONY: fmt
fmt: ## Format all source
	$(not_yet)

.PHONY: migrate-up
migrate-up: ## Apply database migrations
	@cd $(API_DIR) && go run ./cmd/api migrate up

.PHONY: migrate-down
migrate-down: ## Roll back the last migration
	@cd $(API_DIR) && go run ./cmd/api migrate down

.PHONY: migrate-status
migrate-status: ## Show migration status
	@cd $(API_DIR) && go run ./cmd/api migrate status

.PHONY: seed
seed: ## Seed the database with development data
	@cd $(API_DIR) && go run ./cmd/api seed

.PHONY: build
build: build-api build-web ## Build production artifacts for both apps

.PHONY: build-api
build-api: ## Build the API binary
	@cd $(API_DIR) && CGO_ENABLED=0 go build -o bin/api ./cmd/api && echo "built $(API_DIR)/bin/api"

.PHONY: build-web
build-web: ## Build the web production bundle / image
	$(not_yet)
