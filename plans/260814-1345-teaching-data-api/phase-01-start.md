---
phase: 1
title: "DB migration — teaching tables"
status: completed
priority: P1
effort: "4h"
dependencies: []
---

# Phase 1: DB migration — teaching tables

## Overview

Create migration `000009_teaching` adding `class_curricula`, `lesson_plans`, `session_notes`, `session_marks` with center-scoped integrity per the `000007_centers` pattern.

## Requirements

- Functional: four tables exactly as specified in plan.md § Database design; up and down migrations.
- Non-functional: FK guard `(teacher_id, center_id) → center_members` on every table; composite FKs to parents' `(id, center_id)` uniques; partial index for pending plans; no soft-delete columns (deliberate — non-financial data).

## Architecture

- Parents already expose `UNIQUE (id, center_id)` from `000007` — verified: `uq_classes_cid`, `uq_class_sessions_cid`, `uq_students_cid` (`000007_centers.up.sql:163-167`); reference them, do not re-create.
- `ON DELETE CASCADE` from parents: deleting a class/session/student (hard-delete path) drops its teaching rows; membership-guard FK uses the same `NO ACTION`/`CASCADE` semantics as sibling business tables in `000007` — copy that exact shape.
- `session_notes.session_id` is the PK (1:1 with session). `lesson_plans` unique on `(class_id, lesson_index)`; `session_marks` unique on `(session_id, student_id)`.
- JSONB columns (`lessons`, `activities`) hold ordered string arrays; no GIN index (no containment queries).
- CHECKs: `lesson_plans.status IN ('draft','pending','approved','redo')`; `lesson_plans.lesson_index >= 0`; `session_marks.score BETWEEN 0 AND 10` (NUMERIC(4,1), NULLable).

## Related Code Files

- Create: `apps/api/migrations/000009_teaching.up.sql`
- Create: `apps/api/migrations/000009_teaching.down.sql`
- Modify: `apps/api/migrations/migrations_test.go` (extend coverage if the test enumerates migrations explicitly — read it first; if it's generic up/down it may need no change)
- Read (evidence): `apps/api/migrations/000007_centers.up.sql`, `000001_baseline_schema.up.sql`

## Implementation Steps

1. Read `000007_centers.up.sql` fully; note parent unique-constraint names and the FK-guard shape used by the 16 business tables.
2. Write `up.sql`: 4 tables in dependency order, indexes (`idx_lesson_plans_pending` partial on `status='pending'` keyed by `center_id`; `idx_session_marks_session`), Vietnamese comments only where a constraint's product reason is non-obvious (mirror repo style, e.g. why no soft delete).
3. Write `down.sql`: drop in reverse order.
4. Run migration test suite; verify up → down → up cycles clean on a scratch database.

## Success Criteria

- [x] `migrate up` and `migrate down` both clean; re-up idempotent from empty.
- [x] FK guard rejects a row whose `(teacher_id, center_id)` has no `center_members` row (covered by a migration/integration test).
- [x] `go test ./migrations/...` (or the repo's migration test entry point) green.

## Risk Assessment

- **Constraint-name drift** — the plan assumes `(id, center_id)` uniques exist on `classes`/`class_sessions`/`students`; step 1 verifies names before use. If absent on a needed parent, add them in `000009` following `000007`'s comment style.
- **NUMERIC precision** — UI uses 0–10 step 0.1; NUMERIC(4,1) is exact, no float drift.

## Completion notes

- `idx_session_marks_session` was deliberately omitted: `UNIQUE (session_id, student_id)` already backs an index leading with `session_id`, so a dedicated index would be redundant.
- `migrations_test.go`'s `TestCenterTenancyBackfill` now targets its rollback point via `m.Migrate(6)` instead of `MigrateDown(m, 2)`, so later migrations (including `000009`) don't shift the intended rollback target.
- Migration up/down round-trip verified both in the unit migration test and via `go test -tags integration ./...`.
