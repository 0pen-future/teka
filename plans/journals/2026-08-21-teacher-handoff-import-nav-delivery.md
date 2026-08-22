---
title: Teacher handoff + import nav delivery
date: 2026-08-21
summary: "Shipped owner-only class teacher handoff, import nav relocation, and empty-import 422 guard; fixed a timezone-boundary review finding"
---

# Teacher handoff + import nav delivery

## What happened
Executed plan `260821-2252-teacher-handoff-and-import-nav` end-to-end (4 phases).

- **Phase 1 (API):** `PUT /api/v1/classes/:id/teacher`, owner-only, in a new `handoff`
  coordinating feature package (mirrors `imports`). One transaction under the center
  advisory lock: class + weekly schedules + future `planned` sessions move to the new
  teacher; held/cancelled/past-planned sessions and attendance/billing history stay
  with the old teacher. 403 for members, 422 for a non-member target, no-op when
  target == current teacher.
- **Phase 2 (Web):** owner-only "Giáo viên phụ trách" card on class-settings-page;
  invalidates class + session query keys after handoff.
- **Phase 3 (Web):** "Nhập từ Excel" moved into sidebar "Trung tâm" group (owner-gated);
  removed the import button from the students-page header; "⚙ Cài đặt lớp" moved into the
  header before "+ Tạo lớp mới".
- **Phase 4:** empty workbook (parses to zero classes AND zero students) → 422 with
  row-3 guidance on both dry-run and commit; UI all-zero warning card; import-page copy.

## Decision
- **Additive contract extension:** Phase 2 needed the class's current teacher, so
  `teacher_id` was added to `classes.ClassResponse` (Go) and the web `classSchema` —
  additive only, all Class fixtures updated, swagger regenerated.
- **Code-review Finding #1 (timezone boundary):** the first cut filtered future sessions
  with SQL `CURRENT_DATE`, which resolves in the DB session zone (UTC in deployment) and
  diverges from the sessions feature, which computes "today" in the teacher's IANA zone
  in Go. Fixed cause-aligned: resolve today in the OLD teacher's timezone
  (`dateOnly(now().In(loc), UTC)`, default Asia/Ho_Chi_Minh) and pass it to the repo as an
  explicit `notBefore` parameter, matching `ListPending`. Chose passing `oldTeacherID`
  through the interface over reorder-and-self-Get to avoid a hidden ordering dependency.
  Added `TestReassignPlannedBoundaryUsesTeacherTimezone`; de-flaked the integration seed to
  the teacher-local calendar day.
- **Two Low findings accepted, not actioned:** membership check + no-op detection run
  before the tx/lock (benign TOCTOU, identical to `imports`); deliberate
  account-enumeration avoidance in `IsActiveMember`.

## Next steps
- Not committed (user chose to leave the working tree uncommitted; 38 changes on master).
- Tests green: Go 27 packages ok, web 370 pass.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
