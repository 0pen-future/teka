---
title: "01 API Schema Replacement and Auth"
description: "Replace the scaffold schema with the Sổ Lớp V1 baseline from docs/schema_design.sql and rewrite auth as phone + password over user_accounts/teachers."
status: completed
priority: P1
effort: "14h"
tags: [api, go, migrations, auth, schema, multi-tenancy]
created: 2026-08-03
blocks: [260803-2244-02-api-roster-management]
---

# 01 API Schema Replacement and Auth

## Overview

The API scaffold ships a generic `users` table (email + password, roles
`admin`/`user`) that has nothing to do with the product. `docs/schema_design.sql`
is the real V1 schema: `user_accounts` (login identity) split from `teachers`
(business profile), plus 15 domain tables scoped by `teacher_id`.

This plan throws away the scaffold schema, installs `docs/schema_design.sql` as
the migration baseline, and re-points authentication at `user_accounts`: login
by **phone + password**, register creates `user_accounts` + `teachers` in one
transaction. The existing JWT access token + rotating refresh token machinery
(families, reuse-revocation) is kept as-is and only re-pointed.

Every downstream plan (roster, sessions, billing, payments, statements) depends
on this one. Nothing else can start until the baseline lands.

## Scope

**In scope**

- Fresh migration baseline: `000001_baseline_schema` = `docs/schema_design.sql`
  verbatim; `000002_refresh_tokens` recreated against `user_accounts`.
- Auth feature rewrite: register / login / refresh / logout / me on phone.
- `users` feature repurposed into the teacher profile feature (`GET/PUT /me`).
- Shared plumbing every later plan consumes: UUIDv7 generator, teacher-scoping
  helper in `authctx`, soft-delete read discipline at the repository layer.
- Seeds, fixtures, and swagger regenerated against the new schema.

**Non-goals**

- Any domain feature table beyond identity — contacts/students/classes are
  plan 02, sessions are plan 03. Their **migrations exist** after this plan
  (the baseline creates all tables at once); their **Go code does not**.
- Parent and student login. `role` supports `'parent'` and `'students'` per
  schema note (n), but V1 creates `'teachers'` only.
- OTP login. `user_accounts.password_hash` is nullable for that future, but V1
  always writes a bcrypt hash.
- Postgres Row Level Security. Schema note (m) recommends it; this plan
  enforces tenancy in the repository layer and records RLS as a pre-launch
  hardening item.
- Data migration from the old `users` table. No production data exists.

## Phases

| # | Phase | Effort | Depends on | Status |
|---|-------|--------|------------|--------|
| 1 | [Baseline schema replacement](./phase-01-baseline-schema-replacement.md) | 4h | — | Pending |
| 2 | [Phone-based auth rewrite](./phase-02-phone-auth-rewrite.md) | 6h | 1 | Pending |
| 3 | [Teacher profile and scoping utilities](./phase-03-teacher-profile-and-scoping.md) | 4h | 2 | Pending |

## Key Decisions

**D1 — Replace, do not migrate.** Migrations `000001_create_users` and
`000002_create_refresh_tokens` are deleted and replaced by a fresh
`000001_baseline_schema` holding `docs/schema_design.sql` verbatim, plus
`000002_refresh_tokens` recreating the token table with
`user_id → user_accounts(id) ON DELETE CASCADE`. `refresh_tokens` deliberately
lives outside `docs/schema_design.sql`: it is auth infrastructure, not domain
data, and the schema file stays the untouched product contract. No production
data exists; developers reset local databases (`make dev-nuke` then
`make migrate-up`).

**D2 — Phone is the login identifier.** `user_accounts.phone VARCHAR(20)`
(E.164) with `uq_users_phone` partial-unique on `deleted_at IS NULL`. Register
inserts `user_accounts` (role `'teachers'`) and `teachers` (`id` = the account
id) inside one transaction. The JWT `sub` claim carries that id, which is
simultaneously the account id and the `teacher_id` used by every downstream
query — that is the whole point of `teachers.id REFERENCES user_accounts(id)`.
Access/refresh token rotation with family revocation is preserved unchanged.

**D3 — UUIDv7 generated in Go.** `docs/schema_design.sql` declares
`id UUID PRIMARY KEY` with **no** `DEFAULT gen_random_uuid()`. Every insert
must supply an id. A shared `internal/shared/id` package generates UUIDv7 so
primary keys sort by creation time and index locality stays good.

**D4 — Tenancy at the repository layer.** Every query in every feature filters
by `teacher_id`, and every read on a soft-delete table adds
`deleted_at IS NULL`. Composite FKs (`(id, teacher_id)` uniques referenced by
child tables) make cross-teacher stitching impossible at the DB level, but they
do not stop a *read* from returning another teacher's row — the repository
must. RLS (schema note (m)) is the belt-and-braces version and is deferred.

**D5 — Money is `BIGINT` đồng, states are `VARCHAR` + `CHECK`.** No float
anywhere. Every `CHECK (x IN (...))` in the schema gets a matching Go constant
block so the compiler catches typos the DB would only catch at runtime.

**D6 — GORM mirrors, never migrates.** Models are hand-written to match the
schema exactly. `AutoMigrate` is never called; golang-migrate is the only
schema authority. Notably this means GORM struct tags must **not** declare
`default:gen_random_uuid()` any more (see D3).

## Acceptance Criteria

- [x] `make migrate-up` on an empty database creates every table, index, view,
      and constraint in `docs/schema_design.sql`, plus `refresh_tokens`.
- [x] `make migrate-down` from a fully migrated database returns it to empty.
- [x] A teacher registers with phone + password + full name; one
      `user_accounts` row (`role = 'teachers'`) and one `teachers` row with the
      same `id` exist afterwards, or neither does if any step fails.
- [x] Login with the registered phone returns an access token whose `sub` is
      the teacher id; login with a wrong phone and a wrong password are
      indistinguishable in body, status, and latency.
- [x] Refresh rotates the token; presenting an already-rotated token revokes the
      whole family (behaviour preserved from the scaffold).
- [x] `GET /api/v1/me` returns the authenticated teacher's profile;
      `PUT /api/v1/me` updates `full_name` and `timezone` only.
- [x] A disabled account (`user_accounts.status = 'disabled'`) or a soft-deleted
      account cannot log in or refresh.
- [x] `make test-api` passes; no test references the old `users` table.
- [x] `make api-docs` regenerates the OpenAPI spec with no stale email-based
      auth operations.

## Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Baseline SQL drifts from `docs/schema_design.sql` after hand-editing | Medium | High — every later plan assumes the doc is truth | Copy the file byte-for-byte; add a test that runs the migration and asserts table/index counts |
| `down` migration leaves orphans (views, extension) and blocks re-up | Medium | Medium — breaks local dev loop | Down drops in reverse dependency order, views first; CI runs up → down → up |
| Developers with an existing local DB hit a dirty migration state | High | Low | Document `make dev-nuke` in the phase steps; `migrate status` shows the version |
| Losing refresh-token reuse-revocation during the rewrite | Low | High — silent security regression | Keep `service.go` rotation logic structurally identical; existing `service_test.go` cases are ported, not rewritten |

## Open Questions

1. Phone normalisation: store E.164 (`+84...`) or the local form (`0...`)?
   Recommendation is to normalise to E.164 on write and accept both on login,
   but this affects the frontend form and the Zalo send path in plan 06.
2. Is a registration OTP required before an account can be used? V1 assumes no;
   `user_accounts.status` already supports gating if legal review (PRD Q2)
   demands it.
3. Retention/anonymisation job scheduling (schema note (q)) is unowned. Plan 02
   builds the per-student anonymise action; the periodic job is not scheduled
   in any plan yet.

<!-- slug: 01-api-schema-replacement-and-auth -->
