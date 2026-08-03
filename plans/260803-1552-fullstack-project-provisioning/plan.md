---
title: "Fullstack Project Provisioning"
description: "Provision a production-ready monorepo with a Go/Gin/GORM/PostgreSQL feature-oriented API and a React/TypeScript/Vite/Tailwind/shadcn feature-oriented web app, plus Docker Compose local dev, tooling, and CI/CD."
status: in-progress
priority: P1
effort: "6d"
tags: [go, gin, gorm, postgres, cobra, react, typescript, vite, tailwind, shadcn, docker, monorepo]
created: 2026-08-03
---

# Fullstack Project Provisioning

## Overview

Provision a production-ready full-stack project skeleton from an empty repository:

- **Backend API** — Go + Gin + GORM + PostgreSQL, Cobra CLI, golang-migrate, feature-oriented architecture (each feature owns handlers, DTOs, service, repository, models, validation, routes, tests).
- **Web app** — React + TypeScript + Vite + Tailwind CSS + shadcn/ui, feature-oriented architecture (each feature owns components, pages, hooks, API clients, types, schemas, state, tests).
- **Local dev** — full stack via Docker Compose with hot reload, health-checked startup order, and a DB admin tool.
- **Tooling** — Makefile task surface, lint/format for Go and TS, git hooks with conventional commits, security scanning, CI/CD workflows, production Dockerfiles.

This plan delivers scaffolding plus two working reference features (`auth`, `users`) so every architectural convention is demonstrated by real, tested code — not empty folders.

## Monorepo Decision

**Recommendation: single monorepo, `apps/` layout, Makefile as orchestrator.**

Rationale:

- One product, two tightly coupled apps sharing an API contract, one team → atomic cross-stack commits, single CI trigger surface, one place for docs/infra.
- Go modules and npm workspaces coexist cleanly; no shared JS packages are needed yet, so **no Nx/Turborepo/pnpm-workspace machinery** (YAGNI). A root Makefile + per-app toolchains is enough.
- CI uses path filters (`apps/api/**`, `apps/web/**`) so each app builds/tests only when it changes.
- Revisit only if a third consumer of shared TS types appears; then introduce `packages/` + pnpm workspaces.

## Proposed Directory Tree

```text
project-root/
├── apps/
│   ├── api/                                # Go backend (single Go module)
│   │   ├── cmd/
│   │   │   └── api/
│   │   │       └── main.go                 # thin entrypoint → internal/cli
│   │   ├── internal/
│   │   │   ├── app/                        # bootstrap: container wiring, lifecycle
│   │   │   │   ├── app.go
│   │   │   │   └── container.go            # manual DI container
│   │   │   ├── cli/                        # Cobra commands
│   │   │   │   ├── root.go
│   │   │   │   ├── serve.go                # start HTTP server
│   │   │   │   ├── migrate.go              # up / down / to / status
│   │   │   │   ├── seed.go                 # seed database
│   │   │   │   └── admin.go                # create admin account
│   │   │   ├── config/
│   │   │   │   ├── config.go               # env-based typed config
│   │   │   │   └── config_test.go
│   │   │   ├── database/
│   │   │   │   ├── postgres.go             # GORM + pgx pool setup
│   │   │   │   ├── migrate.go              # golang-migrate runner
│   │   │   │   └── tx.go                   # transaction manager
│   │   │   ├── server/
│   │   │   │   ├── server.go               # HTTP server + graceful shutdown
│   │   │   │   ├── router.go               # engine setup, feature route mounting
│   │   │   │   └── health.go               # /healthz, /readyz
│   │   │   ├── middleware/
│   │   │   │   ├── auth.go                 # JWT auth + role guard
│   │   │   │   ├── cors.go
│   │   │   │   ├── logger.go               # request logging (slog)
│   │   │   │   ├── recovery.go
│   │   │   │   └── requestid.go
│   │   │   ├── shared/                     # cross-feature infrastructure
│   │   │   │   ├── apperror/               # typed errors + HTTP mapping
│   │   │   │   ├── response/               # response envelope + pagination meta
│   │   │   │   ├── validation/             # validator setup + custom rules
│   │   │   │   ├── pagination/             # page/filter/sort params
│   │   │   │   └── logger/                 # slog construction
│   │   │   └── features/
│   │   │       ├── auth/
│   │   │       │   ├── handler.go
│   │   │       │   ├── dto.go
│   │   │       │   ├── service.go
│   │   │       │   ├── repository.go       # interface + GORM impl
│   │   │       │   ├── model.go            # refresh tokens etc.
│   │   │       │   ├── routes.go
│   │   │       │   ├── service_test.go     # unit
│   │   │       │   └── integration_test.go # against real Postgres
│   │   │       └── users/
│   │   │           ├── handler.go
│   │   │           ├── dto.go
│   │   │           ├── service.go
│   │   │           ├── repository.go
│   │   │           ├── model.go
│   │   │           ├── routes.go
│   │   │           ├── service_test.go
│   │   │           └── integration_test.go
│   │   ├── migrations/                     # golang-migrate SQL files
│   │   │   ├── 000001_create_users.up.sql
│   │   │   ├── 000001_create_users.down.sql
│   │   │   ├── 000002_create_refresh_tokens.up.sql
│   │   │   └── 000002_create_refresh_tokens.down.sql
│   │   ├── seeds/
│   │   │   └── seed.go                     # idempotent dev seed data
│   │   ├── docs/                           # generated swagger (swag output)
│   │   ├── testutil/                       # testcontainers helpers, fixtures
│   │   ├── .air.toml                       # hot reload config
│   │   ├── .golangci.yml
│   │   ├── Dockerfile                      # multi-stage production image
│   │   ├── Dockerfile.dev                  # air-based dev image
│   │   ├── go.mod
│   │   └── go.sum
│   └── web/                                # React frontend
│       ├── src/
│       │   ├── app/                        # bootstrap
│       │   │   ├── main.tsx                # entry: mount + providers
│       │   │   ├── app.tsx
│       │   │   ├── providers.tsx           # QueryClient, Router, Theme, Toaster
│       │   │   └── router.tsx              # route tree + protected routes
│       │   ├── layouts/
│       │   │   ├── root-layout.tsx
│       │   │   ├── auth-layout.tsx         # public (login/register)
│       │   │   └── dashboard-layout.tsx    # authenticated shell
│       │   ├── features/
│       │   │   ├── auth/
│       │   │   │   ├── components/
│       │   │   │   ├── pages/              # login-page.tsx, register-page.tsx
│       │   │   │   ├── hooks/              # use-login.ts, use-session.ts
│       │   │   │   ├── api/                # auth-api.ts
│       │   │   │   ├── schemas/            # zod schemas
│       │   │   │   ├── stores/             # auth-store.ts (zustand)
│       │   │   │   ├── types/
│       │   │   │   └── __tests__/
│       │   │   └── users/
│       │   │       ├── components/         # users-table.tsx, user-form.tsx
│       │   │       ├── pages/
│       │   │       ├── hooks/              # use-users.ts (query hooks)
│       │   │       ├── api/
│       │   │       ├── schemas/
│       │   │       ├── types/
│       │   │       └── __tests__/
│       │   ├── components/
│       │   │   ├── ui/                     # shadcn/ui generated components
│       │   │   └── shared/                 # app-owned: data-table, empty-state,
│       │   │                               #   error-boundary, page-header, spinner
│       │   ├── lib/
│       │   │   ├── api/                    # axios instance, interceptors, errors
│       │   │   ├── config/                 # env parsing (zod-validated)
│       │   │   └── utils/                  # cn(), formatters
│       │   ├── hooks/                      # shared hooks (use-debounce, …)
│       │   ├── assets/
│       │   ├── styles/
│       │   │   └── globals.css             # tailwind entry + theme tokens
│       │   └── test/
│       │       ├── setup.ts                # vitest setup (jest-dom, msw)
│       │       ├── msw/                    # API mock handlers
│       │       └── utils.tsx               # renderWithProviders
│       ├── e2e/                            # Playwright specs
│       ├── public/
│       ├── index.html
│       ├── vite.config.ts
│       ├── vitest.config.ts
│       ├── playwright.config.ts
│       ├── tsconfig.json
│       ├── eslint.config.js                # flat config
│       ├── .prettierrc
│       ├── components.json                 # shadcn config
│       ├── Dockerfile                      # multi-stage → nginx
│       ├── Dockerfile.dev                  # vite dev server
│       ├── nginx.conf
│       └── package.json
├── infrastructure/
│   ├── docker/
│   │   └── postgres/
│   │       └── init/                       # 00-create-db.sql (init scripts)
│   └── github/                             # deploy manifests placeholder (k8s/tf later)
├── scripts/
│   ├── setup.sh                            # one-shot dev environment bootstrap
│   ├── wait-for.sh
│   └── check-tools.sh
├── docs/
│   ├── architecture.md
│   ├── api-guidelines.md                   # error/response/pagination contracts
│   ├── frontend-guidelines.md
│   ├── local-development.md
│   └── deployment.md
├── .github/
│   ├── workflows/
│   │   ├── api-ci.yml
│   │   ├── web-ci.yml
│   │   └── security.yml
│   └── dependabot.yml
├── plans/
├── docker-compose.yml
├── docker-compose.prod.yml                 # production-shaped reference compose
├── Makefile
├── lefthook.yml                            # git hooks (polyglot)
├── commitlint.config.mjs
├── .editorconfig
├── .gitignore
├── .env.example
└── README.md
```

## Goals

| # | Goal | Priority |
|---|------|----------|
| 1 | Feature-oriented Go API skeleton with 2 working, tested reference features | P1 |
| 2 | Cobra CLI: serve, migrate up/down, seed, admin create | P1 |
| 3 | Feature-oriented React app with auth + users reference features | P1 |
| 4 | One-command local stack: `make dev` via Docker Compose with hot reload | P1 |
| 5 | Full tooling: lint, format, hooks, conventional commits, security scans, CI | P2 |
| 6 | Documentation: architecture, API contracts, local dev, deployment | P2 |

## Phases

| # | Phase | Status |
|---|-------|--------|
| 1 | [Phase 1: Repository Foundation](./phase-01-start.md) | Completed |
| 2 | [Phase 2: Backend Core Infrastructure](./phase-02-backend-core-infrastructure.md) | Completed |
| 3 | [Phase 3: Backend Features and Migrations](./phase-03-backend-features-and-migrations.md) | Completed |
| 4 | [Phase 4: Backend Testing and API Docs](./phase-04-backend-testing-and-api-docs.md) | Completed |
| 5 | [Phase 5: Frontend Foundation](./phase-05-frontend-foundation.md) | Completed |
| 6 | [Phase 6: Frontend Features and Testing](./phase-06-frontend-features-and-testing.md) | Pending |
| 7 | [Phase 7: Docker Compose Local Dev](./phase-07-docker-compose-local-dev.md) | Pending |
| 8 | [Phase 8: CI/CD, Tooling and Deployment](./phase-08-ci-cd-tooling-and-deployment.md) | Pending |

**Dependencies:** 2→1, 3→2, 4→3, 5→1, 6→5 (+3 for real API in e2e), 7→(3,5), 8→(4,6,7). Phases 2–4 (backend) and 5–6 (frontend) can run as two parallel tracks after Phase 1.

## Key Technology Decisions

| Concern | Choice | Why |
|---|---|---|
| DI (Go) | Manual constructor injection via `app.Container` | Explicit, debuggable; wire/fx are YAGNI at this size |
| Config (Go) | `caarlos0/env` + `joho/godotenv` (dev only) | Typed structs, env-first, 12-factor; simpler than Viper |
| Logging (Go) | stdlib `log/slog`, JSON in prod | No dependency, structured, leveled |
| Validation (Go) | `go-playground/validator` via Gin binding + custom messages | Standard for Gin |
| Auth | JWT access (15 min) + rotating refresh token (httpOnly cookie), bcrypt, role field | Stateless API auth; simple RBAC via middleware |
| OpenAPI | `swaggo/swag` annotations → `/swagger` in non-prod | Lowest-friction docs for Gin |
| Integration tests | `testcontainers-go` (Postgres) | Real DB, no shared state |
| Routing (web) | React Router v7 (library mode) | Standard, data APIs, no framework lock-in |
| Server state | TanStack Query v5 | Cache, retries, invalidation |
| Client state | Zustand (auth/session, UI state only) | Minimal; server state stays in Query |
| Forms | react-hook-form + zod + `@hookform/resolvers` | Pairs with shadcn/ui form primitives |
| HTTP client | axios instance + interceptors (auth header, 401 refresh, error normalization) | Familiar interceptor model |
| E2E | Playwright | Modern default |
| Git hooks | lefthook + commitlint | Polyglot repo, fast, conventional commits |
| Hot reload (Go) | `air` in dev container | De-facto standard |

## Success Criteria

- [ ] `make setup && make dev` on a clean machine brings up Postgres, API, and web with migrations applied and seeds loaded
- [ ] `curl localhost:8080/healthz` and `/readyz` return 200; web app loads at `localhost:5173`, login works against seeded admin
- [ ] `make test` passes: Go unit + integration tests, Vitest suites; `make lint` passes both stacks
- [ ] All Cobra commands work: `serve`, `migrate up|down|status`, `seed`, `admin create`
- [ ] Each reference feature demonstrates the full feature-module contract (docs list the contract; code matches)
- [ ] CI workflows green on a PR touching both apps; security scan jobs run
- [ ] Docs cover architecture, API standards, local dev, deployment

## Open Questions

- None blocking. Deployment target (Section: Phase 8) is kept platform-agnostic (container registry + compose/k8s-ready images) until a target is chosen.

<!-- slug: fullstack-project-provisioning -->
