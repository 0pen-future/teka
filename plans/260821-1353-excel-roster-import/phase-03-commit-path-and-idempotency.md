---
phase: 3
title: "Commit Path and Idempotency"
status: completed
priority: P1
effort: "6h"
dependencies: [2]
---

# Phase 3: Commit Path and Idempotency

## Overview

`dry_run=false` runs the Phase 2 resolution and, if it is clean, writes every
row inside one transaction through the four features' **existing** `Create`
methods, anchored by a synthetic scope. All-or-nothing. Re-running the same
file against an unmodified database creates nothing.

## Key Insights

- **No refactor is needed to anchor a row.** No `Create` reads `sc.IsOwner`
  (`grep -n "IsOwner"` across the four service files → zero matches); each
  derives its anchor from the `authctx.Scope` argument alone. So
  `Create(ctx, authctx.Scope{TeacherID: target, CenterID: sc.CenterID, IsOwner: false}, req)`
  already stamps `target` and still runs every reference check without owner
  rights. `docs/api-guidelines.md:96-97` already sanctions this.
- **Nested `WithinTx` joins the ambient transaction** (`database/tx.go:24-26`),
  and every repository resolves its handle through
  `database.FromContext(ctx, r.db)` — verified: `grep "r\.db"` in the four
  repositories, excluding `FromContext`, returns nothing. So the imports
  service opens **one** transaction and calls four feature services inside it;
  `classes.Create` internally calling `s.tx.WithinTx` is harmless.
- **That same fact forbids insert-and-translate.** Joining rather than nesting
  means there is **no savepoint** (`grep -rn "SAVEPOINT"` → zero hits). A
  `23505` puts Postgres in the aborted state and every later statement returns
  `25P02`. Catching `ErrDuplicatePhone` and continuing cannot work here even
  though it is the right pattern in a single-statement handler
  (`contacts/service.go:33-34`). **All five keys are pre-checked; any `23505`
  reaching the import is a hard failure and a full rollback.**
- **Lookups belong to the feature that owns the table**, take `authctx.Scope`,
  and go through that feature's `scoped()` helper — the mandated contract
  (`docs/api-guidelines.md:80-93`). One already exists: `enrollments.List` with
  `ListFilter{StudentID, ClassID}` and `Active: nil` matches any enrollment,
  open or ended, which is exactly rule 6. Four are new. Do **not** reuse
  `contacts.List`/`students.List` `Query` — they are `ILIKE '%…%'` display
  filters, not exact-match keys.
- The FK guard `(teacher_id, center_id) → center_members` blocks a
  **cross-center** anchor only; it targets the table's PRIMARY KEY and a
  removed member's row survives with `left_at` set
  (`migrations/000007_centers.up.sql:66-72`). The live-membership filter is
  `ListMembers`' `ua.status = active`. Both cases get a test.

## Requirements

- Functional: `dry_run=false` → `200` with `Report{Committed:true}` and
  per-entity created/reused counts; `422` with the same `details.errors` shape
  when the file is invalid, having written nothing; `409` when another import
  holds the center lock.
- Non-functional: one transaction; a re-run against an unmodified database
  writes zero rows; the commit completes inside the server's `WriteTimeout`.

## Architecture

Four new lookups, each on the owning feature, each scope-typed:

```go
// classes/service.go
// FindActiveByName resolves a class by its exact name within the scope's
// teacher and center. status='active' is part of the key: an archived class
// has deleted_at IS NULL and would otherwise be silently reused by an import.
func (s *Service) FindActiveByName(ctx context.Context, sc authctx.Scope, name string) (*Class, bool, error)

// ScheduleExists reports whether the class already carries this exact weekly
// slot. effective_from is part of the key, so callers must pass the same value
// they will write — see the AddSchedule note below.
func (s *Service) ScheduleExists(ctx context.Context, sc authctx.Scope, classID uuid.UUID,
    weekday int16, startTime TimeOfDay, effectiveFrom time.Time) (bool, error)

// contacts/service.go
// FindIDByPhone resolves a contact by the exact E.164 phone within the scope's
// teacher and center — the same shape as uq_contacts_phone(teacher_id, phone).
func (s *Service) FindIDByPhone(ctx context.Context, sc authctx.Scope, phone string) (uuid.UUID, bool, error)

// students/service.go
// FindIDByName resolves a student by contact, exact name and note. note is a
// *string because display_note is NULL when unset (notePtr, dto.go:56-61) and
// `display_note = ''` never matches NULL; the predicate is
// `display_note IS NOT DISTINCT FROM ?`.
func (s *Service) FindIDByName(ctx context.Context, sc authctx.Scope,
    contactID uuid.UUID, fullName string, note *string) (uuid.UUID, bool, error)
```

Every one runs through its feature's existing `scoped()` helper, so the anchor
scope (`IsOwner: false`) narrows to that teacher automatically —
`classes/repository.go:59-65` is the reference shape.

Consumer-declared interfaces in `imports/service.go` target these plus the
existing `Create` methods. Nothing imports another feature's repository.

Commit flow:

```go
func (s *Service) Import(ctx context.Context, sc authctx.Scope, b []byte, dryRun bool) (*Report, error) {
    if !sc.IsOwner { return nil, apperror.Forbidden("chỉ chủ trung tâm được import") }
    plan, rowErrs, err := s.resolve(ctx, sc, b)
    if err != nil { return nil, err }
    if len(rowErrs) > 0 { return nil, rowErrorsAppErr(rowErrs) }

    var rep Report
    err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
        got, err := s.locker.TryLockCenter(ctx, sc.CenterID)   // pg_try_advisory_xact_lock
        if err != nil { return err }
        if !got { return apperror.Conflict("một lượt import khác đang chạy") }
        if err := s.setStatementTimeout(ctx); err != nil { return err }  // SET LOCAL statement_timeout
        return s.apply(ctx, sc, plan, dryRun, &rep)
    })
    if err != nil { return nil, err }
    rep.Committed = !dryRun
    return &rep, nil
}
```

`dry_run` runs the **same** `apply` with writes suppressed, so the dry run's
created/reused split is the one the commit will produce. A dry run that always
reported "tạo mới" would train the operator to ignore the only number that
tells them a re-import was a no-op.

`pg_try_advisory_xact_lock(hashtext($1::text))` — note the cast: `hashtext`
takes `text` and `center_id` is `uuid`. `try`, not the blocking form: the
blocking variant waits forever, parking a pooled connection, and with
`DB_MAX_OPEN_CONNS` defaulting to 25 (`config.go:47`) a handful of retries
starves every tenant. The lock releases on commit or rollback — no unlock path
to forget. Ordinary create routes take no lock and are **not** serialised
against the import; accepted, since the import is a one-off onboarding action.

Write order and anchoring, per resolved class group:

1. **Class** — `anchor := authctx.Scope{TeacherID: group.TeacherID, CenterID: sc.CenterID, IsOwner: false}`.
   `FindActiveByName` hit → compare `start_date`, `unit_price` and `end_date`
   against the file; **any difference is a row error** (`CLASS_EXISTS_MISMATCH`,
   user decision 7) — the file and the database disagreeing about one class is
   ambiguous, and silently keeping the DB price would invoice families at a
   rate the operator did not type. Equal → reuse, `Reused++`.
   Miss → `classesSvc.Create(ctx, anchor, CreateClassRequest{…, Schedules: every row of the group})`.
   For a **reused** class, add only the slots `ScheduleExists` reports missing,
   via `classesSvc.AddSchedule(ctx, sc, classID, req)` — it already inherits
   `class.TeacherID` (`classes/service.go:185`), so no new seam is needed.
   **Pass `EffectiveFrom` explicitly** on every `ScheduleRequest`: with it
   blank, `AddSchedule` defaults to the *database* class's `start_date`
   (`:185` → `:79-100`) while the pre-check asked with the file's date, so the
   two disagree and every re-import appends another duplicate slot forever —
   `class_schedules` has no unique index to stop it. (The mismatch rule above
   makes the two dates provably equal, but pass it anyway so the invariant is
   local and not two call frames away.)
2. **Contact** — anchor is the *class's* teacher, because `uq_contacts_phone`
   is per-teacher. `FindIDByPhone` hit → reuse. Miss → `contactsSvc.Create`.
3. **Student** — same anchor, `contact_id` from step 2. `FindIDByName` hit →
   reuse. Miss → `studentsSvc.Create`.
4. **Enrollment** — same anchor.
   `enrollmentsSvc.List(ctx, anchor, ListFilter{StudentID, ClassID}, …)`:
   an **open** row → reuse; an **ended** row → row error
   (`ENROLLMENT_ENDED`), never a silent re-open. `uq_enrollments_active` is
   partial on `ended_on IS NULL`, so a departed student has no open enrollment
   and would otherwise be re-enrolled with `started_on` = the class start date
   — retroactively active for every past session
   (`enrollments/repository.go:182-193`), producing months of attendance rows
   and invoices for a child who left. Re-admitting a student is a deliberate
   act. No row → `enrollmentsSvc.Create`; the unit price is copied by the
   enrollments service from the class default and this feature never sets one.

Error codes owned by this phase: `CLASS_EXISTS_MISMATCH`, `ENROLLMENT_ENDED`,
`CONTACT_DELETED` (a soft-deleted contact matching the phone — `uq_contacts_phone`
is partial on `deleted_at IS NULL`, so it would otherwise silently duplicate).

**Consequence to state in the release note:** because contacts are keyed per
teacher, one parent whose two children study under two different teachers
becomes two contact rows — two statement links, two balances, two Zalo
mappings. That is the recorded 260811 design
(`schema_design.sql:155-161`), not an import artefact, but the import is where
an operator will first notice it.

**Measure `MaxRowsPerSheet`.** Per student row this design issues roughly ten
round trips (three pre-checks plus, inside the services, `checkContact`,
`ClassDefaultPrice`, `StudentExists`, two inserts and two joined `GetByID`
read-backs). The stand-in cap of 500 must be re-derived here against a measured
commit time and the server's `WriteTimeout: 30s` (`server.go:24`). If the
measurement forces a much smaller number, lower the cap — do **not** switch to
a partial commit, which contradicts user decision 4.

## Related Code Files

- Modify: `apps/api/internal/features/classes/service.go`, `repository.go`
  (+ `FindActiveByName`, `ScheduleExists`) and their tests
- Modify: `apps/api/internal/features/contacts/service.go`, `repository.go`
  (+ `FindIDByPhone`) and their tests
- Modify: `apps/api/internal/features/students/service.go`, `repository.go`
  (+ `FindIDByName`) and their tests
- Modify: `apps/api/internal/features/imports/{service,dto}.go`
- Create: `apps/api/internal/features/imports/integration_test.go`
- Create: `apps/api/internal/features/imports/lock.go` (advisory lock +
  `SET LOCAL statement_timeout`)

## Implementation Steps (TDD)

### Tests Before
1. Per-feature `service_test.go`: each new lookup returns `false` for a row
   belonging to a different teacher when the scope is non-owner; `FindIDByName`
   matches a row whose `display_note` is **NULL** when passed `nil`, and does
   not match it when passed a non-nil note; `FindActiveByName` does not match
   an **archived** class.
2. `imports/service_test.go` (fake lookups + fake writers + `noopTxManager`):
   every entity created on a clean run; every entity reused on a second run
   with the same input **and blank display notes**; a class whose stored
   `unit_price` differs from the file → `CLASS_EXISTS_MISMATCH` and no writes;
   an ended enrollment → `ENROLLMENT_ENDED`; a writer error aborts and no later
   writer is called; `dry_run=true` produces the same counts as the commit and
   calls no writer; lock not acquired → `Conflict`.
3. `integration_test.go` (build tag `integration`, real Postgres):
   - **round trip**: the example workbook → 2 classes, 3 schedules, 3 contacts,
     3 students, 3 enrollments, each anchored to the expected `teacher_id`
   - **idempotency**: the same bytes again, **with every `display_note`
     blank** → identical row counts and unchanged `updated_at` on every touched
     row. (An idempotency test where every student has a note would pass while
     the real path is broken — this is the exact defect the first draft shipped)
   - **schedule stability**: importing three times leaves exactly 3 schedule
     rows, not 9
   - **rollback**: a workbook whose last row violates a constraint leaves
     **zero** rows across all five tables
   - **FK guard, both halves**: an anchor from **another center** fails the
     insert; an anchor for a **removed member of the same center** also fails —
     the FK does not catch this one, so the guard must come from resolution
     (`ListMembers` is active-only). This is the case the first draft's test
     silently skipped
   - **unassigned class**: blank teacher phone → class `teacher_id` = owner
   - **length overflow**: a 101-character class name is a row error before any
     write, never a `22001`

### Refactor
4. Implement the four lookups, `lock.go`, then `apply`.
5. Measure the commit time for the cap and record the number in `columns.go`
   with a comment naming the measurement.

### Regression Gate
```sh
make test-api && make lint-api
```

## Todo

- [x] Four scope-typed lookups on their owning features
- [x] `TryLockCenter` + `SET LOCAL statement_timeout`
- [x] `apply` shared by dry run and commit
- [x] Integration suite incl. NULL-note idempotency, thrice-import schedule
      stability, rollback, both FK-guard halves
- [x] `MaxRowsPerSheet` re-derived from a measurement

## Success Criteria

- [x] Second import of the same file — with blank display notes — reports
      `0 created` for all five entities and mutates nothing
- [x] Three imports leave three schedule rows
- [x] Rollback test leaves zero rows
- [x] Both FK-guard halves tested; the removed-member case is refused by
      resolution
- [x] Imported rows carry the *class teacher's* id, not the owner's
- [x] Archived-class, field-mismatch and ended-enrollment cases are row errors

## Risk Assessment

- **A `23505` anywhere kills the transaction.** Every insert must be preceded
  by its pre-check; the advisory lock is what makes the pre-checks sound. If
  the lock is cut, a duplicate class is the failure mode — do not cut it
  silently.
- **Transaction size.** The measurement in step 5 is not optional; without it
  the cap is a guess and the client's 10s axios timeout (Phase 4) will fire
  while the server keeps committing.
- **`AddSchedule`'s `effective_from` default** is two call frames away from
  this code and is the single easiest thing to regress. The explicit
  `EffectiveFrom` plus the thrice-import test are the guard.
- **A service growing its own `WithinTx` later** still joins the ambient
  transaction, so this stays correct as the four services evolve.

## Security Considerations

Every anchor originates from `MemberIDsByPhone(sc)` or from `sc.TeacherID`.
The FK guard is a cross-center backstop only — stated plainly, and both halves
are now tested. The commit re-runs the full resolution rather than trusting
anything the client learned from a dry run: there is no token, no staged file,
and nothing the client can replay to skip a check.

## Next Steps

Phase 4 puts a UI on the endpoint and closes out the docs.

## Outcome

Done. `make test-api` green (75.2% coverage, floor 60%), `make lint-api` clean.

Three things the plan did not predict:

1. **`SET LOCAL statement_timeout = ?` is a syntax error.** Postgres parses SET
   before parameters exist, so the value is spliced from the package constant
   instead of bound (`lock.go`). Every commit failed with `42601` until this
   was fixed; only the integration test caught it.
2. **`Pluck` into a bare `uuid.UUID` does not work.** Both new id lookups
   (`contacts.FindIDByPhone`, `students.FindIDByName`) returned
   `converting driver.Value type string to uint8` — GORM skips
   `uuid.UUID`'s `sql.Scanner` and takes its element-wise `[16]byte` path. They
   now select into a struct field, the same shape `testutil.ScopeFor` uses.
   This only fires when a row **is** found, so the first import passed and
   every re-import 500'd: the exact defect the NULL-note idempotency test was
   written for.
3. **`CONTACT_DELETED` was not needed.** `uq_contacts_phone` is partial on
   `deleted_at IS NULL` (`000001_baseline_schema.up.sql:94-95`), so a
   soft-deleted contact holds no key and a fresh create succeeds. The scoped
   lookup skips it, which is the correct behaviour — no row error to raise.
   Covered by `TestFindIDByPhoneIgnoresDeletedContacts`.

Measured cap: a full 500-class/500-student workbook commits in ~1.3s and
re-imports in ~0.5s against a local Postgres. `MaxRowsPerSheet` stays 500
(user decision 9); the comment in `columns.go` records the measurement and its
caveat.

The removed-member half of the FK-guard test needed a correction of its own:
offboarding is `left_at` **plus** `user_accounts.status = 'disabled'`, and
`ListMembers` reads only the latter — stamping `left_at` alone leaves the
teacher resolvable.
