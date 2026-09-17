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

### Hard-boundary libraries

Two packages are written as if they lived in their own repositories, so they
can be extracted later without a rewrite:

- **`apps/api/pkg/kanban`** — task-board domain core (entities, policy, unit
  of work and repository ports, service). It imports only the standard library
  and `github.com/google/uuid`; a test in the package runs `go list -deps` and
  fails on anything else. A task's place in its column is a float position:
  a move lands at the top or at the midpoint after a given task, and the core
  renormalizes the column inside the same transaction (under a per-column
  lock) when the gap gets too small — see the package README's "Position
  strategy". The `tasks` feature under `internal/features/` supplies the GORM
  repositories, the tenant/actor mapping and the HTTP layer, and owns the
  description's trust boundary: a description is a sanitized HTML subset,
  cleaned by bluemonday on write (`description.go`) and again by DOMPurify on
  the web before it is rendered, so neither side trusts the other's output.
- **`apps/web/src/lib/kanban`** — headless board state (pure reducer,
  selectors, hook with prop getters, keyboard navigation, pure drop-position
  helpers). It has no drag layer of its own: pointer drag-and-drop is an
  opt-in adapter in the `tasks` feature (`hooks/use-board-dnd.ts`, dnd-kit)
  that maps drop events onto the lib's position helpers. The lib may import
  only `react`; an ESLint `no-restricted-imports` override in
  [eslint.config.js](../apps/web/eslint.config.js) enforces that. The `tasks`
  feature adapts TanStack Query data into the lib's data-source contract and
  owns every design-system component.

Neither package may import from a feature, shared infrastructure, or the other
app. Each has a README describing its ports and the extraction procedure.

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
       → gin engine (trusted proxies from API_HTTP_TRUSTED_PROXIES; default none)
       → request-id → logger → recovery → CORS → body-limit (1 MiB default;
                                                  roster import exempt, own 2 MiB cap)
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
