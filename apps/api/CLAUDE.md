# apps/api — Go API (Gin + pgx)

Go module `teka/apps/api`. Entrypoint `cmd/api/main.go`; DI wiring in
`internal/app/container.go`.

## Structure

- `internal/features/<domain>/` — feature modules (attendance, audit, auth,
  billing, …). Each holds `dto.go`, `handler.go`, `service.go`,
  `repository.go`, `routes.go`, `model.go` plus unit and `*_integration_test.go`
  files in the same package. New endpoints follow this layout — do not create a
  separate handlers/services tree.
- `internal/shared/` — cross-feature utilities (apperror, authctx, events, id,
  logger, pagination, response, secrets, token, validation). Check here before
  writing a new helper.
- `internal/testutil/` — test helpers; reuse for integration tests.

## Constraints

- MUST NOT hand-edit `docs/` (`docs.go`, `swagger.json`, `swagger.yaml`): they
  are generated from swag annotations via `make api-docs` (run from repo root).
- MUST NOT modify an existing migration in `migrations/`. Add the next numbered
  `NNNNNN_slug.up.sql` / `.down.sql` pair; migrations are embedded via
  `embed.go` and covered by `migrations_test.go`.
- Dev seed data lives in `seeds/seed.go` (`make seed`).

## Verification

- `make test-api-unit` — fast unit + HTTP tests, no Docker.
- `make test-api` — full unit + integration with coverage floor, needs Docker.

## Pointers

- Conventions (response envelope, error handling, tenancy, pagination,
  validation, auth, testing): `docs/api-guidelines.md` at repo root.
- In-process event bus (used by e.g. audit subscriber): `docs/event-bus.md`.
