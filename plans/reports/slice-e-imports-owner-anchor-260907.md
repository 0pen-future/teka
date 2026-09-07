# Slice E: imports owner anchor — report

Removes the hand-built owner `authctx.Scope` from the roster import path,
anchoring create/lookup entry points in contacts, students, classes,
enrollments through `authctx.OwnerAnchor` / `authctx.Anchor`, and renames
`centers/dashboard.go`'s `targetScope` to `viewAs`.

## Per-method decisions

### centers

- `Service.ResolveOwnerAnchor(ctx, sc authctx.Scope) (authctx.OwnerAnchor, error)`
  (`internal/features/centers/service.go:118`) already existed before this
  session: returns `authctx.MintOwnerAnchor(sc.TeacherID, sc.CenterID)` when
  the caller is the owner, otherwise looks up the current owner via
  `CenterOwner` and mints on that id. This is the only mint point wired into
  `imports`.
- `dashboard.go`'s `targetScope` → `viewAs` rename, with its doc comment
  rewritten, was also already complete before this session (no further edit
  needed): `internal/features/centers/dashboard.go:79-84`.

### contacts / students

Both already carried the "shared core, two entry points" shape from a prior
session: `Create(sc)` keeps the IsOwner gate; `CreateAnchored(ctx, a
authctx.OwnerAnchor, req)` is the anchored write; `FindIDByPhone` /
`FindIDByName` dedupe center-wide via `centerScoped(ctx, sc)`, taking the real
caller's `Scope` rather than an `Anchor`, since the dedupe check only reads
`CenterID`. This session's only additions were:
- `TestFindIDByPhoneIsCenterWideForEveryCaller` (already present from a prior
  session, unedited this session).
- New compile-time signature tests in both packages,
  `TestCreateAnchoredRequiresAnOwnerAnchor`: assigns the method value to a
  variable typed on the exact function signature
  (`func(context.Context, authctx.OwnerAnchor, CreateRequest) (*Row, error)`),
  which fails to build if `CreateAnchored`'s second parameter is ever loosened
  to a plain `authctx.Anchor`.

### classes

- `FindActiveByName` / `ScheduleExists` (`repository.go`, `service.go`)
  changed in place from `sc authctx.Scope` to `a authctx.Anchor` — these have
  no route-handler caller, only the roster import, so there was no dual-entry
  need. Repository gained a table-specific `anchored(ctx, a)` (for
  `classes`) and `anchoredSchedules(ctx, a)` (for `class_schedules`, a
  different table `ScheduleExists` queries) — an unconditional two-column
  filter, mirroring the `scoped`/`readScopedSchedules` split already present.
- `Create` = `CreateAnchored(sc.Self())`. No private lowercase core was
  introduced between them: unlike contacts/students there is no differing
  authorization gate between the two entry points, so `CreateAnchored` holds
  the full logic and `Create` is a one-line delegate — the literal spec
  wording, and the simpler of two equally correct shapes (KISS).
- `AddSchedule` / `AddScheduleAnchored` DO share a private `addSchedule(ctx,
  a authctx.Anchor, classID, startDate, req)` core, because `AddSchedule` (the
  route path) needs an extra `GetByID` lookup to learn the class's anchor and
  start date, which `AddScheduleAnchored` (import path — caller already holds
  the class object) does not need. That divergence is real, so the extra
  layer earns its keep.

### enrollments

Left `centerScoped`/`ActiveOnClass`/`sc.Self()` (already-added, out of scope)
untouched. Added:
- `create(ctx, actor authctx.Scope, a authctx.Anchor, req)` shared core:
  `Create` = `create(sc, sc.Self())`; `CreateAnchored(ctx, actor, a, req)` =
  `create(ctx, actor, a, req)`. The row anchors to `a`; the `StudentEnrolled`
  event's `ActorID` is `actor.TeacherID`, so the audit trail always credits
  whoever ran the write, never the row's anchor.
- `GetByIDAnchored(ctx, a, id)` — a new unconditional read-back needed because
  `CreateAnchored`'s read-back must work when `actor` and `a` differ (a member
  importing into a class anchored on a different teacher); the existing
  `GetByID(ctx, sc, id)` is WriteWide-aware and would miss such a row.
- `FindByStudentAndClassAnchored(ctx, a, studentID, classID)`, sharing a new
  `findByStudentAndClass(q *gorm.DB, ...)` core with the existing
  `FindByStudentAndClass(ctx, sc, ...)`.
- **`FindByStudentAndClass` scoping decision**: kept resolving against the
  caller's own teacher-scoped rows (`r.scoped(ctx, sc)`), i.e.
  teacher-anchored, not center-wide. Evidence:
  `TestFindByStudentAndClassStaysWithinTheAnchorTeacher` in
  `enrollments/service_test.go` (left untouched this session, per constraint)
  already asserts a center peer with a different teacher id does NOT see
  another teacher's enrollment. Enrollments are pedagogical data anchored on
  the class's own teacher (unlike contacts/students, which are center data
  anchored on the owner), so this scoping was already correct and needed no
  change — only the new `...Anchored` sibling was added for the import path,
  which explicitly names the class's own teacher as `a` rather than reading
  it off the caller.

### imports

- `MemberDirectory.CenterOwner` → `ResolveOwnerAnchor(ctx, sc) (authctx.OwnerAnchor, error)`,
  wired straight to `centers.Service.ResolveOwnerAnchor`.
- The four writer interfaces (`ClassWriter`, `ContactWriter`, `StudentWriter`,
  `EnrollmentWriter`) now demand the Anchored methods above; doc comments
  rewritten to explain the actor/owner/anchor split (see file header comment
  in `service.go`).
- `Import()`'s owner-resolution block: hand-built
  `authctx.Scope{TeacherID: ownerID, CenterID: scope.CenterID, IsOwner: true}`
  → `ownerAnchor, err := s.members.ResolveOwnerAnchor(ctx, scope)`.
- `apply(ctx, actor authctx.Scope, owner authctx.OwnerAnchor, plan, dryRun, rep)`:
  `actor` is the real importing caller (used for enrollments' event
  attribution and for contacts/students' center-wide dedupe reads); `owner`
  is the proven anchor contacts/students write under.
- `anchorFor(teacherID, centerID) authctx.Anchor` (was `authctx.Scope` with
  `IsOwner` implicitly false — a guard violation) — this was the actual bug
  motivating the whole slice: a hand-built `Scope{...}` composite literal
  outside `centers/`, caught by `TestScopeLiteralsOnlyWhereResolved`.
- `applyClass(ctx, anchor authctx.Anchor, ...)`: routes through
  `FindActiveByName` / `CreateAnchored` / `ScheduleExists` /
  `AddScheduleAnchored` (threading `existing.StartDate` as the schedule's
  default-effective-date argument, matching the class's own stored start
  date rather than the file's, which is what the route handler's
  `AddSchedule` already does via its `GetByID` lookup).
- `applyStudent(ctx, actor authctx.Scope, owner authctx.OwnerAnchor,
  enrollAnchor authctx.Anchor, ...)`: `contacts.FindIDByPhone` /
  `students.FindIDByName` dedupe reads run under `actor`; `contacts.CreateAnchored`
  / `students.CreateAnchored` writes run under `owner`;
  `enrollments.FindByStudentAndClassAnchored` /
  `enrollments.CreateAnchored(ctx, actor, enrollAnchor, ...)` run under
  `enrollAnchor` with `actor` carried through for event attribution.
- Doc comments on `apply`, `anchorFor`, `applyClass`, `applyStudent`, and the
  `MemberDirectory`/four-writer-interface block rewritten to describe the new
  actor/owner/anchor shape (the old "server-side owner scope" / "IsOwner
  stays false" language was stale).

`grep -rn 'IsOwner:\s*true' internal --include='*.go' | grep -v _test.go | grep -v features/centers/`
returns empty — no stray owner-bypassing literal remains outside tests and
`centers/`.

## New / extended tests (TDD)

1. **Imports integration, member-run anchoring + audit actor** — extended
   `TestGrantedMemberImportAnchorsEverythingOnTheOwner`
   (`internal/features/imports/integration_test.go`) rather than adding a
   sibling, since the existing test already built the exact fixture (owner +
   two teachers, one member holding `imports.run`) the assertion needed.
   Added:
   - a `bus := events.NewSync()` wired into the fixture's `enrollmentsSvc`,
     capturing every `StudentEnrolled` event into `roster.enrolled`;
   - assertions that classes stay anchored on their own workbook teacher
     (`nam`/`lan`) even though `nam` (a member) ran the whole import;
   - assertions that every enrollment row's `teacher_id` is one of the two
     class teachers (never the importing member unless that also happens to
     be the class's own teacher);
   - assertion that all three captured `StudentEnrolled` events have
     `ActorID == nam` (the importing member), proving the audit trail credits
     the actor, not the row's anchor.
   This test failed to compile before `apply.go`/`service.go` were rewritten
   (old `apply` signature), then failed to compile again with the old
   `CenterOwner`-based `MemberDirectory` fake — confirming it exercises the
   real code path, not a fake shortcut.
2. **Compile-time `CreateAnchored` signature test** — added to both
   `contacts/service_test.go` and `students/service_test.go`:
   `TestCreateAnchoredRequiresAnOwnerAnchor`, assigning the method value to a
   variable typed on the exact function signature
   (`func(context.Context, authctx.OwnerAnchor, CreateRequest) (*Row, error)`).
3. Existing owner-path test expectations preserved: no test assertions were
   weakened; `TestFindByStudentAndClassStaysWithinTheAnchorTeacher` and
   `TestImportCommitCreatesEveryEntity` (etc.) were read but left untouched
   where their behavior didn't change, or extended in-place (fakes, call
   signatures) where the contract genuinely changed shape.

## Verification (commands + results)

Run in this order, per the mandate — never `./...` for tests:

```
gofmt -l .                                            → (empty, all formatted)
go build ./...                                         → clean
go vet -tags=integration ./...                         → clean
go test ./internal/features/imports/...                → ok   (0.163s)
go test ./internal/features/contacts/...                → ok   (0.015s)
go test ./internal/features/students/...                → ok   (0.013s)
go test -tags=integration -count=1 ./internal/features/imports/...      → ok (5.947s)
go test -tags=integration -count=1 ./internal/features/contacts/...     → ok (6.528s)
go test -tags=integration -count=1 ./internal/features/students/...     → ok (6.125s)
go test -tags=integration -count=1 ./internal/features/classes/...      → ok (6.532s)
go test -tags=integration -count=1 ./internal/features/centers/...      → ok (10.091s)
go test -tags=integration -count=1 ./internal/features/enrollments/...  → ok (6.936s)
go test -count=1 -run TestScopeLiteralsOnlyWhereResolved ./internal/features/  → PASS (0 failures; was 2 before this slice)
```

`internal/server/router.go`'s build error
(`classesSvc does not implement imports.ClassWriter`), surfaced mid-session
when only `classes` had been migrated, resolved on its own once `imports`'
writer interfaces were rewritten to match `classes.Service`'s real method
set — no direct edit to `router.go` was needed or made.

## Files changed (this slice's ownership)

| File | Nature of change |
|---|---|
| `internal/features/centers/dashboard.go` | Already-complete `targetScope`→`viewAs` rename (verified, not re-edited) |
| `internal/features/classes/repository.go` | `Anchor`-typed `FindActiveByName`/`ScheduleExists`, new `anchored`/`anchoredSchedules` |
| `internal/features/classes/service.go` | `CreateAnchored`, `AddSchedule`/`AddScheduleAnchored` shared core, `Anchor`-typed finders |
| `internal/features/classes/service_test.go` | Fakes + call sites updated to `Anchor` |
| `internal/features/contacts/service_test.go` | Added `TestCreateAnchoredRequiresAnOwnerAnchor` |
| `internal/features/enrollments/repository.go` | `anchored`, `GetByIDAnchored`, `FindByStudentAndClassAnchored` |
| `internal/features/enrollments/service.go` | `create` shared core, `CreateAnchored`, `foundOrNot`, `FindByStudentAndClassAnchored` |
| `internal/features/enrollments/service_test.go` | Fakes for the two new Anchored methods |
| `internal/features/imports/apply.go` | `apply`/`anchorFor`/`applyClass`/`applyStudent` rewritten onto actor/owner/anchor |
| `internal/features/imports/fakes_test.go` | `fakeRoster` + adapters rewritten to the four Anchored writer interfaces |
| `internal/features/imports/handler_test.go` | `countingDirectory.CenterOwner` → `ResolveOwnerAnchor` |
| `internal/features/imports/integration_test.go` | Event-bus wiring + extended member-anchoring/audit-actor assertions |
| `internal/features/imports/service.go` | `MemberDirectory`, four writer interfaces, `Import()` owner resolution |
| `internal/features/students/service_test.go` | Added `TestCreateAnchoredRequiresAnOwnerAnchor` |
| `seeds/seed.go` | One-line fix: `classesSvc.FindActiveByName(ctx, sc.Self(), className)` — downstream consumer of the changed public contract, not otherwise in scope |

## Open concerns

- **Pre-existing, not touched**: `students.Create` (the route-handler entry
  point, not `CreateAnchored`) does not itself carry an explicit `IsOwner`
  gate the way `contacts.Create` does — noted in a prior session as a
  pre-existing discrepancy outside this slice's scope. Flagging again rather
  than silently fixing it, since it wasn't part of the assigned spec and
  touching it would be an undisclosed scope change.
- `seeds/seed.go` was edited (one line) though not in the explicit file
  ownership list, because it's a direct downstream consumer of
  `classes.Service.FindActiveByName`'s changed signature and would otherwise
  break `go build ./...`. No other unlisted files were touched.
- Many other files show as modified in `git status` (attendance, billing,
  notifications, payments, sessions, statements, and parts of
  contacts/students/enrollments outside what's listed above) — these belong
  to other slices' work in this same multi-agent refactor and were not
  touched by this session.
