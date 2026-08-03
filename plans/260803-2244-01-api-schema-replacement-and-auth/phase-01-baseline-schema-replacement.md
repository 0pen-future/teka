---
phase: 1
title: "Baseline Schema Replacement"
status: pending
priority: P1
effort: "4h"
dependencies: []
---

# Phase 1: Baseline Schema Replacement

## Overview

Delete the scaffold schema and install `docs/schema_design.sql` as migration
`000001_baseline_schema`. Recreate `refresh_tokens` as `000002` pointing at
`user_accounts` instead of the deleted `users` table. After this phase the
database has every V1 table; the Go code that uses most of them arrives in
later plans.

This phase deliberately breaks compilation of `internal/features/users` and
`seeds/seed.go` — both reference the dead `users` table. The phase ends with
the seed rewritten and the users feature temporarily unmounted so
`go build ./...` succeeds. Phases 2 and 3 restore full functionality.

## Requirements

- Migration `000001_baseline_schema.up.sql` contains `docs/schema_design.sql`
  verbatim (D1). No reformatting, no comment stripping, no reordering — a
  reviewer must be able to `diff` the two files and get no output.
- `000001_baseline_schema.down.sql` drops every object created by the up
  migration, in reverse dependency order.
- `000002_refresh_tokens.up.sql` recreates `refresh_tokens` with
  `user_id UUID NOT NULL REFERENCES user_accounts(id) ON DELETE CASCADE`.
- Old migrations `000001_create_users.*` and `000002_create_refresh_tokens.*`
  are deleted, not renamed or commented out.
- `seeds/seed.go` creates teacher accounts (`user_accounts` + `teachers`), not
  generic users.
- A shared UUIDv7 generator exists (D3), because the schema declares bare
  `UUID PRIMARY KEY` with no DB-side default.
- `go build ./...` succeeds at the end of the phase.

## Architecture

**Migration set after this phase**

```
apps/api/migrations/
  000001_baseline_schema.up.sql     <- docs/schema_design.sql verbatim
  000001_baseline_schema.down.sql   <- DROP everything, reverse order
  000002_refresh_tokens.up.sql      <- auth infrastructure, FK -> user_accounts
  000002_refresh_tokens.down.sql
  embed.go                          <- unchanged (//go:embed *.sql)
```

`embed.go` needs no edit: it embeds `*.sql`, and golang-migrate's `iofs` source
driver discovers versions from filenames.

**Why `refresh_tokens` is separate.** `docs/schema_design.sql` is the product
data contract and must stay diff-able against the design doc. Refresh tokens
are an implementation detail of the chosen auth mechanism — a future switch to
opaque sessions or OTP-only login would delete that table without touching the
domain schema. Keeping it in its own migration makes that boundary visible.

**Down-migration ordering.** Postgres refuses to drop a table another table
references. The down script drops in this order: views
(`v_unbilled_attendance`, `v_contact_balance`) → `notifications` → `statements`
→ `payment_allocations` → `payments` → `invoice_adjustments` → `invoice_lines`
→ `invoices` → `billing_periods` → `attendance_records` → `class_sessions` →
`enrollments` → `class_schedules` → `classes` → `students` → `contacts` →
`teachers` → `user_accounts`. Indexes and constraints drop with their tables.
The `pgcrypto` extension is **not** dropped (it may pre-exist and is shared).

**Seed data shape.** One teacher account is enough for phases 2–3; plan 02 adds
roster seeds. Seed teachers get a `+84` phone, role `'teachers'`, a bcrypt
hash, and a matching `teachers` row inserted in the same transaction. The
idempotency key changes from email to phone.

## Related Code Files

**Create**

- `apps/api/migrations/000001_baseline_schema.up.sql`
- `apps/api/migrations/000001_baseline_schema.down.sql`
- `apps/api/migrations/000002_refresh_tokens.up.sql`
- `apps/api/migrations/000002_refresh_tokens.down.sql`
- `apps/api/internal/shared/id/id.go` — UUIDv7 generator (D3)
- `apps/api/internal/shared/id/id_test.go`
- `apps/api/migrations/migrations_test.go` — up/down/up round-trip against a
  testcontainer database

**Delete**

- `apps/api/migrations/000001_create_users.up.sql`
- `apps/api/migrations/000001_create_users.down.sql`
- `apps/api/migrations/000002_create_refresh_tokens.up.sql`
- `apps/api/migrations/000002_create_refresh_tokens.down.sql`

**Modify**

- `apps/api/seeds/seed.go` — seed teacher accounts instead of `users`
  (currently keyed on email at `seeds/seed.go:28-33`)
- `apps/api/internal/server/router.go:63-73` — `registerFeatures` temporarily
  stops mounting `users.RegisterRoutes` (line 69); phase 3 mounts the
  teacher-profile routes in its place
- `apps/api/internal/testutil/fixtures.go` — `User()` helper becomes
  `Teacher()`; `WithEmail` becomes `WithPhone`
- `apps/api/internal/features/users/*` — left on disk but unmounted until phase
  3 rewrites it. Do **not** delete the directory: phase 3 reuses its
  handler/service/repository structure as the template.

## Implementation Steps

1. Reset the local database first so you are not fighting a dirty migration
   state: `make dev-nuke` (destroys the local volume), then `make dev`.
2. `git rm` the four old migration files listed under **Delete**.
3. Copy `docs/schema_design.sql` to
   `apps/api/migrations/000001_baseline_schema.up.sql` with `cp`. Do not open
   and retype it. Verify with
   `diff docs/schema_design.sql apps/api/migrations/000001_baseline_schema.up.sql`
   — expect no output.
4. Write `000001_baseline_schema.down.sql` as `DROP VIEW IF EXISTS ...`
   followed by `DROP TABLE IF EXISTS ... CASCADE;` in the reverse order listed
   under **Architecture**. Do not drop `pgcrypto`.
5. Write `000002_refresh_tokens.up.sql`. Start from the deleted
   `000002_create_refresh_tokens.up.sql` body (id, user_id, token_hash,
   family_id, expires_at, revoked_at, created_at, plus the three indexes) and
   change only the FK target to `user_accounts (id) ON DELETE CASCADE`. Keep
   `id uuid PRIMARY KEY DEFAULT gen_random_uuid()` — refresh-token ids are not
   domain ids, so D3 does not apply to them.
6. Write `000002_refresh_tokens.down.sql`: `DROP TABLE IF EXISTS refresh_tokens;`
7. Create `internal/shared/id/id.go` exposing `func New() uuid.UUID` returning
   `uuid.Must(uuid.NewV7())` and `func NewString() string`.
   `github.com/google/uuid` is already a dependency — confirm `apps/api/go.mod`
   pins v1.6.0 or later (that is where `NewV7` landed) and bump if not. State
   in the package comment that the domain schema declares bare
   `UUID PRIMARY KEY` with no default, so ids must be supplied by the caller.
8. Rewrite `seeds/seed.go`: replace the `seedUser{Email,Password,Name,Role}`
   struct and its `seedUsers` slice with `seedTeacher{Phone, Password,
   FullName}`. For each entry, look up `user_accounts` by phone; if absent,
   open a transaction, insert `user_accounts` with `id: id.New()`,
   `role: "teachers"`, `status: "active"`, and a bcrypt hash at the existing
   cost 12, then insert `teachers` with the same id, `full_name`, and the
   default timezone. Keep the existing idempotency contract ("existing records
   are never modified") and the structured log lines, keyed on phone. Use raw
   `db.Exec` or a package-local struct rather than importing the unmounted
   `users` package.
9. In `internal/server/router.go`, remove the `users.RegisterRoutes(...)` call
   and the now-unused import so the build is green. Leave the `auth` wiring
   alone — phase 2 rewrites it.
10. Write `migrations_test.go` with the `integration` build tag, matching the
    convention in `apps/api/internal/features/auth/integration_test.go`. Start
    a Postgres container via `testutil.StartPostgres`, run migrate up, assert
    `information_schema.tables` contains all 16 domain tables plus
    `refresh_tokens` and `schema_migrations`, migrate down to version 0, assert
    only `schema_migrations` remains, then migrate up again.
11. Run `make migrate-up`, `make migrate-status`, then `make seed` twice to
    prove idempotency.
12. Run `go build ./...` and `make test-api`.

## Success Criteria

- [x] `diff docs/schema_design.sql apps/api/migrations/000001_baseline_schema.up.sql` produces no output
- [x] `make migrate-up` on an empty database exits 0 and creates 16 domain tables, `refresh_tokens`, and 2 views
- [x] Migrating down to version 0 leaves only `schema_migrations`
- [x] The up → down → up round trip passes in `migrations_test.go`
- [x] `make seed` inserts a teacher and is a no-op on the second run
- [x] `refresh_tokens.user_id` FK targets `user_accounts(id)` with `ON DELETE CASCADE`
- [x] `internal/shared/id` returns version-7 UUIDs (asserted in `id_test.go`)
- [x] `go build ./...` succeeds
- [x] `grep -rn "create_users" apps/api` returns nothing

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Down migration fails midway, leaving a half-dropped schema that blocks re-up | Medium | Medium | `DROP ... IF EXISTS ... CASCADE` throughout so partial state stays recoverable; the round-trip test catches ordering errors before merge |
| `uuid.NewV7` unavailable in the pinned `github.com/google/uuid` version | Low | Low | Checked at step 7; `NewV7` landed in v1.6.0. Prefer the version bump over hand-rolling v7 bytes |
| Someone later edits the baseline `.up.sql` instead of adding a new migration | Medium | High | The verbatim-copy invariant is restated in the migration's own header; the diff check is re-runnable at any time |
| Local databases with the old schema silently keep working, hiding breakage | High | Low | `make dev-nuke` is step 1, not a footnote |
| Test suite still asserts against the `users` table and fails wholesale | High | Low | Expected and intentional; phases 2–3 rewrite those tests. Do not weaken assertions to make them pass in the interim |
