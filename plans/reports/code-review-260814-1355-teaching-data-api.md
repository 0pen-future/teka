# Code Review — Teaching data API

**Plan:** `plans/260814-1345-teaching-data-api/plan.md`
**Reviewer:** reviewer-teaching · 2026-08-14
**Verdict:** No blockers. 2 High, 4 Medium, 6 Low.

## Scope

- Uncommitted changeset on `master`: 23 modified files + untracked `apps/api/internal/features/teaching/` (10 files, 3544 LOC), `apps/api/migrations/000009_teaching.{up,down}.sql`, `apps/web/src/features/teaching/{api,schemas,hooks}/`, `apps/web/src/features/teaching/__tests__/teaching-handlers.ts`.
- Focus: tenancy leaks, member-vs-owner authorization, state-machine holes, optimistic-cache divergence, tri-state marks upsert.

## Evidence re-run (not taken on trust)

| Check | Result |
|---|---|
| `go vet ./internal/features/teaching/...` | clean |
| `go test ./internal/features/teaching/...` | ok |
| `go test -tags integration` (7 teaching tests, real Postgres via testcontainers) | ok, 5.16s |
| `npx vitest run src/features/teaching src/layouts` | 11 files, 82 tests passed |
| `npx tsc --noEmit` | clean |
| `npx eslint src/features/teaching src/layouts` | 0 findings |
| swagger old vs new operation/definition diff | **12 ops added, 0 removed, 0 definitions removed** |

The swagger comparison is the decisive backwards-compatibility check: the 1628-line `docs.go`/`swagger.json` diff is purely additive, so accepted decision #6 (stale committed spec) holds and no existing operation or definition was dropped.

## Authorization and tenancy — verified sound

Traced the full gate chain rather than trusting the package comments:

- `teaching.Service.resolveClass` → `classes.Service.Get` → `classes.gormRepository.scoped` (`classes/repository.go:59-66`) which adds `classes.teacher_id = ?` for non-owners. Same shape for `sessions.gormRepository.scoped` (`sessions/repository.go:70-77`). A member therefore gets 404, not 403, for a peer's class — the correct non-enumerating shape.
- `requireClassTeacher` / `requireSessionTeacher` (`service.go:662-689`) block the owner from writing another teacher's content while `resolveClass` still lets them read it. Matches accepted decision #8.
- Every repository query carries `center_id = sc.CenterID`. The one unqualified join (`LEFT JOIN class_curricula ON class_curricula.class_id = lesson_plans.class_id`, `repository.go:177`) is safe because `class_curricula.class_id` is `UNIQUE` on a globally unique UUID, and the driving `lesson_plans` row is already center-filtered.
- `TeacherNames` has no center filter by design; the ids come only from `submitted_by` on rows the caller already resolved through their own scope.

Proven end-to-end by `TestCrossCenterTeachingIsNotFound`, `TestMemberGets403OnOwnerActions`, `TestPeerHiddenOwnerReadsButNeverEdits` — all run against real Postgres in this review.

## High

### H1 — Teaching write DTOs have no size bounds, deviating from repo convention

`apps/api/internal/features/teaching/dto.go`

None of the teaching request DTOs bound input size:

- `PutCurriculumRequest.Lessons []string` (dto.go:26-29) — unbounded array, unbounded element length, straight into a JSONB column.
- `SavePlanRequest.Goal/Activities/Homework` (dto.go:52-57) — unbounded TEXT and JSONB.
- `PutNoteRequest.Body` (dto.go:108-110), `MarkEntryRequest.PersonalNote` (dto.go:126), `ReviewRequest.Comment` (dto.go:62-64) — unbounded TEXT.

The repository convention is explicitly the opposite: `classes/dto.go:31-35` uses `min=1,max=100` and `required,min=1,dive`; `attendance/dto.go:23` uses `omitempty,max=500`. This is authenticated-only, so it is not an anonymous DoS, but any member can write an arbitrarily large curriculum or plan into a row that **every subsequent classbook read for that class loads in full**, and `GET /classes/:id/lesson-plans` returns all of them at once.

Fix: add `max=` tags matching the sibling features, e.g. `Lessons []string \`json:"lessons" binding:"omitempty,max=200,dive,max=200"\``, `Goal string \`json:"goal" binding:"omitempty,max=2000"\``, `Body string \`json:"body" binding:"omitempty,max=2000"\``.

### H2 — A failed save silently discards the user's typed draft

`apps/web/src/features/teaching/components/session-detail-panel.tsx:100-128`

Both save handlers clear local draft state and fire the success toast synchronously, immediately after `mutate()` returns:

```ts
saveNoteMutation.mutate({ sessionId: session.id, body: noteDraft });
setNoteDraft(null);
hvToast(`Đã lưu nhận xét buổi ${sessionLabel} — ${classTitle}`);
```

`useSaveSessionNote`/`useSaveMarks` write the cache optimistically in `onMutate` and, on failure, `onError` invalidates `teachingKeys.classMarks(classId)` (`use-teaching-mutations.ts:118-122, 183-187`). The refetch restores server truth, the draft was already dropped, and the user's text is gone. They see "Đã lưu nhận xét…" followed by "Không lưu được nhận xét — vui lòng thử lại", with nothing to retry from.

This is genuinely new behavior: the old `updateTeachingState` write could not fail. It will not show up in tests because msw always succeeds.

Fix: move the toast and the draft reset into the mutation's `onSuccess`, or keep the draft on error (`onError: (_e, vars) => setNoteDraft(vars.body)`).

## Medium

### M1 — Concurrent first-save of the same plan returns 500 instead of 409

`service.go:141-166` reads with `GetPlan`, then `writePlan` routes to `CreatePlan` when `CreatedAt` is zero (`service.go:521-532`). Two saves racing on a never-saved `(class_id, lesson_index)` both read nil; the loser violates `uq_lesson_plans_class_lesson` and the error goes out through `apperror.Internal` as a 500. The repository doc comment already names this ("A concurrent duplicate save loses to `uq_lesson_plans_class_lesson` and surfaces as an error", `repository.go:54-56`) but nothing translates it.

Fix: either make `CreatePlan` an `ON CONFLICT (class_id, lesson_index) DO UPDATE` like `UpsertCurriculum`, or translate `gorm.ErrDuplicatedKey` to `apperror.Conflict` so the web's existing 409 handler (which invalidates and re-reads) does the right thing.

### M2 — `useReviewQueue` is a dead export

`apps/web/src/features/teaching/hooks/use-review-queue.ts:13`. Grepped all of `apps/web/src`: the only consumer of the review-queue endpoint is `usePendingPlanCount`, used once by `dashboard-layout.tsx:55`. `useReviewQueue` has no caller, no test, and is not re-exported from `features/teaching/index.ts`.

This is a direct miss against the plan's success criterion "no dead exports". Accepted decision #4 makes it permanent, not temporary: `/lesson-plans` deliberately derives from per-class curriculum+plans queries, so nothing will ever consume this hook. Delete it (YAGNI) — `usePendingPlanCount` already owns the query key.

### M3 — Optimistic cache writes don't cancel in-flight reads

`use-teaching-mutations.ts:114` and `:163`. Both `onMutate` callbacks call `setQueryData` without first `await queryClient.cancelQueries({ queryKey: teachingKeys.marks(classId, month) })`. A month read already in flight (refetch-on-focus, per validation-log answer 3) that resolves after the optimistic write will overwrite it with pre-write data, and the input reverts until `onSuccess` lands. The window is small, but cancelling first is the standard React Query optimistic recipe and costs one line.

### M4 — The roster gate blocks clearing marks for a since-unenrolled student

`service.go:341-355` rejects any entry whose `student_id` is not in `roster.ActiveOn(classID, session.SessionDate)`. That is right for *setting* a mark. It is wrong for *clearing* one: if a student's enrollment is later ended or removed, their existing `session_marks` row becomes uncorrectable — the teacher's blur-save from `/records/:studentId` 422s with "student … was not on the session's roster" and the stale score keeps feeding `classbook-stats` and the CSV export.

Fix: skip the roster check for entries that resolve to both-fields-null (a delete), or accept a student who already has a mark row for that session.

## Low

1. **Latent nil deref in `applyTransition`** (`service.go:472-488`). `plan.Status = next` runs on a possibly-nil `plan`; it is safe today only because no `none → X` entry exists for submit/approve/request-redo/reopen in `planTransitions`. A future table edit turns this into a panic. Add `if plan == nil { return nil, transitionConflict(StatusNone, action) }`.
2. **`PutNote` stores an untrimmed body** (`service.go:306-321`) while using the trimmed form to decide deletion, so `"  x  "` persists with its padding. Everything else in the package goes through `trimmedPtr`/`cleanLines`.
3. **Unqualified `SELECT *` on joined month reads** (`repository.go:185-203`). Neither `ListNotesForClassMonth` nor `ListMarksForClassMonth` sets a `Select`, so the join pulls `class_sessions`' `id`, `center_id`, `created_at`, `updated_at` alongside the teaching table's. The fields actually read back (`session_id`, `student_id`, `score`, `personal_note`, `body`) are unambiguous so responses are correct, but `SessionMark.ID` scans the session's id. Qualify with `.Select("session_marks.*")`, as `withClassName` does at `sessions/repository.go:82-86`.
4. **`lesson_plans.submitted_by` has a bare FK** (`000009_teaching.up.sql:51`) — `REFERENCES teachers(id)` with no `ON DELETE`, unlike every other FK in the migration. A hard teacher delete would be blocked by plan rows. Harmless while teachers soft-delete; `ON DELETE SET NULL` would match the null-safe reading the review queue already performs.
5. **Cross-feature test-internal import**: `layouts/__tests__/dashboard-layout.test.tsx` imports `@/features/teaching/__tests__/teaching-handlers`. Every other stateful-handler consumer stays inside its own feature (all three `roster-handlers` importers). Pragmatic here, but it makes a test-internal module a de facto shared surface.
6. **Month bounds compare a UTC timestamp against a DATE column** (`service.go:263-273`). `time.Parse("2006-01", month)` yields UTC midnight, and a non-UTC Postgres session `TimeZone` would shift the month's first/last day. Not new — `sessions/repository.go:116` has the identical exposure through `parseDate` — so this is a repo-wide note, not a defect in this changeset.

## Non-finding (accepted decision #4, threshold noted)

`/lesson-plans` now runs three `useQueries` fan-outs over the active class list — sessions, curriculum, and plans — so page load issues 3N requests where it previously issued N (`lesson-plans-page.tsx:33-52`). With `per_page: 100` that is a 300-request ceiling. The shared-cache benefit with the classbook is real and the decision is accepted, so this is not raised as a finding; recording the threshold for whoever revisits it if active-class counts grow.

## Success Criteria

| # | Criterion | Status |
|---|---|---|
| 1 | Migration applies/rolls back; FK guards + partial pending index; `migrations_test.go` covers it | **Met** — `TestTeachingTablesIntegrity` asserts the membership guard, both uniques, the status CHECK rejecting `'none'`, the negative-index CHECK, the 0–10 score CHECK, and cascade-on-class-delete across all four tables |
| 2 | 12 endpoints; 409 on state-machine violations; 403 for members; swagger regenerated | **Met** — 12 swagger ops added and 0 removed; `TestPlanTransitionMatrix`, `TestMemberGets403OnOwnerSurfaces`, `TestMemberGets403OnOwnerActions` |
| 3 | Four screens behave identically, typing stays instant | **Met with caveat** — 82 web tests green; the failure path is new behavior (H2) |
| 4 | Teacher submits → owner sees dot + queue → approve/request-redo round-trips | **Met at the API layer.** `TestLessonPlanReviewLoop` and `TestPeerHiddenOwnerReadsButNeverEdits` prove the cross-account loop against real Postgres. Note the web test named "review loop across pages" re-signs in as the *same* teacher (`lesson-plans-page.test.tsx`), so it proves API round-trip, not account separation — consistent with accepted decision #7, but the Go tests are what actually carry this criterion |
| 5 | localStorage removed; no dead exports; transition helper retained | **Partial** — persistence and `resetTeachingStoreForTests` are gone, `transitionLessonPlanStatus` retained for button gating, but `useReviewQueue` is dead (M2) |
| 6 | Web suite, api suite, eslint, tsc green | **Met** for everything re-run here (table above) |

## No regressions found in the named touchpoints

- `classbook-stats.ts` / `student-stats.ts` untouched; they still take `TeachingState["sessionScores"]` and the hooks rebuild exactly that shape (`use-class-marks.ts:39-53`).
- `sessions`/`roster`/`attendance` consumed only through the new read-only consumer interfaces (`ClassSource`, `SessionSource`, `RosterSource`); no existing service signature changed.
- `dashboard-layout.tsx` — `usePendingPlanCount()` lost its `centerId` argument. Internal to the app, single call site, and the new test asserts a member never issues the request at all.
- msw two-layer convention followed: empty defaults in `test/msw/handlers.ts:425-435`, stateful handlers in the feature's `__tests__/teaching-handlers.ts`.

## Recommended order

1. H1 — add `max=` binding tags across the teaching DTOs.
2. H2 — move toast + draft reset into `onSuccess` (or restore the draft in `onError`).
3. M1 — translate the duplicate-key race to 409, or upsert.
4. M2 — delete `useReviewQueue`.
5. M4 — let a clearing entry bypass the roster gate.
6. M3, then the Low items as convenient.

## Unresolved questions

1. Should a mark for a student whose enrollment ended remain editable (M4)? This is a product call — either "history is frozen with the roster" or "the teacher can always correct their own record".
2. Is `useReviewQueue` intended for a later screen? If yes it should land with that screen, not before it (M2).

## Resolution (2026-08-14, post-review fix pass)

| Finding | Outcome |
|---------|---------|
| H1 | Fixed — `binding:"max=..."` tags on `PutCurriculumRequest` (100 lessons × 200 chars), `SavePlanRequest` (goal/homework 2000, 50 activities × 500, file_name 255), `ReviewRequest` (1000), `PutNoteRequest` (2000); `Optional`-typed `personal_note` (≤1000 runes) and batch size (≤100 entries) bounded in `validateMarkEntries` since the custom unmarshaller bypasses gin's validator. Covered by the extended `TestMarksBatchValidation`. |
| H2 | Fixed — `session-detail-panel.tsx` moves the success toast + draft reset into the mutate-level `onSuccess` for both note and scores; `student-record-page.tsx` moves its success toast likewise. New test: "keeps the typed note draft editable when the save fails" (msw 500 → danger toast, draft + "Chưa lưu" survive). |
| M1 | Fixed — `writePlan` maps `gorm.ErrDuplicatedKey` from the first-save race to `apperror.Conflict` (409), riding the client's existing reload path. New test: `TestSavePlanConcurrentCreateConflict`; fake repo now returns `gorm.ErrDuplicatedKey` + gained a `createPlanErr` hook. |
| M2 | Fixed — dead `useReviewQueue` export deleted; `usePendingPlanCount` (sole consumer of the endpoint) keeps the query and its owner gate. Plan success criterion 5 now fully met. |
| M3 | Fixed — both optimistic mutations `await queryClient.cancelQueries(...)` on the month key before writing the cache. |
| M4 | Fixed — user chose "always-correctable": `PutMarks` now loads existing rows before the roster gate, which only guards NEW-row creation; an existing row stays editable and clearable after the enrollment ends (`service.go`, swagger description updated, `TestMarksExistingRowSurvivesUnenrollment`). |
| Low items | Deferred — recorded here; none blocks ship (the nil-deref is unreachable under the current transition table, the SELECT-scan quirk does not affect responses). |

Post-fix verification: web 336/336 (55 files), tsc clean, eslint 0 errors / 4 pre-existing warnings; `go vet ./...`, `go test ./...`, integration `./internal/features/teaching` + `./migrations` all green; swagger regenerated after the DTO tag change.
