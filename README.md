# Teka

[![API CI](https://github.com/0pen-future/teka/actions/workflows/api-ci.yml/badge.svg)](https://github.com/0pen-future/teka/actions/workflows/api-ci.yml)
[![Web CI](https://github.com/0pen-future/teka/actions/workflows/web-ci.yml/badge.svg)](https://github.com/0pen-future/teka/actions/workflows/web-ci.yml)
[![Security](https://github.com/0pen-future/teka/actions/workflows/security.yml/badge.svg)](https://github.com/0pen-future/teka/actions/workflows/security.yml)

Full-stack application: Go API (Gin + GORM + PostgreSQL) and React web app
(TypeScript + Vite + Tailwind + shadcn/ui) in a single monorepo.

## Prerequisites

- Git, Make
- Go 1.22+
- Node.js 20+
- Docker with the Compose plugin

## Quickstart

```bash
make setup   # check tools, create .env, install git hooks
make dev     # start Postgres + API + web via Docker Compose
```

Then open http://localhost:5173 (web), http://localhost:8080/healthz (API),
http://localhost:8081 (Adminer).

## Commands

Run `make help` for the full annotated list. The everyday ones:

| Command | What it does |
|---------|--------------|
| `make setup` | Check tools, create `.env`, install git hooks |
| `make dev` / `make dev-down` | Start / stop the Docker Compose dev stack |
| `make dev-nuke` | Stop the stack and wipe volumes (destroys local DB data) |
| `make test` | All backend + frontend tests (`test-api`, `test-web`) |
| `make lint` | Lint both apps (`lint-api`, `lint-web`) |
| `make e2e` | Playwright end-to-end tests against the running dev stack |
| `make api-docs` | Regenerate the OpenAPI spec from swag annotations |
| `make migrate-up` / `make seed` | Apply migrations / seed dev users |
| `make build` | Build both production Docker images |

CI (`.github/workflows/`) runs the same targets — a green local run means a
green pipeline. See [`docs/deployment.md`](./docs/deployment.md) for how the
production images are published and deployed.

## Repository layout

| Path | Contents |
|------|----------|
| `apps/api/` | Go backend — feature-oriented modules, Cobra CLI, golang-migrate migrations |
| `apps/web/` | React frontend — feature-oriented modules, Vite, shadcn/ui |
| `infrastructure/` | Docker/infra support files (Postgres init scripts, deploy manifests) |
| `scripts/` | Development shell scripts (setup, tool checks) |
| `docs/` | Architecture, API/frontend guidelines, local dev, deployment |
| `plans/` | Implementation plans (AgentKit) |
| `.github/` | CI/CD workflows and Dependabot configuration |

## Conventions

- Commits follow [Conventional Commits](https://www.conventionalcommits.org)
  (enforced by commitlint via lefthook; bypass a hook once with `LEFTHOOK=0`).
- Architecture and per-stack standards live in [`docs/`](./docs/architecture.md).
