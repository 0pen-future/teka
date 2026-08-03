# Architecture

Owns: system overview, monorepo rationale, cross-app boundaries, dependency-injection approach.

## Monorepo

Single repository, two applications under `apps/`, orchestrated by the root
`Makefile`. No workspace tooling (Nx/Turbo) — the apps share no code packages;
they share only the HTTP API contract. CI builds each app only when its path
changes. Revisit if a shared TypeScript package becomes necessary.

## Applications

- **`apps/api`** — Go, feature-oriented. Each feature under
  `internal/features/<name>/` owns its handlers, DTOs, service, repository
  (interface + GORM implementation), models, validation, routes, and tests.
  Shared infrastructure (config, database, server, middleware, logging, errors,
  responses, pagination) lives outside features and never contains business logic.
- **`apps/web`** — React, feature-oriented. Each feature under
  `src/features/<name>/` owns its components, pages, hooks, API clients, zod
  schemas, types, state, and tests. `src/app/` owns bootstrap and routing;
  `src/lib/` owns API/config/utility infrastructure.

_Filled in as phases land: DI container details, request lifecycle diagram._
