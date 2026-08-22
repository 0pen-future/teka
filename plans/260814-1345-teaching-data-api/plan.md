---
title: "Teaching data API"
description: "Backend for teaching data (curriculum, giáo án, điểm, nhận xét): Postgres schema + Go API feature, then swap the web teaching store to API-backed hooks without breaking the v2 teaching screens"
status: completed
priority: P1
effort: "5d"
tags: [api, database, web, teaching]
created: 2026-08-14
relatedTo: [260813-2128-prototype-v2-teaching-screens]
---

# Teaching data API

## Overview

The four v2 teaching screens (`/classbook`, `/records`, `/records/:studentId`, `/lesson-plans`) currently keep teaching-only data in a device-local store (`apps/web/src/features/teaching/lib/teaching-store.ts`: in-memory + `localStorage`, keyed per center name). This plan moves that data to the backend:

1. **Database design** — new Postgres tables for curriculum, lesson plans (giáo án + review state), session notes (nhận xét buổi), and per-student session marks (điểm + ghi chú riêng), following the center-scoping FK-guard pattern of migration `000007_centers`.
2. **API design** — a new `teaching` feature package in `apps/api` (gin, dto/handler/service/repository/routes layout) exposing class-scoped reads and per-action writes, with the lesson-plan state machine enforced server-side and owner-only review actions.
3. **Web swap** — replace the local store with React Query hooks that return the **same `TeachingState`-shaped slices** the stats libs and components already consume, so the UI does not change.

Supersedes decision #1 of plan `260813-2128-prototype-v2-teaching-screens` ("client-side local teaching store") — that plan is completed; this one lifts its accepted UI-first trade-off.

## Decisions (user-approved 2026-08-14)

| # | Decision | Rationale |
|---|----------|-----------|
| 1 | **Fresh start** — no import of existing localStorage teaching data. | Local data is UI-first test data, non-authoritative. No import endpoint, no migration flow. |
| 2 | **File giáo án stays metadata-only** (`file_name TEXT`). | Matches current UI exactly; real upload (object storage, limits, scanning) is a later plan if the need materializes. |

Derived decisions (from repo evidence, revisit only with new evidence):

- **Curriculum stored as `lessons JSONB` (ordered array of titles) + `current_index`**, one row per class — mirrors the UI editor (whole-list replace) and the store shape 1:1. Normalized `curriculum_lessons` rows add FK bookkeeping without a consumer; the review queue can extract a title with a JSONB index. Trade-off: no per-lesson FK integrity for lesson plans — accepted, the prototype semantics key plans by `(class_id, lesson_index)` and index-shift on curriculum edit is existing, understood behavior.
- **`session_marks` merges score + personal note** into one row per `(session_id, student_id)` — same key, one upsert path, one read per class/month. Distinct from `attendance_records.note` (attendance semantics).
- **`status='none'` is the absence of a row** — DB enum is `draft|pending|approved|redo`; the web adapter maps missing → `"none"`. No soft delete on teaching tables (non-financial, replaceable data; billing-adjacent tables keep theirs).
- **`SESSION_COST_VND` stays a web constant** — still no backend cost setting; out of scope, unchanged.

## Database design

New migration `000009_teaching` (up/down). All tables follow the `000007_centers` integrity pattern: `center_id` NOT NULL, composite FK to the parent's `(id, center_id)` unique, and the FK guard `(teacher_id, center_id) → center_members` keeping "the row's teacher is/was a member of the row's center".

```
class_curricula                          lesson_plans
  id            UUID PK                    id            UUID PK
  class_id      UUID NOT NULL UNIQUE       class_id      UUID NOT NULL
  teacher_id    UUID NOT NULL              lesson_index  INT  NOT NULL CHECK (>= 0)
  center_id     UUID NOT NULL              teacher_id    UUID NOT NULL
  lessons       JSONB NOT NULL DEF '[]'    center_id     UUID NOT NULL
  current_index INT  NOT NULL DEF 0        goal          TEXT NOT NULL DEF ''
  created_at/updated_at TIMESTAMPTZ        activities    JSONB NOT NULL DEF '[]'
                                           homework      TEXT NOT NULL DEF ''
session_notes                              file_name     TEXT
  session_id  UUID PK                      status        VARCHAR(20) NOT NULL
  teacher_id  UUID NOT NULL                              CHECK IN (draft,pending,approved,redo)
  center_id   UUID NOT NULL                redo_note     TEXT
  body        TEXT NOT NULL                owner_comment TEXT
  created_at/updated_at                    submitted_by  UUID NULL → teachers(id)
                                           submitted_at  TIMESTAMPTZ
session_marks                              created_at/updated_at
  id            UUID PK                    UNIQUE (class_id, lesson_index)
  session_id    UUID NOT NULL
  student_id    UUID NOT NULL
  teacher_id    UUID NOT NULL
  center_id     UUID NOT NULL
  score         NUMERIC(4,1) NULL CHECK (0 <= score <= 10)
  personal_note TEXT NULL
  created_at/updated_at
  UNIQUE (session_id, student_id)
```

Key constraints & indexes (exact FK/unique names verified against `000007` in Phase 1):

- `class_curricula.class_id`, `lesson_plans.class_id` → `classes(id, center_id)`; `session_notes.session_id`, `session_marks.session_id` → `class_sessions(id, center_id)`; `session_marks.student_id` → `students(id, center_id)`; all `(teacher_id, center_id)` → `center_members`.
- `idx_lesson_plans_pending ON lesson_plans(center_id) WHERE status = 'pending'` — the owner review queue + nav-dot count query.
- ~~`idx_session_marks_session ON session_marks(session_id)`~~ — **not implemented**: the `UNIQUE (session_id, student_id)` constraint already backs an index leading with `session_id`, so a separate index would be redundant.
- `session_notes` is 1:1 with the session → `session_id` is the PK (no surrogate id).

**Scale note:** all hot reads are center-scoped and month-bounded (a class ≈ tens of students, a month ≈ tens of sessions), so client-side stat computation stays cheap and no aggregate SQL is needed. The pending partial index keeps the owner nav-dot O(pending) regardless of history size.

## API design

New feature package `apps/api/internal/features/teaching/` (dto, errors, handler, model, repository, routes, service + `_test` files), mounted like siblings behind `requireAuth, resolveScope` — `authctx.Scope{TeacherID, CenterID, IsOwner}` is the authorization input; owner-only actions return `apperror.Forbidden` in the service, matching `centers`.

| Method & path | Who | Purpose |
|---|---|---|
| `GET  /classes/:id/curriculum` | class teacher, owner | `{lessons: string[], current_index}`; 200 with empty default when no row |
| `PUT  /classes/:id/curriculum` | class teacher | Whole replace (matches editor modal); upsert row |
| `GET  /classes/:id/lesson-plans` | class teacher, owner | All plans of the class (course view + teacher chips) |
| `PUT  /classes/:id/lesson-plans/:index` | class teacher | Save: upsert content; status per state machine (`none/draft→draft`, `redo` stays `redo`) |
| `POST /classes/:id/lesson-plans/:index/submit` | class teacher | `draft|redo → pending`; stamps `submitted_by/submitted_at` |
| `POST /classes/:id/lesson-plans/:index/approve` | owner | `pending → approved` |
| `POST /classes/:id/lesson-plans/:index/request-redo` | owner | `pending → redo`; body `{comment}` **required** (UI requires it) |
| `POST /classes/:id/lesson-plans/:index/reopen` | owner | `approved|redo → pending` |
| `GET  /teaching/review-queue` | owner | Pending plans across center: class name, teacher name, lesson title (from curriculum JSONB), `submitted_at`. Also the nav-dot source (`length`) — **implementation note:** ended up feeding only the nav-dot count (`usePendingPlanCount`, owner-gated); the `/lesson-plans` page table itself is derived from per-class curriculum+lesson-plans queries (shared React Query cache with classbook) so it can show all statuses and support reopen, not just pending |
| `GET  /classes/:id/marks?month=YYYY-MM` | class teacher, owner | Batch read for the month's sessions: session notes + all `session_marks` rows |
| `PUT  /sessions/:id/note` | session teacher | Upsert whole-class nhận xét `{body}`; empty body deletes |
| `PUT  /sessions/:id/marks` | session teacher | Batch upsert `[{student_id, score?, personal_note?}]`; null field clears, both-null row deletes |

Contract rules:

- **State machine is server authority.** The service ports `transitionLessonPlanStatus` (see `teaching-store.ts:82-99`) exactly — illegal action → `409 Conflict` (`apperror`), including the subtle rules: save from `redo` keeps `redo`; reopen from `redo` is the owner withdrawing their request.
- Teacher scoping: writes require the class/session belongs to the caller (`teacher_id` match via composite-FK-backed queries); owner has center-wide **read** everywhere plus the three review actions — owner does not edit content, scores, or notes of another teacher's class (matches UI capabilities).
- Validation: score `0–10` step `0.1` (mirror UI input), `lesson_index < len(curriculum.lessons)` on plan writes, response envelope + error shapes per `internal/shared/response` / `apperror`.
- Swagger annotations on handlers; regenerate `apps/api/docs`.

## Web swap — the no-break-UI strategy

The stats libs (`classbook-stats.ts`, `student-stats.ts`) and components consume **`TeachingState`-shaped record maps** (`sessionScores: Record<sessionId, Record<studentId, number>>` etc.), not the store itself. The swap therefore:

1. Adds `features/teaching/api/teaching-api.ts` + zod schemas (roster feature conventions) and React Query hooks that **assemble those exact slice shapes** from `GET /classes/:id/marks` and `GET /classes/:id/lesson-plans` — libs and presentational components stay untouched.
2. Replaces per-keystroke store writes with **debounced mutations + optimistic cache updates** so typing stays instant (current behavior).
3. Keeps `transitionLessonPlanStatus` client-side only for button enable/disable; server is authority, `409` invalidates the query.
4. `usePendingPlanCount` (nav dot) becomes a React Query select over the review-queue query — `Object.is`-stable count, owner-only, same as today's semantics. **Implementation deviation:** the review-queue query ended up scoped to only this nav-dot count; the `/lesson-plans` review page itself reads per-class curriculum+lesson-plans queries (shared cache with classbook) instead of the review-queue endpoint, because it needs all statuses and reopen, not just pending.
5. Deletes localStorage persistence; `useCenterContext`'s name-as-centerId workaround becomes irrelevant (server resolves scope) but its `isOwner` gating stays.

MSW handlers back all new endpoints in web tests; existing test expectations (screens, flows, a11y) must pass with handler-provided data instead of store seeding.

## Goals

| # | Goal | Priority |
|---|------|----------|
| 1 | Teaching data (curriculum, giáo án + review loop, điểm, nhận xét) persisted in Postgres, center-scoped, state machine server-enforced | P1 |
| 2 | API follows existing feature-package, scoping, error, and swagger conventions; owner review actions owner-only | P1 |
| 3 | All four teaching screens work unchanged on API data — cross-device now, no UI/UX changes, no component contract changes | P1 |
| 4 | No regressions: web suite + api suite green, eslint/tsc clean, `go vet`/tests clean, migration up/down tested | P1 |

## Phases

| # | Phase | Status |
|---|-------|--------|
| 1 | [Phase 1: DB migration — teaching tables](./phase-01-start.md) | Done |
| 2 | [Phase 2: API teaching feature: curriculum & lesson plans](./phase-02-api-teaching-feature-curriculum-lesson-plans.md) | Done |
| 3 | [Phase 3: API teaching feature: session notes & marks](./phase-03-api-teaching-feature-session-notes-marks.md) | Done |
| 4 | [Phase 4: Web: teaching API client & classbook swap](./phase-04-web-teaching-api-client-classbook-swap.md) | Done |
| 5 | [Phase 5: Web: records, review queue & nav dot](./phase-05-web-records-review-queue-nav-dot.md) | Done |
| 6 | [Phase 6: Verification & consistency](./phase-06-verification-consistency.md) | Done |

Dependency shape: 1 → (2, 3); 2+3 → 4; 4 → 5; 6 last. Phases 2 and 3 are independent of each other (separate endpoints, shared migration) but share the feature package scaffold — Phase 2 creates it, Phase 3 extends it, so run 2 before 3.

## Success Criteria

- [x] Migration `000009_teaching` applies and rolls back cleanly; FK guards + partial pending index in place; `migrations_test.go` covers it.
- [x] All 12 endpoints implemented per the contract table; state-machine violations return 409; owner-only actions return 403 for members; swagger regenerated.
- [x] `/classbook`, `/records`, `/records/:studentId`, `/lesson-plans` render and behave identically to the current store-backed screens (same components, same flows), backed by API data; typing in score/note inputs stays instant.
- [x] Teacher submits → owner sees pending dot + queue on another account/device → approve/request-redo round-trips (the loop localStorage could never do cross-device). (Verified via the msw-backed cross-account test — the lesson-plans-page test round-trips teacher submit → owner approve through the shared msw store — because the local docker stack is operator-managed and `.env` is absent; phase-06's own risk note allows this downgrade. Live-stack demo remains outstanding.)
- [x] `teaching-store.ts` localStorage persistence removed; no dead exports; state-machine helper retained where the UI needs button gating.
- [x] Full web suite, api suite (`go test ./...`), eslint, tsc all green.

## Open questions

None.

## Validation Log

### Session 1 — 2026-08-14
**Trigger:** User selected "/ak:plan validate" at post-plan handoff.
**Questions asked:** 4 (plus 2 scope questions answered pre-plan: fresh start, file metadata-only)

### Verification Results
- **Tier:** Full (6 phases)
- **Claims checked:** 18 (parent unique constraints, feature-package layout, middleware/scope symbols, `apperror.Forbidden`, response/testutil packages, swagger annotations, store shape + transitions source lines, stats-lib input shapes, consumer file list, msw setup, `hv-toast`, roster api layout, `use-month-sessions`, `idx_class_sessions_class_date`)
- **Verified:** 18 | **Failed:** 0 | **Unverified:** 0
- Notable exact evidence: `uq_classes_cid`, `uq_class_sessions_cid`, `uq_students_cid` (`000007_centers.up.sql:163-167`); msw = `src/test/msw/{server,handlers}.ts` + per-feature `__tests__/*-handlers.ts`; swaggo `@Summary/@Router` on handlers (`attendance/handler.go:49-92`).

#### Questions & Answers
1. **[Authorization]** Owner write access to other teachers' classes at the API layer?
   - **Answer:** Chỉ đọc — owner reads center-wide + the 3 review actions only; content writes stay class-teacher-only. Narrower is easier to widen later.
2. **[Authorization]** Owner's own lesson plans — who approves?
   - **Answer:** Owner tự duyệt được — no higher approval tier exists; matches current store behavior. No self-approve guard in the service.
3. **[UX/Architecture]** Pending dot + review queue freshness?
   - **Answer:** Refetch on focus (React Query defaults) — no polling.
4. **[UX/Architecture]** Typing durability under debounce (~500ms + flush on blur/unmount)?
   - **Answer:** Chấp nhận — rare hard-tab-close may lose final keystrokes; smooth typing preferred over per-keystroke sync.

#### Impact on Phases
- All four answers confirm existing plan assumptions — no design changes.
- Phase 1: parent constraint names pinned to verified `uq_*_cid` names.
- Phase 4: msw target pinned to per-feature `teaching-handlers.ts` + registration in `src/test/msw/handlers.ts`.

### Whole-Plan Consistency Sweep
- Files reread: plan.md, phase-01 … phase-06 (all written this session, decisions confirmed as assumed)
- Reconciled: 2 precision updates (phase-01 constraint names, phase-04 msw location); no contradictions introduced or found
- Unresolved contradictions: 0

<!-- slug: teaching-data-api -->
