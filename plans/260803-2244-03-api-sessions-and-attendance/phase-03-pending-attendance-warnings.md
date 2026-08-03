---
phase: 3
title: "Pending Attendance Warnings"
status: completed
priority: P1
effort: "4h"
dependencies: [2]
---

# Phase 3: Pending Attendance Warnings

## Overview

PRD **R2**, third acceptance criterion: *"Given buổi học đã qua nhưng chưa điểm
danh, When giáo viên mở app, Then thấy cảnh báo ngay màn hình đầu."* A session
that has happened but has not been recorded is money that will silently go
uncollected — PRD user story 7: *"để tôi không dạy xong mà quên tính tiền."*

This phase exposes that feed. It is small, but it is the mechanism that
protects the North Star metric G4 (≥90% of sessions recorded within 24 hours):
without a visible reminder, the metric decays and, per the PRD, the product
becomes worthless regardless of how good the reports look.

The same query later gates period closing (PRD R4: *"có buổi chưa điểm danh
trong kỳ, Then hệ thống chặn chốt và chỉ rõ buổi nào"*). Plan 04 consumes it
rather than writing its own.

## Requirements

- `GET /api/v1/sessions/pending` returns the teacher's unconfirmed past
  sessions, newest first, with the class name, date, and expected student count.
- "Pending" means: `session_date` is in the past (in the teacher's timezone),
  `deleted_at IS NULL`, `attendance_confirmed_at IS NULL`, and `status` is
  either `held` or `planned`. Cancelled sessions are never pending.
- The endpoint reports a total count alongside the rows so the dashboard can
  render a badge without fetching everything.
- Optional `from`/`to` filtering so plan 04 can reuse it for a billing period.
- Teacher-scoped (D4).

## Architecture

**The index.** `docs/schema_design.sql` provides
`idx_class_sessions_pending ON class_sessions(teacher_id, session_date) WHERE
status = 'held' AND attendance_confirmed_at IS NULL AND deleted_at IS NULL`,
described there as "truy vấn nóng nhất" — the hottest query.

Note the mismatch worth being explicit about: the index predicate covers
`status = 'held'` only, while the product definition of pending also includes
`planned` sessions whose date has passed. A session generated for last Tuesday
that the teacher never touched is still sitting at `planned`, and it is exactly
the case the warning exists for — the teacher forgot, so nothing moved it to
`held`.

Two options, and the second is the recommendation:

1. Query `status IN ('held','planned')`. Correct, but the partial index does
   not cover the `planned` half, so that half is a heap scan filtered by
   `teacher_id`. At V1 scale (one teacher, a few hundred sessions a year) this
   is genuinely fine.
2. Query `status IN ('held','planned')` **and** add a second migration widening
   the index predicate to match. Preferred, because the query is on the app's
   first screen and the fix is three lines.

If option 2 is taken, it is a **new** migration (`000003_...`), never an edit
to the baseline — D1 keeps `000001_baseline_schema.up.sql` byte-identical to
`docs/schema_design.sql`, and `docs/schema_design.sql` should be updated in the
same change so the design doc and the database stay in agreement.

**"In the past" needs a timezone.** A session dated today is not pending until
the day is over — warning a teacher at 9am about a class they teach at 7pm is
noise, and noise is how a warning feed gets ignored. Compare against "today" in
the teacher's timezone (`teachers.timezone`, default `Asia/Ho_Chi_Minh`):
pending means `session_date < today_in_teacher_tz`.

A refinement worth considering but not building now: a session whose
`start_time` has passed today is arguably already pending. It adds a second
time comparison and a partially-populated column (`class_sessions.start_time`
is nullable) for a case the next day's feed catches anyway. YAGNI.

**Data flow**

```
GET /api/v1/sessions/pending?from=&to=&limit=
  -> handler: teacherID from authctx
  -> service.ListPending
       today := now().In(teacher.timezone) truncated to date
       repo.ListPending(teacherID, before: today, from, to, limit)
         SELECT cs.*, c.name, (roster size)
           FROM class_sessions cs
           JOIN classes c ON c.id = cs.class_id AND c.teacher_id = cs.teacher_id
          WHERE cs.teacher_id = ?
            AND cs.session_date < ?
            AND cs.attendance_confirmed_at IS NULL
            AND cs.status IN ('held','planned')
            AND cs.deleted_at IS NULL
          ORDER BY cs.session_date DESC
  -> 200 {total, items: [...]}
```

The expected student count comes from the same roster query phase 2 uses
(`enrollments.ActiveOn`). Computing it per row is an N+1; if the feed is
typically short (which it should be — a long feed means the product is
failing), a bounded loop is acceptable, but prefer a single grouped join over
`enrollments` filtered by each session's date. Cap the result set with a
default limit and say so in the response.

**Why cancelled sessions are excluded.** A cancelled session bills nobody
(plan-level decision, PRD edge case *"Buổi bị hủy do giáo viên → không tính
tiền cho ai"*), so there is nothing to record and nothing to warn about. The
schema also forbids a cancelled session from carrying
`attendance_confirmed_at`, so including them would produce a feed entry that
can never be cleared.

**Reuse by plan 04.** Period closing must refuse while any session inside the
period is unconfirmed, and must name the offenders. That is this exact query
with `from`/`to` bound to the period. Exposing the range filter now means plan
04 consumes a tested function instead of reimplementing the predicate — and a
divergence between "what the dashboard warns about" and "what blocks closing"
would be a genuinely confusing bug.

## Related Code Files

**Create**

- `apps/api/internal/features/sessions/pending.go` — repository query, service
  method, and DTO for the feed (kept in the sessions feature; it is a
  `class_sessions` query, and a separate package for one endpoint is not a real
  boundary)
- `apps/api/internal/features/sessions/pending_test.go`
- `apps/api/migrations/000003_widen_pending_sessions_index.up.sql` and
  `.down.sql` — only if option 2 above is taken

**Modify**

- `apps/api/internal/features/sessions/repository.go` — add `ListPending` to
  the `Repository` interface
- `apps/api/internal/features/sessions/service.go` — add `ListPending`,
  resolving the teacher's timezone
- `apps/api/internal/features/sessions/handler.go`,
  `apps/api/internal/features/sessions/routes.go` — the new endpoint
- `apps/api/internal/features/sessions/integration_test.go` — pending-feed
  cases
- `docs/schema_design.sql` — only if the index is widened, kept in step with
  the new migration
- `apps/api/seeds/seed.go` — ensure at least two past sessions stay unconfirmed
  so the feed is non-empty in a seeded environment

## Implementation Steps

1. Decide on the index. Recommendation: widen it. Write
   `000003_widen_pending_sessions_index.up.sql` dropping
   `idx_class_sessions_pending` and recreating it with
   `WHERE status IN ('held','planned') AND attendance_confirmed_at IS NULL AND
   deleted_at IS NULL`; the down migration restores the original predicate.
   Update `docs/schema_design.sql` in the same commit so the design doc stays
   truthful, and leave `000001_baseline_schema.up.sql` untouched (D1).
2. Add `ListPending(ctx, teacherID uuid.UUID, before time.Time, from, to
   *time.Time, limit int) ([]PendingSession, int64, error)` to the sessions
   repository, implementing the query above. Return the total separately from
   the limited rows.
3. Fetch the expected student count with one grouped join against `enrollments`
   (`started_on <= session_date AND (ended_on IS NULL OR ended_on >=
   session_date) AND deleted_at IS NULL`), not a per-row call.
4. Add `Service.ListPending`, which loads the teacher's timezone, computes
   today in that zone, and delegates. Default the limit (50 is ample) and cap
   it.
5. Add `PendingSessionResponse{SessionID, ClassID, ClassName, SessionDate,
   StartTime, Status, ExpectedStudentCount, DaysOverdue}`. `DaysOverdue` is
   computed, not stored — it is what makes the warning actionable, and it is
   the number the G4 metric is really about.
6. Mount `GET /sessions/pending` behind `requireAuth`. Register it **before**
   any `/sessions/:id` route in the same group, or Gin will try to parse
   `pending` as an id. Verify with a test that hits the literal path.
7. Extend the seeds so a seeded environment shows a non-empty feed.
8. Tests in `pending_test.go` and `integration_test.go`:
   - A past unconfirmed `planned` session appears.
   - A past unconfirmed `held` session appears.
   - A confirmed session does not.
   - A cancelled session does not, whether or not its date has passed.
   - A soft-deleted session does not.
   - Today's session does not (teacher timezone), and yesterday's does.
   - A teacher in a different timezone sees the correct boundary — set
     `teachers.timezone` to something with a large offset and assert the cutoff
     shifts.
   - `from`/`to` filtering returns only sessions inside the range, which is the
     shape plan 04 will call.
   - Teacher A never sees teacher B's pending sessions.
   - `total` reflects the unlimited count while `items` respects the limit.
9. `make api-docs`, `make test-api`, `make lint-api`.

## Success Criteria

- [ ] **R2 acceptance:** a past session that has not been recorded appears in
      the feed the dashboard loads first
- [ ] Both `planned` and `held` past sessions appear
- [ ] Confirmed, cancelled, and soft-deleted sessions never appear
- [ ] Today's session is not pending; yesterday's is, evaluated in the
      teacher's timezone
- [ ] The timezone boundary is asserted with a non-default `teachers.timezone`
- [ ] `from`/`to` filtering works and is the same predicate plan 04 will use
      for close-blocking
- [ ] Each row carries the class name and the expected student count
- [ ] `DaysOverdue` is correct
- [ ] `total` is the unlimited count; `items` respects the limit
- [ ] The feed query issues a bounded number of statements — no per-row roster
      lookup
- [ ] `GET /sessions/pending` resolves to the feed, not to `/sessions/:id`
- [ ] Teacher A never sees teacher B's sessions
- [ ] If the index was widened, it lives in a new migration and
      `000001_baseline_schema.up.sql` is byte-identical to
      `docs/schema_design.sql`
- [ ] `make test-api` and `make lint-api` pass

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Only `held` sessions queried, so forgotten `planned` sessions never warn | High | High — this is precisely the case the feature exists for, and it would look like it works | Called out explicitly under Architecture; a test covers the `planned` case first |
| Baseline migration edited to widen the index, breaking the D1 verbatim invariant | Medium | Medium — the baseline stops being diff-able against the design doc | New migration mandated at step 1; the diff check is a success criterion |
| "Past" computed in UTC, so the feed warns about tonight's class all afternoon | Medium | Medium — noisy warnings get ignored, and G4 decays quietly | Teacher timezone used; asserted with a non-default zone |
| N+1 on the expected student count | Medium | Low — the feed should be short | Grouped join at step 3; query-count assertion |
| `/sessions/pending` shadowed by `/sessions/:id` | Medium | Low — immediate 404 or a UUID parse error | Route ordering at step 6 plus a literal-path test |
| Plan 04 reimplements the predicate and diverges from the dashboard | Medium | Medium — closing blocked by sessions the dashboard never showed | Range filter exposed now specifically so plan 04 reuses this function |
