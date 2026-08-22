---
phase: 1
title: "API teacher handoff endpoint"
status: done
priority: P1
effort: "6h"
dependencies: []
---

# Phase 1: API teacher handoff endpoint

## Overview

Owner-only `PUT /api/v1/classes/:id/teacher` that reassigns a class to another
center member: class row + all its schedule rows + future planned sessions move;
held/cancelled and past planned sessions stay with the old teacher.

## Requirements

- Functional: owner submits `{"teacher_id": "<uuid>"}`; target must be an
  active member of the caller's center; idempotent no-op when target equals the
  current teacher; whole change in one transaction.
- Non-functional: no schema change; follow the `imports` coordinating-feature
  pattern so no feature touches a table it does not own; center advisory lock
  held during the write (same locker as imports) so a concurrent import cannot
  interleave.
- Boundary (validated 2026-08-21; mechanism corrected in code review): the
  future-session cutoff is **inclusive of today** by design — a late-day handoff
  moves today's still-planned session; if it already ran, the owner records
  attendance first (held sessions never move). "Today" is resolved in the old
  teacher's IANA timezone in Go and passed to the repository as a `notBefore`
  date (matching `ListPending`), **not** SQL `CURRENT_DATE` — which would resolve
  in the DB session's zone (UTC in deployment) and drift the boundary in the
  early-morning-VN window.
- Non-goal (validated 2026-08-21): no Zalo/in-app notification to either
  teacher on handoff.
<!-- Updated: Validation Session 1 - inclusive-today boundary + no-notification non-goal -->

## Architecture

New package `apps/api/internal/features/handoff` (mirrors `imports`):

- Consumer-defined interfaces (implemented by existing services, wired in
  `server/router.go` AFTER `sessionsSvc` exists — this sidesteps the
  classes↔sessions construction cycle at router.go:119/150):
  - `ClassReassigner` (impl `*classes.Service`): `GetByID`-equivalent scoped
    fetch + new method `ReassignTeacher(ctx, sc, classID, newTeacherID)` that
    updates `classes.teacher_id` and every `class_schedules.teacher_id` row of
    that class (both tables owned by classes).
  - `SessionReassigner` (impl `*sessions.Service`): new method
    `ReassignPlanned(ctx, sc, classID, oldTeacherID, newTeacherID)` — `UPDATE
    class_sessions SET teacher_id = $new WHERE class_id = $id AND status =
    'planned' AND session_date >= $notBefore` (table owned by sessions).
    `notBefore` is today resolved in the old teacher's timezone in Go (see the
    boundary note above), not SQL `CURRENT_DATE`.
  - `MemberChecker` (impl `*centers.Service`): new method
    `IsActiveMember(ctx, sc, teacherID) (bool, error)` built on the existing
    `ListMembers` repo path (same center-scoped source as `MemberIDsByPhone`).
  - Reuse `imports.Locker` semantics: either export the existing locker or add
    an identical `TryLockCenter`/`SetStatementTimeout` pair; do NOT duplicate
    lock keys — same center key as imports so the two features exclude each
    other.
- Service flow: `requireOwner(scope)` → fetch class via scope (404 if not in
  center) → target == current → return current state (no-op) → `IsActiveMember`
  else 422 "giáo viên này không thuộc trung tâm" → `WithinTx`: lock center,
  `ReassignTeacher`, `ReassignPlanned` → return updated class summary +
  `{moved_planned_sessions: n}`.
- Route: register under the existing auth middleware pair like
  `imports.RegisterRoutes`; path `PUT /classes/:id/teacher` (gin: same `:id`
  param name as classes' group avoids wildcard conflict).
- Swagger annotations per repo convention; run swagger generation to keep the
  drift check green.

## Related Code Files

- Create: `apps/api/internal/features/handoff/{service,handler,routes,dto}.go` + tests
- Modify: `apps/api/internal/features/classes/{service,repository}.go` (ReassignTeacher)
- Modify: `apps/api/internal/features/sessions/{service,repository}.go` (ReassignPlanned)
- Modify: `apps/api/internal/features/centers/service.go` (IsActiveMember)
- Modify: `apps/api/internal/server/router.go` (wire + register after sessionsSvc)

## Implementation Steps

1. Add `ReassignTeacher` to classes (service + repo; scoped update, both tables).
2. Add `ReassignPlanned` to sessions (service + repo; status+date filtered).
3. Add `IsActiveMember` to centers service.
4. Build `handoff` package: dto (request/response), service with owner gate +
   validation + tx + lock, handler, routes; unit tests with fakes (copy the
   `imports/fakes_test.go` style).
5. Wire in router.go; add integration test: seed class w/ schedules + one held
   + one past-planned + one future-planned session → handoff → assert exactly
   class/schedules/future-planned moved; assert 403 member, 422 non-member,
   no-op same teacher.
6. Regenerate swagger; `go build ./... && go test ./...` for apps/api.

## Success Criteria

- [ ] Integration test proves the move/stay split per approved semantics.
- [ ] 403 (member), 404 (foreign class), 422 (non-member target) covered.
- [ ] Concurrent-import exclusion via shared center lock key (unit-tested).
- [ ] Swagger drift check green; full API suite green.

## Risk Assessment

- **Gin route conflict** on `/classes/:id/teacher` vs classes' group — same
  param name; verify at server startup test (router builds in tests already).
- **Lock-key drift** between imports and handoff → single shared constant.
- **Billing edge:** future planned sessions moving mid-billing-period is
  by-design (approved); held sessions never move, so closed books never change.
