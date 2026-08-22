---
title: "Excel Roster Import (Owner-Run)"
description: "Owner uploads one .xlsx to create classes + schedules, parent contacts, students, and enrollments for any teacher in their center. One endpoint with dry_run, all-or-nothing, idempotent on re-import."
status: completed
priority: P2
effort: 3d
branch: master
tags: [feature, backend, frontend, api, import, tenancy]
blockedBy: []
blocks: []
created: 2026-08-21
revised: 2026-08-21
---

# Excel Roster Import (Owner-Run)

## Overview

A center owner uploads a two-sheet workbook (`Lop`, `HocSinh`) and the API
creates, in one transaction: classes + their weekly schedules, parent contacts,
students, and open enrollments. One endpoint, with `dry_run` for the check
pass. Any invalid row rejects the whole file; nothing is written. Re-importing
the same file is a no-op because every row is matched on a natural key, not on
file identity.

The feature exists because a new center arrives with a roster already on paper
or in a spreadsheet, and typing it into the app class-by-class is the reason
they never finish onboarding.

Template spec source: [`task/import_student_and_class.md`](../../task/import_student_and_class.md)
(amended — see *Scope cuts*).

> **This plan was rewritten on 2026-08-21 after a 4-reviewer red-team pass.**
> The first draft's `CreateFor` refactor across four services was deleted as
> redundant, its two-endpoint split collapsed to one, and five defects in the
> idempotency design were corrected. See `## Red Team Review` at the end.

## User decisions this plan encodes

| # | Decision | Consequence |
|---|----------|-------------|
| 1 | Import is **owner-only** | Service-level `if !sc.IsOwner` gate, same shape as `requireOwner` (`invitations/service.go:117-123`) |
| 2 | Owner **may** create rows anchored to any teacher in their center | Already permitted by `docs/api-guidelines.md:96-97`. No new seam, no ADR — see below |
| 3 | A class **may have no teacher** | `SĐT giáo viên` optional; an empty cell anchors the class to the **owner**, who is himself a teacher |
| 4 | Invalid row ⇒ reject whole file | `dry_run` pass before the real one; the real pass re-validates |
| 5 | Template download endpoint, header + one example row | `GET /imports/roster/template` returns a generated `.xlsx` |
| 6 | A **wrong-but-valid** teacher phone is an accepted risk | 2026-08-21. See *Accepted risks* |
| 7 | A reused class whose fields differ from the file is a **row error** | 2026-08-21. The file and the database disagreeing about one class is ambiguous; the owner resolves it, not the import |
| 8 | One parent with children under two teachers ⇒ **two contact rows is correct** | 2026-08-21. No warning, no dedupe across teachers. Two statement links, two balances, two Zalo mappings is the intended behaviour of `uq_contacts_phone(teacher_id, phone)` |
| 9 | `MaxRowsPerSheet = 500` stands for now | 2026-08-21. Phase 3 still measures the commit time and reports it; the cap only moves if the measurement forces it |
| 10 | `task/import_student_and_class.md` **stays** | 2026-08-21. Deleted later, once the generated template has been used in anger. Phase 4 does not touch it |

### Decision 2 needs no ADR — the contract already allows it

The first draft claimed this decision superseded
`plans/260811-1055-manager-class-oversight/plan.md:98` ("không tạo hộ") and
budgeted an ADR entry plus a `CreateFor` seam on four services. **That was
wrong.** The written contract already says:

```
docs/api-guidelines.md:96-97
- Writes keep the invariant `teacher_id = $self` for the teacher role; owners
  may write on behalf of any teacher in their center.
```

And no create path reads `sc.IsOwner` — `grep -n "IsOwner"` across
`classes/service.go`, `contacts/service.go`, `students/service.go`,
`enrollments/service.go` returns **zero matches**. Every one of them derives
its anchor from the `authctx.Scope` argument alone. So calling the *existing*
`Create` with a synthetic anchor is already the whole feature:

```go
anchor := authctx.Scope{TeacherID: resolvedTeacherID, CenterID: sc.CenterID, IsOwner: false}
classesSvc.Create(ctx, anchor, req)   // stamps resolvedTeacherID, checks refs without owner rights
```

The 260811 rule that still binds is the narrower one, and this plan keeps it:
**a teacher id never comes from client input.** No request DTO gains a
`teacher_id`; the anchor comes from a center-scoped phone lookup.

## Cross-Plan Dependencies

None blocking. `plans/` holds 26 plan directories; 16 declare
`status: completed`/`done` and **9 carry no `status:` frontmatter at all**, so
"all plans done" cannot be asserted from a status grep alone.
`260811-1508-class-schedule-slots` owns the schedule surface this plan writes
to and its header reads *"implemented, review fixes applied — pending commit"*,
but `features/classes/` is committed (`a4c8277`) and the working tree is clean
there — stale bookkeeping, not in-flight work. Its stated rule
(`effective_from = start_date`, plan line 43) matches the code and this plan.

## Phases

| # | Phase | Status |
|---|-------|--------|
| 1 | [Workbook Parser and Template Builder](./phase-01-workbook-parser-and-template-builder.md) | Done |
| 2 | [Resolution and Import Endpoint](./phase-02-resolution-and-import-endpoint.md) | Done |
| 3 | [Commit Path and Idempotency](./phase-03-commit-path-and-idempotency.md) | Done |
| 4 | [Web UI, OpenAPI and Verification](./phase-04-web-ui-openapi-and-verification.md) | Done |

Sequential: 1 → 2 → 3 → 4.

## HTTP contract

| Method | Path | Auth | Body | Success |
|---|---|---|---|---|
| `GET` | `/api/v1/imports/roster/template` | owner | — | `200` `.xlsx` stream (outside envelope) |
| `POST` | `/api/v1/imports/roster` | owner | `multipart/form-data`: `file`, `dry_run` (`"true"`/`"false"`) | `200` envelope, `Report` |

Two routes, not three (94 → 96 endpoints). `dry_run=true` runs the identical
resolution and the identical existence lookups, writes nothing, and returns
`committed:false`. The first draft split these into `/validate` and the commit
route and justified it with "no server-side staging" — a property a `dry_run`
flag preserves identically, so the split bought nothing and cost a duplicated
handler test suite, a second swag block, a second web API function, a second
MSW handler, and a two-step UI state machine.

Non-owner ⇒ `403 FORBIDDEN`. Any row error ⇒ `422 VALIDATION_ERROR` with
`details.errors[]`. Concurrent import in the same center ⇒ `409 CONFLICT`.

## Three design points that decide whether this feature is safe

### 1. Tenancy — the phone lookup *is* the authorization check

The workbook names teachers by phone. Resolving a phone to a `teacher_id` is
the single place this feature can become an authorization bypass.

```go
// WRONG — a global directory. Any phone in the country resolves; the
// owner of center A can now write rows into center B.
tid, err := teachersSvc.GetByPhone(ctx, phone)      // teachers/service.go:211
```

```go
// RIGHT — the directory is derived from the caller's own scope, so a phone
// outside the center simply does not resolve. No separate "is this teacher
// mine?" check exists, because it cannot be forgotten.
dir, err := s.members.MemberIDsByPhone(ctx, sc)     // Phase 2, reads sc.CenterID
tid, ok := dir[normalized]
if !ok { /* row error: "số này không thuộc trung tâm của bạn" */ }
```

The method takes `authctx.Scope`, **not a raw `centerID`** — a raw-uuid
signature would let any caller dump any center's phone directory, which is the
opposite of "cannot be forgotten". It is implemented over the existing
`centers.Repository.ListMembers` (`centers/repository.go:200-215`) — one query,
already `WHERE t.center_id = ?`, already joined to active `user_accounts`.

**Regression risks to guard in review:**
- A future "improve the error message" change is exactly what tempts someone to
  add a global lookup so the API can say *"phone belongs to another center"*.
  That message is an enumeration oracle. The error text must stay
  center-relative.
- `centers.Service` is a **process-lifetime singleton** that also backs
  `middleware.ResolveScope` on every authenticated request. A per-center cache
  on that struct would be a cross-tenant leak with a very quiet diff.
  `MemberIDsByPhone` must stay a query-per-call.

**What the FK guard actually does.** The first draft called
`(teacher_id, center_id) → center_members` a live-membership backstop and
licensed skipping a membership check. **False.** The FK targets
`center_members`'s PRIMARY KEY `(teacher_id, center_id)`, and offboarding is
`UPDATE left_at`, never `DELETE` (`migrations/000007_centers.up.sql:66-72`;
`centers/repository.go:268-278`). `uq_center_members_active` is a *separate*
partial index no FK can reference. So the guard blocks a **cross-center** anchor
and nothing else; a removed member of the *same* center passes it. The real
protection is `ListMembers`' `ua.status = active` filter — the thing the draft
called redundant. Phase 3 tests both cases.

### 2. Anchoring — a synthetic `Scope`, no refactor

Nothing in `classes`, `contacts`, `students`, `enrollments` changes. The
imports service builds one anchor per resolved teacher and calls their existing
`Create`. Two properties follow, and both must be asserted by test:

- `anchor.IsOwner` is `false`, so `students.checkContact` and
  `enrollments.ClassDefaultPrice`/`StudentExists` still run without owner
  rights — an imported student can only point at a contact belonging to **the
  same anchor teacher**. Cross-teacher stitching stays impossible.
- `anchor` is only ever built from `MemberIDsByPhone` output or from
  `sc.TeacherID`. Never from a request body.

**But those services validate nothing.** Every constraint on
`CreateClassRequest`, `CreateRequest` &c. is a gin `binding` tag executed at
bind time; a service-to-service caller gets none of it. Two consequences the
parser must absorb (Phase 1):

- `classes/service.go:52` dereferences `*req.DefaultUnitPrice` and `:97`
  dereferences `*sr.Weekday`. A nil pointer is a **panic**, not a 422.
- Length caps (`name` ≤ 100, `display_note` ≤ 50, `full_name` ≤ 100) are
  enforced only by Postgres `VARCHAR`, i.e. as a `22001` **mid-transaction**,
  which rolls the whole import back with an opaque 500 and no line number.

So `coerce.go` mirrors every `binding` tag, with a `TOO_LONG` row error, and a
test pins the caps against the DTO tags.

### 3. Idempotency — natural keys, pre-checked under a lock

The user's question — *"làm sao phân biệt file nào đã import?"* — has no good
answer at the file level. Excel rewrites bytes on every save, so a hash changes
without the content changing. **File identity is not tracked at all.** Every
row is matched on a business key:

| Row | Natural key | How |
|---|---|---|
| class | `(center_id, teacher_id, name)`, alive **and `status='active'`** | pre-check; name trimmed + NFC-normalized |
| schedule | `(class_id, weekday, start_time, effective_from)` | pre-check; `effective_from` passed **explicitly** |
| contact | `(teacher_id, phone)` alive | pre-check (index exists but see below) |
| student | `(teacher_id, contact_id, full_name, display_note)` alive | pre-check; `display_note IS NOT DISTINCT FROM $4` |
| enrollment | `(student_id, class_id)`, **any** enrollment, open or ended | pre-check |

**All five are pre-checks.** The first draft said the two indexed keys should
instead insert and translate the unique violation into a "reuse", citing
`contacts/service.go:33-34`. That pattern is safe in a single-statement request
and **fatal here**: PostgreSQL aborts the whole transaction on `23505`, every
later statement returns `25P02`, and `GormTxManager.WithinTx` joins the ambient
transaction without a savepoint (`database/tx.go:24-26`; `grep -rn SAVEPOINT`
→ zero hits). Any `23505` reaching the import is a hard failure and a full
rollback, never a reuse.

Five correctness rules the keys need, each a real defect in the first draft:

1. **`display_note` is NULL, not `''`.** `notePtr("")` returns `nil`
   (`students/dto.go:56-61`), so `display_note = ''` never matches — and
   `Ghi chú phân biệt` is blank on essentially every row. Predicate must be
   `IS NOT DISTINCT FROM`. Without this, every re-import duplicates every
   student, each duplicate gets a fresh `student_id` which sails past
   `uq_enrollments_active`, and every parent is billed twice.
2. **Names are trimmed and NFC-normalized** in `coerce.go`. Postgres `=` on
   `VARCHAR` is byte comparison; macOS Excel emits NFD, the web UI emits NFC,
   and `Toán` ≠ `Toa´n`.
3. **`status='active'` is part of the class key.** An archived class has
   `deleted_at IS NULL` and would otherwise be silently reused.
4. **A reused class whose file fields differ from the DB is a row error**
   (`CLASS_EXISTS_MISMATCH`) — user decision 7. This also removes the
   `effective_from` ambiguity in rule 5, since a reused class now provably
   shares the file's `start_date`.
5. **`effective_from` is passed explicitly** on every `ScheduleRequest`.
   `AddSchedule` otherwise defaults it to the *database* class's `start_date`
   (`classes/service.go:185` → `:79-100`), while the pre-check asks with the
   file's date — so writer and reader disagree and every re-import appends a
   duplicate schedule row, forever, with no unique index to stop it.
6. **An ended enrollment is a row error, not a silent re-open.**
   `uq_enrollments_active` is partial on `ended_on IS NULL`, so a departed
   student has no open enrollment and would be re-enrolled with
   `started_on` = the class start date — retroactively active for every past
   session (`enrollments/repository.go:182-193`), producing months of
   attendance and invoices for a child who left. Re-admitting a student is a
   deliberate act.

Re-importing an unchanged file against an **unmodified database** therefore
reports `0 tạo mới / N dùng lại` and writes nothing.

Twins: two rows identical on `(contact phone, student name, class)` with the
same or empty `display_note` → row error naming both lines.

**Concurrency.** All five keys are pre-checks, so the commit takes
`pg_try_advisory_xact_lock(hashtext(center_id::text))` — `try`, not the
blocking variant, returning `409` when another import holds it. The blocking
form parks a pooled connection indefinitely, and with `DB_MAX_OPEN_CONNS`
defaulting to 25 (`config.go:47`) a handful of retries starves **every**
tenant. Note the `::text` cast: `hashtext` takes `text`, `center_id` is `uuid`,
and the draft's `hashtext(center_id)` does not compile. Ordinary create routes
(`POST /classes` &c.) take no lock and are **not** serialised against the
import — accepted, since the import is an onboarding action.

## Key design decisions (plan-level)

- New feature slice `internal/features/imports/`. It coordinates four other
  features; putting it inside any one of them would couple the rest.
- **No `imports` repository.** The first draft gave it five raw lookups across
  four features' tables, three of them with no tenant column at all, bypassing
  the mandated `scoped()` contract (`docs/api-guidelines.md:80-93`). Instead
  each lookup lives on the feature that owns the table and takes
  `authctx.Scope`, so every one goes through that feature's existing `scoped()`
  helper. Exactly one is free: `enrollments.List` already filters
  `{StudentID, ClassID}` exactly, with `Active: nil` meaning "open or ended" —
  which is the key rule 6 needs. The other four are new, ~8 lines each.
  (`contacts.List` and `students.List` search with `ILIKE '%…%'`, so they are
  display filters, not exact-match keys, and must not be reused as such.)
  Details in Phase 3.
- **Dependency:** `github.com/xuri/excelize/v2`, **BSD-3-Clause** (not MIT — the
  first draft had this wrong). The only maintained pure-Go xlsx reader *and*
  writer; the template endpoint needs writing.
- **First multipart surface in the API** — `grep -rn "multipart\|FormFile"
  internal/` returns nothing. Phase 2 sets the precedent: `http.MaxBytesReader`
  **before** `c.FormFile`; `excelize.OpenReader` with `UnzipSizeLimit` and
  `UnzipXMLSizeLimit`; the streaming `Rows()` iterator, not `GetRows`, which
  materializes the whole sheet before any row cap can apply; content validated
  by opening the workbook, never by the client-supplied filename.
- **Owner gate in the service**, matching `requireOwner`
  (`invitations/service.go:117-123`). No `RequireOwner` middleware exists and
  this feature is not the place to add one.
- **Read every cell as a string.** `GetCellValue` returns the display string;
  typed getters re-interpret locale-dependent dates and eat the leading zero of
  a phone.
- **A class with no teacher is anchored to the owner.** `classes.teacher_id` is
  `NOT NULL` with an FK guard, and every downstream row carries
  `teacher_id NOT NULL` too. Nullable is a nine-table schema change.
- Money stays `BIGINT` đồng; `weekday` follows `time.Weekday` (CN = 0).

## Scope cuts (deliberate)

| Cut | Why |
|---|---|
| `CreateFor` seam on 4 services | Redundant — no `Create` reads `IsOwner`, so a synthetic anchor `Scope` already does it. Removes 4 new public methods, 4 delegations, 4 test suites, an ADR entry, a `CLAUDE.md` line and an api-guidelines note |
| Separate `/validate` endpoint | A `dry_run` field preserves every stated property |
| `imports` repository over 4 features' tables | Lookups belong to the feature that owns the table, scope-typed, through `scoped()` |
| `import_batches` audit table + migration | Idempotency is by natural key. **This plan adds no migration** |
| `Đơn giá riêng` column on Sheet 2 | `enrollments.CreateRequest` has no price field; `Create` always copies `classes.default_unit_price` (`enrollments/service.go:40,70`) |
| Two-goroutine concurrency integration test | `docs/api-guidelines.md:198-210` reserves integration tests for index-dependent SQL. The `try`-lock is one line; the test would be the flakiest in the suite |
| Client-side byte cap, 100-error truncation | Duplicates the server cap; the row cap already bounds the error list |
| `#`-prefix example rows + template↔parser round-trip test | `columns.go` is the single source of column names, so drift is structurally impossible. The example row is a plain second row the parser skips by index |
| Playwright e2e spec | Needs an undeclared npm xlsx writer; handler tests + the integration round-trip already cover the contract |
| Partial import, row updates, class reassignment | User decisions 4, 7 and *Accepted risks* |

## Accepted risks (user decision 6, 2026-08-21)

**A wrong-but-valid teacher phone is unrecoverable.** If the owner writes the
phone of teacher B for a class actually taught by teacher A, and both are
members, the phone resolves cleanly and nothing errors. The class, its
students, its contacts and its enrollments are all anchored to B. Consequences:
A never sees the class; billing periods are unique on
`(teacher_id, year, month)` so the money lands in B's period; statements are
sent from B's personal Zalo.

Recovery is manual and destructive: `classes.Delete` refuses while enrollments
are open (`classes/service.go:160-172`), and `students.Delete` overwrites
`full_name` and stamps `anonymized_at` (`students/repository.go:157-174`) — so
undoing a mis-keyed import means irreversibly destroying the children's names.

The user was shown a dry-run confirmation screen showing resolved teacher
**names** and a name-column cross-check, and chose neither. Recorded here so a
later audit does not re-open it as an oversight. The first draft's claimed
remedy — *"correct the file and re-import before the class has students"* — is
impossible, since the import creates class and students in the same
transaction; it has been removed.

## Success Criteria

- [ ] Owner imports the example workbook and gets: 2 classes, 3 schedules,
      3 contacts, 3 students, 3 enrollments
- [ ] Re-importing the identical file **against an unmodified database**
      reports `0 created / all reused` and changes no row — asserted with
      **blank** `display_note` on every student, and with `updated_at`
      compared before/after
- [ ] A workbook naming a phone outside the center is rejected with a
      row-level error and **zero** rows written
- [ ] A row exceeding a `VARCHAR` cap is a row error with a line number, never
      a `22001`
- [ ] A name matching an **archived** class, a class whose file fields differ
      from the DB, and a student with an **ended** enrollment are each row
      errors
- [ ] A member (non-owner) gets `403`; a second concurrent import gets `409`
- [ ] A class row with an empty teacher phone lands on the owner
- [ ] `make test-api` (≥60% coverage), `make test-web`, `make lint`,
      `make api-docs` all clean

## Open questions

None. Questions 1-3 of the 2026-08-21 draft were answered by the user the same
day and are recorded as decisions 8-10 in the table above.

## Red Team Review

### Session — 2026-08-21
**Reviewers:** 4 (Security Adversary / Fact Checker, Failure Mode Analyst /
Flow Tracer, Assumption Destroyer / Scope Auditor, Scope & Complexity Critic /
Contract Verifier), Full verification tier.
**Findings:** 21 raw → 15 after evidence filter and dedupe. **15 accepted, 0
rejected.** Severity: 5 Critical, 7 High, 3 Medium.
Every Critical was independently re-verified against source before adoption.

| # | Finding (evidence) | Sev | Applied to |
|---|---|---|---|
| 1 | "Insert and translate the unique violation → reuse" aborts the transaction. `WithinTx` joins without a savepoint (`database/tx.go:24-26`); `grep -rn SAVEPOINT` → 0 hits. Postgres `23505` → `25P02` on every later statement. Phase 4 also contradicted itself: its own interface declared the pre-checks it forbade | Critical | P3 |
| 2 | Student key compared `display_note` to `''` while `notePtr("")` stores **NULL** (`students/dto.go:56-61`). Blank notes are the common case → every re-import duplicates every student → fresh `student_id` sails past `uq_enrollments_active` → **parents billed twice** | Critical | plan §3, P3 |
| 3 | `AddSchedule` stamps `effective_from` from the **DB** class (`classes/service.go:185` → `:79-100`) while the pre-check asks with the file's date → a duplicate schedule row appended on every re-import, forever; no unique index on `class_schedules` | Critical | plan §3, P3 |
| 4 | The whole `CreateFor` refactor was redundant — `grep -n "IsOwner"` across the four create services → **zero matches**; a synthetic anchor `Scope` into the existing `Create` is byte-identical. Also `docs/api-guidelines.md:96-97` already sanctions owner-on-behalf writes, so the "supersedes 260811" framing and its ADR entry were wrong | Critical | Phase 2 deleted; plan §2, §"Decision 2" |
| 5 | Services validate nothing — all constraints are gin `binding` tags. `classes/service.go:52,97` deref `*DefaultUnitPrice`/`*Weekday` (**panic**); length caps surface as a mid-transaction `22001` with no line number | Critical | P1 (`TOO_LONG`, DTO-cap pin) |
| 6 | The FK guard is a **cross-center** guard, not a live-membership one: it targets `center_members`'s PK and offboarding is `UPDATE left_at` (`migrations/000007_centers.up.sql:66-72`). The draft licensed skipping a membership check, and its proof test exercised only the case the FK already catches | High | plan §1, P3 (both halves tested) |
| 7 | Decision 3b's remedy was impossible — the import creates class **and** students in one transaction, so "re-import before it has students" can never hold. Recovery requires `students.Delete`, which overwrites `full_name` and stamps `anonymized_at` | High | plan §"Accepted risks" (user decision 6) |
| 8 | Timeouts unmodelled: axios `timeout: 10_000` (`client.ts:15`), `ReadTimeout 10s` / `WriteTimeout 30s` (`server.go:23-24`). Client aborts, handler keeps committing, owner cannot tell whether data landed | High | P3 (measure cap), P4 (per-request timeout) |
| 9 | `pg_advisory_xact_lock` blocks forever, no rate limit, `DB_MAX_OPEN_CONNS` 25 shared across tenants (`config.go:47`) → one owner's retries starve every center. Also `hashtext(center_id)` does not compile — `hashtext` takes `text` | High | P2 (rate limit), P3 (`try` lock + `::text` + `statement_timeout`) |
| 10 | Phase 5 rested on two false frontend facts: `ApiError`/`toApiError` never read `details` (`lib/api/errors.ts:11-29,52-73`), and `GET /centers/me` is a role-shaped union where the member body is `{center_name}` with no `is_owner` (`center-schemas.ts:38-49`) | High | P4 |
| 11 | Zip-bomb mitigation ineffective — `GetRows` materializes the sheet before any row cap; no `UnzipSizeLimit`/`UnzipXMLSizeLimit`; content gated on the client-supplied filename | High | P1 (`Rows()` iterator + limits), P2 (content validation) |
| 12 | The `imports` repository hand-rolled tenancy across four features' tables, three methods with no tenant column at all, bypassing the mandated `scoped()` contract (`docs/api-guidelines.md:80-93`) | High | P3 (lookups moved onto owning features, scope-typed) |
| 13 | The class key was byte-exact, status-blind and price-blind: NFD/NFC spellings diverge; an **archived** class has `deleted_at IS NULL` and was silently reused; a reused class invoiced at the **stored** price, not the file's. An **ended** enrollment was silently re-opened, retroactively active for every past session (`enrollments/repository.go:182-193`) | High | P1 (NFC), plan §3, P3 (user decision 7) |
| 14 | Two endpoints unjustified — the "no server-side staging" argument holds identically for a `dry_run` flag. Cost: a duplicated handler suite, a second swag block, a second web API function, a second MSW handler, a two-step UI gate. Folded in: `validate` structurally could not report `Reused` (its phase forbade DB I/O while the other phase owned the lookups) | Medium | plan §"HTTP contract", P2, P3 |
| 15 | Gold plating and bookkeeping: two-goroutine concurrency test, three overlapping size caps, 100-error truncation, `#`-prefix + round-trip test; excelize is **BSD-3-Clause** not MIT; roster has **no `/roster` URL prefix**; the e2e xlsx fixture needed an undeclared npm dependency; `centers/service.go:124` was mis-cited as an owner gate (it is a reduced-response branch — the real pattern is `invitations/service.go:117-123`); "all 16 plans done" was a count of the matching subset — there are 26 plans and **9 carry no `status:` at all** | Medium | plan §"Scope cuts", §"Cross-Plan Dependencies", P1, P4 |

Two reviewer claims were **rejected on evidence**:
`apps/web/src/features/roster/__tests__/roster-handlers.ts` was reported
missing — it exists. And "three of the five lookups already exist as scoped
`List` calls" — only `enrollments.List` does;
`contacts.List`/`students.List` search with `ILIKE '%…%'` and are display
filters, not exact-match keys.

**Net effect:** 6 phases → 4, 3.5d → 3d (5h+6h+6h+7h), three routes → two, one
migration → none. The reviewers estimated ~2d; the phase breakdown does not
support that once the measurement step and the shared `errors.ts` change are
counted.

### Whole-Plan Consistency Sweep

Re-read `plan.md` and all four phase files after applying the findings.
Checked for: `CreateFor`, the `/validate` route, `import_batches`,
`file_sha256`, the `imports` repository, `GetRows`, `MaxRowsPerSheet = 2000`,
MIT, `/roster/import`, the ADR entries, the `CLAUDE.md` edit, the e2e spec,
`MemberIDsByPhone(ctx, centerID, phones)`, the 94→97 endpoint count, and the
"supersedes 260811" framing. All removed or corrected; no phase file references
a deleted phase. Phase numbering and dependencies are contiguous (1 → 2 → 3 →
4). **Zero unresolved contradictions.**

## Implementation Review

### Phase 1 — code review, 2026-08-21
One reviewer over the parser package, with every claim reproduced against the
source. **13 findings, all accepted**; 1 reviewer claim rejected on evidence.

| # | Finding (evidence) | Sev | Fix |
|---|---|---|---|
| 1 | `iter.Columns()` returns the **display** string, which is rendered through the cell's number format. A genuine Excel date cell reads `09/01/2025` under mm/dd and `01/09/2025` under dd/mm — the same day, **eight months apart** once parsed as dd/mm, with nothing to warn on. The plan's "read every cell as a string" closes the typed-getter trap, not this one | Critical | `Columns(excelize.Options{RawCellValue: true})`; a date cell now arrives as its serial and is rejected with a message naming the fix. Bonus: `150,000` now reads as `150000` instead of being refused |
| 2 | excelize's row iterator discards XML decoder errors, so a worksheet cut mid-stream ends early with `iter.Error()` nil — a 300-row roster imports as 40 and reports success | High | Verified: truncated file iterates 6 rows of 12, no error. `<dimension>` is useless for the cross-check (excelize leaves it at `A1` when cells are written individually), so the parser validates each worksheet part is well-formed XML before opening |
| 3 | An empty sheet never enters the row loop, so the header check never runs — a blank file imports as a silent success | High | Explicit header-seen flag |
| 4 | `GetSheetIndex` returns `(-1, nil)`, not an error, for a missing sheet — the guard was dead code and the operator saw the wrong message. It also matches case-insensitively, so `LOP` passed a contract saying the name is exact | Medium | Own exact-match lookup over `GetSheetList` |
| 5 | An appended 9th column was silently truncated with its data. `Đơn giá riêng` is a scope cut, i.e. exactly the column an operator re-adds — every per-student price would have been silently replaced by the class default | Medium | Non-empty header cells past the contract are a whole-file error |
| 6 | One stray cell anywhere on the sheet kept an otherwise-empty row alive, producing a wall of `MISSING_REQUIRED` errors pointing at blank rows | Medium | Blankness judged on the contract's columns only |
| 7 | The DTO cap pin passed through three realistic drifts (cap removed, field renamed, new earlier field) because the unanchored regex fell through to the *Update* request's same-named field | Medium | Rewritten to parse the file with `go/ast` and select the named struct. Mutation-tested: all three drifts now fail the test |
| 8 | The template round-trip test read the same slices it verified, so a header rename stayed green while invalidating every template already downloaded | Medium | Golden literal header lists |
| 9 | `time.Parse` accepts years 0000–9999 and `classes/dto.go` validates only the layout, so `01/09/0225` would create a class 1800 years off | Medium | Plausibility window 2000–2100 |
| 10 | The unzip-limit comment overstated the protection: `UnzipXMLSizeLimit` spills to a temp file rather than rejecting; only `UnzipSizeLimit` errors | Medium | Comment corrected to say what actually bounds a bomb (upload cap + row cap) |
| 11 | `cleanName` left control characters and zero-width codepoints in names — invisible in Excel, in the app and in Zalo, and fatal to every natural-key match in Phase 3 | Low | `stripInvisible` before NFC; newline collapses to a space rather than joining words |
| 12 | Money message named dots/commas/suffixes but not spaces, which it also rejects | Low | Covered by the raw-value fix |
| 13 | The 51-char note rule was only exercised by calling `capped` directly, never through the student row | Low | Covered by the new hardening tests |

**Rejected:** the reviewer reported `apps/web/src/features/roster/__tests__/roster-handlers.ts` as missing — it exists.

**Verified sound and left alone:** the NFD test literal is genuinely decomposed (bytes checked); the row cap is enforced as rows arrive, before materialisation; `parseStartTime` output always satisfies `hhmmPattern`; `parsePhone` output always satisfies `vnphone`; line numbers survive skipped and physically absent rows; the pointer-field guarantee holds by type, not by accident.
