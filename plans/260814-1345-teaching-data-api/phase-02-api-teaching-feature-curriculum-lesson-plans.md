---
phase: 2
title: "API teaching feature: curriculum & lesson plans"
status: completed
priority: P1
effort: "1.5d"
dependencies: [1]
---

# Phase 2: API teaching feature: curriculum & lesson plans

## Overview

Create the `teaching` feature package and implement curriculum CRUD, lesson-plan save/submit, the owner review actions (approve / request-redo / reopen), and the center-wide review queue.

## Requirements

- Functional: endpoints 1–9 of the plan.md contract table (`/classes/:id/curriculum`, `/classes/:id/lesson-plans[...]`, `/teaching/review-queue`).
- Non-functional: state machine enforced in the service (single transitions table, mirroring `teaching-store.ts:82-99` semantics); owner-only actions via `authctx.Scope.IsOwner` → `apperror.Forbidden`; illegal transition → `apperror` mapping to 409; swagger annotations.

## Architecture

- Package layout copies `attendance`/`centers`: `dto.go`, `errors.go`, `handler.go`, `model.go`, `repository.go`, `routes.go`, `service.go` + `handler_test.go`, `service_test.go`, `integration_test.go`. Routes registered in the server wiring next to sibling features, behind `requireAuth, resolveScope`.
- **Authorization matrix** (service layer):
  - class teacher (`class.teacher_id == scope.TeacherID`): read + write curriculum, save/submit plans;
  - owner (`scope.IsOwner`): read any class's curriculum/plans in the center, approve/request-redo/reopen, review queue;
  - everyone else in/outside center: 404-shaped not-found (match how sibling features hide cross-teacher rows — read one to confirm 403 vs 404 convention and follow it).
- **State machine**: `map[status]map[action]status` in `service.go` — the only transition source; save keeps `redo` in `redo`, submit stamps `submitted_by = scope.TeacherID`, `submitted_at = now()`. Request-redo requires non-empty `comment` (400 otherwise) and writes `owner_comment`+`redo_note` per the store's field usage — check `plan-editor-modal.tsx`/`plan-review-panel.tsx` to map which field feeds which UI slot before finalizing dto names.
- **Review queue query**: pending plans joined to `classes` (name), `teachers` (name), `class_curricula` (`lessons ->> lesson_index` as title; NULL-safe when curriculum shrank), ordered by `submitted_at`. Served from the partial index.
- Validation: `lesson_index` within current curriculum length on save/submit; whole-list curriculum PUT replaces `lessons` + clamps `current_index` into range.

## Related Code Files

- Create: `apps/api/internal/features/teaching/{dto,errors,handler,model,repository,routes,service}.go`
- Create: `apps/api/internal/features/teaching/{handler_test,service_test,integration_test}.go`
- Modify: server wiring that registers feature routes (locate via how `centers.RegisterRoutes` is called in `internal/server` or `internal/app`)
- Modify: `apps/api/docs/` (swagger regen)

## Implementation Steps

1. Read `centers` and `attendance` packages end-to-end (handler/service/repository + one integration test) to lock conventions: scope usage, error mapping, response envelope, test harness (`internal/testutil`).
2. Scaffold package + models + repository (sqlc/squirrel/raw — whatever siblings use; follow exactly).
3. Implement curriculum GET/PUT; empty-default GET (no row → `{lessons: [], current_index: 0}`).
4. Implement plan save/submit + owner actions with the transitions table; unit-test every legal and illegal transition (table-driven).
5. Implement review queue; integration tests: member forbidden, owner sees cross-teacher pending, title NULL-safe.
6. Swagger annotations + regenerate docs; `go vet ./... && go test ./...`.

## Success Criteria

- [x] Table-driven service test covers all 5×5 status×action combinations against the store's matrix (`none` = missing row).
- [x] Owner review actions 403 for members; teacher save on another teacher's class hidden per repo convention.
- [x] Request-redo without comment → 422 (validation error, not 400 — see deviation below); with comment → status `redo`, comment visible in GET.
- [x] Review queue returns class/teacher/lesson-title/submitted_at; empty when nothing pending.
- [x] `go test ./...` green, swagger updated.

## Risk Assessment

- **Semantic drift from the web state machine** — mitigated by porting the exact transitions table and testing the full matrix; Phase 4 then relies on server truth.
- **Field-mapping ambiguity (`redo_note` vs `owner_comment`)** — resolved by reading the two web components before freezing DTOs (step 1); do not guess.

## Completion notes

- Request-redo with a blank comment returns `422` (validation error) rather than the `400` the plan originally called out — matches the repo's existing convention of using 422 for request-body validation failures.
- Reopen clears both `redo_note` and `owner_comment` server-side (not just one), so a reopened plan starts review clean.
- Owner self-approve is allowed (no self-approve guard), per the plan's validated Q&A decision #2 — no higher approval tier exists.
