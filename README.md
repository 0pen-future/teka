# Teka

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
http://localhost:8081 (Adminer). Run `make help` for the full command list.

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
