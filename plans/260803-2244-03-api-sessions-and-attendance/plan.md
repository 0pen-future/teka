---
title: "03 API Sessions and Attendance"
description: "Class sessions generated from schedules, session lifecycle, and one-touch attendance (điểm danh) — the raw data every fee calculation is derived from."
status: pending
priority: P1
effort: "18h"
tags: [api, go, attendance, sessions, north-star]
created: 2026-08-03
blockedBy: [260803-2244-02-api-roster-management]
blocks: [260803-2244-04-api-billing-engine]
---

# 03 API Sessions and Attendance

## Overview

PRD **R2** ("Điểm danh 1 chạm" — one-touch attendance) and the system's North
Star metric **G4**: *"Tỉ lệ buổi được điểm danh trong vòng 24h ≥ 90%."* The PRD
is blunt about the stakes: *"Mọi giá trị phía sau đều là hệ quả toán học của dữ
liệu điểm danh. Nếu tỉ lệ này dưới 90%, sản phẩm vô giá trị bất kể báo cáo đẹp
đến đâu."*

Two tables: `class_sessions` (one row per class per teaching date) and
`attendance_records` (one row per student per session — including the students
who were present). Everything the billing engine computes is a `SELECT` over
those rows.

The interaction budget is the hard requirement. PRD R2 acceptance: a class of
30 students with 2 absentees must be recorded in **at most 3 interactions** —
tick the two absentees, press confirm. The API shape follows from that budget:
the confirm endpoint receives only the absent student ids and the server
materialises everything else.

## Scope

**In scope**

- Generating `class_sessions` from `class_schedules` for a date range,
  idempotently.
- Session lifecycle: `planned` → `held`, or `cancelled` with a reason.
- One-touch attendance confirmation and re-editing of past confirmed sessions.
- The pending-attendance warning feed that PRD R2's third acceptance criterion
  and the dashboard need.

**Non-goals**

- Any money. Fee recalculation after an attendance edit is plan 04's
  responsibility; this plan defines the contract and the trigger point.
- `status = 'excused'` (nghỉ có phép — excused absence). The schema reserves the
  value and the `billable` column exists for it, but PRD lists it under P1.
  V1 writes only `present` and `absent`, both `billable = true`.
- Make-up sessions in another class (P1, schema note (r)).
- Push notifications or reminders for unconfirmed sessions. This plan exposes
  the data; delivery is out of scope for V1.
- Auto-generating sessions on a cron. Generation is on demand and idempotent
  (see Key Decisions).

## Phases

| # | Phase | Effort | Depends on | Status |
|---|-------|--------|------------|--------|
| 1 | [Session generation and lifecycle](./phase-01-session-generation-and-lifecycle.md) | 7h | — | Pending |
| 2 | [One-touch attendance](./phase-02-one-touch-attendance.md) | 7h | 1 | Pending |
| 3 | [Pending attendance warnings](./phase-03-pending-attendance-warnings.md) | 4h | 2 | Pending |

## Key Decisions

Inherited from plan 01 (D3 UUIDv7 in Go, D4 repository-layer tenancy plus
`deleted_at IS NULL`, D5 BIGINT money and `VARCHAR`+`CHECK` states mirrored as
Go constants, D6 GORM mirrors and never migrates). Plan-specific:

**Sessions are generated on demand, not by a scheduler.** A request for a
class's sessions over a date range materialises any missing rows from the
effective `class_schedules` and returns the union. No cron job, no background
worker, no "session generation service" to operate. The idempotency guarantee
comes from `uq_class_sessions_per_day` — unique `(class_id, session_date)` where
`deleted_at IS NULL` — so a concurrent double-generation inserts one row and
the other conflict is swallowed. This is the YAGNI choice: a teacher with three
classes does not need a scheduler, and a scheduler would need its own failure
handling, monitoring, and backfill story.

**Cancelled sessions bill nobody.** `status = 'cancelled'` with a
`cancel_reason`, and the schema enforces
`CHECK (status <> 'cancelled' OR attendance_confirmed_at IS NULL)` — a
cancelled session can never carry a confirmation. PRD edge case: *"Buổi bị hủy
do giáo viên → không tính tiền cho ai."* Cancelling is distinct from
soft-deleting: the reason is shown to parents on the statement link, so a
mistakenly-created session is what `deleted_at` is for.

**The confirm endpoint receives absentees only.** `POST
/sessions/:id/attendance` takes `{absent_student_ids: [...], note?}`. The
server resolves the roster from open enrollments on the session date, writes
`present` for everyone not listed and `absent` for everyone listed, and stamps
`attendance_confirmed_at`. This is what makes the 3-interaction budget
achievable, and it is why `attendance_records` stores present students too:
without those rows there is no way to distinguish "present" from "not yet
recorded", and the pending-session warning becomes unimplementable.

**Editing a confirmed past session is a first-class operation, not an
exception.** PRD user story 8: *"tôi hay điểm danh muộn hoặc nhầm."* Re-posting
the confirm endpoint replaces the record set for that session. Records are
updated in place (never soft-deleted — the schema warns that soft-deleting an
attendance record skews the billable count and therefore the money already sent
to a parent).

**Attendance edits notify billing through a documented contract, not a
callback.** Plan 04 recomputes from `attendance_records` whenever it runs, so an
edit to an open period needs no notification at all. An edit to a *closed*
period is the case that needs handling, and the schema already provides the
mechanism: `invoice_adjustments.source_session_id` points back at the edited
session. This plan therefore only guarantees that edits are recorded with
`updated_at` and that the session id is stable; plan 04 owns detection and
adjustment. PRD Q5 (resend the message or carry the difference forward) is
still open and is plan 04's to resolve.

## Acceptance Criteria

Traced to PRD R2.

- [ ] **R2 acceptance:** a 30-student class with 2 absentees is recorded in at
      most 3 interactions — two ticks and one confirm. Measured as: one API
      call carrying two ids.
- [ ] **R2 acceptance:** a session confirmed three days ago can be reopened,
      edited, and reconfirmed, and the stored records reflect the edit.
- [ ] **R2 acceptance:** a past session that has not been confirmed appears in
      the warning feed the dashboard renders on first load.
- [ ] Sessions are generated from `class_schedules` for a requested range and
      generating twice creates no duplicates.
- [ ] Sessions are never generated before the class `start_date` or outside a
      schedule's effective range.
- [ ] A cancelled session carries a reason, has no attendance records, and
      cannot be confirmed.
- [ ] Attendance rows exist for **every** enrolled student on the session date,
      not only absentees.
- [ ] A student who joined after a session's date gets no record for it; a
      student who left before it gets none either.
- [ ] Confirming twice concurrently produces exactly one record per student.
- [ ] All endpoints are teacher-scoped; another teacher's session id returns
      404.

## Data model summary

```
classes ──1:n── class_schedules
   │                  │  (weekday, start_time, effective range)
   │                  ▼
   └──1:n── class_sessions (class_id, session_date UNIQUE where not deleted)
                      │  status: planned | held | cancelled
                      │  attendance_confirmed_at
                      ▼
            attendance_records (session_id, student_id UNIQUE where not deleted)
                      status: present | absent | (excused, P1)
                      billable BOOLEAN
                      enrollment_id ──► enrollments (carries unit_price)
```

`attendance_records.enrollment_id` is the link that lets plan 04 price a
session without re-deriving which enrollment was active — it is captured at
confirmation time, when the answer is unambiguous.

## Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Attendance records written only for absentees | Medium | High — "present" becomes indistinguishable from "not recorded", G4 unmeasurable and billing wrong | The schema comment says so explicitly; phase 2 asserts a full record set for a 30-student class |
| Duplicate sessions from concurrent generation | Medium | Medium — double-billed sessions | `uq_class_sessions_per_day` plus `ON CONFLICT DO NOTHING`; concurrent-generation test |
| Roster resolved from current enrollments rather than as of the session date | Medium | High — a student who left last month reappears on an old session and gets billed | `enrollments.ActiveOn` from plan 02 phase 4 is the only sanctioned query; asserted by test |
| Attendance records soft-deleted on edit instead of updated | Medium | High — skews billable counts, changes money already reported | Schema warning quoted in the code; phase 2 asserts record ids are stable across edits |
| The confirm API grows into a per-student endpoint, blowing the 3-interaction budget | Low | High — kills the North Star metric | The budget is an acceptance criterion with a concrete measurement |
| Session dates drift by a day through timezone conversion | Medium | High — sessions on the wrong date, attendance against the wrong roster | `session_date` is a `DATE`; generate in the teacher's timezone (`teachers.timezone`) and never convert through UTC timestamps |

## Open Questions

1. How far ahead should on-demand generation materialise sessions? Generating
   the requested window only is simplest, but the dashboard needs "today and
   this week" and the billing screen needs "this month". Phase 1 proposes
   generating the requested range and letting callers ask for what they need.
2. When a schedule changes mid-term, what happens to already-generated future
   sessions under the old timetable? They are not automatically removed. A
   teacher would have to cancel or delete them. Whether the API should offer a
   "regenerate future sessions" action is undecided and depends on how often
   teachers actually move a class slot.
3. Does a session need an explicit `held` transition, or is confirming
   attendance the transition? Phase 1 proposes that confirming attendance sets
   `status = 'held'` implicitly, so the teacher never presses two buttons —
   consistent with the interaction budget. Explicit transition remains
   available for the case where a teacher marks a session held before
   recording attendance.
4. PRD Q5, inherited: after a parent has been notified, does editing attendance
   resend the message or carry the difference to the next period? Owned by plan
   04, but the edit path built here is where the divergence starts.

<!-- slug: 03-api-sessions-and-attendance -->
