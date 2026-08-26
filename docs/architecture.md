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

## Dependency injection (backend)

Manual constructor injection, no framework. `internal/app.Container` holds the
app-wide dependencies (`Cfg`, `Log`, `DB`); `app.RunServer` wires
config → container → router → HTTP server. Feature wiring
(repository → service → handler) happens in `server.registerFeatures`, keeping
features decoupled from bootstrap. Adopt `google/wire` only if wiring exceeds
~5 features.

## Request lifecycle (backend)

```
client → http.Server (timeouts: read-header 5s / read 10s / write 30s / idle 120s)
       → gin engine (no trusted proxies)
       → request-id → logger → recovery → CORS
       → /healthz | /readyz | /api/v1/<feature routes>
                                └─ request-events (publishes one bus event per
                                   mutating request — see docs/event-bus.md)
```

Graceful shutdown: SIGINT/SIGTERM cancels the serve context; the server drains
in-flight requests (10s budget), then the event bus and audit batcher drain,
before the DB pool closes. A second signal force-quits.

## In-process events (backend)

Cross-feature side effects (today: the audit trail) ride an in-process event
bus with non-blocking publish and at-most-once delivery. Contract, event
catalog, capture pipeline, and extension conventions:
[`docs/event-bus.md`](./event-bus.md).
