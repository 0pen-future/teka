---
title: "Teacher handoff and import nav"
description: "Class teacher reassignment (owner-only) + move import into Trung tâm nav + relocate class-settings button"
status: done
priority: P1
effort: "1.5d"
tags: [api, web, classes, imports, nav]
created: 2026-08-21
completed: 2026-08-21
---

# Teacher handoff and import nav

## Overview

Three user-approved changes, all downstream of the import investigation
(`plans/reports/debug-260821-2255-import-empty-workbook.md`, Addendum 2):

1. **Teacher handoff (option B):** an owner can reassign a class to another
   center member after setup. Import's blank-teacher-phone semantic (class →
   owner) stays; handoff closes the loop. Approved semantics: class + weekly
   schedules + **future planned sessions** move to the new teacher; held/
   cancelled sessions, attendance, and billing history keep the old teacher.
2. **Nav:** "Nhập từ Excel" becomes an owner-gated entry in the sidebar's
   "Trung tâm" group; the button is **removed** from the Lớp & học sinh page
   header (user chose "move", not "both").
3. **Header:** "⚙ Cài đặt lớp" moves from the class-tabs row into the page
   header, before "+ Tạo lớp mới" (still only when a class is selected).
4. **Empty import guard (added in validation, 2026-08-21):** an import whose
   workbook parses to zero data rows returns 422 with row-3 guidance instead
   of a silent all-zero "success"; UI warns on all-zero reports.

## Constraints / decisions

- `classes.teacher_id` stays NOT NULL — no schema change (verified live schema;
  nullable teacher was rejected: it is the authz anchor for ~20 features).
- Circular wiring: `sessions.NewService(..., classesSvc, ...)` (router.go:150)
  means classes cannot depend on sessions. The handoff endpoint therefore lives
  in a new coordinating feature package modeled on `imports` (consumer-defined
  interfaces over classes/sessions/centers services, TxManager).
- Past `planned` sessions (unconfirmed attendance) stay with the old teacher —
  only `planned` with `session_date >= today` moves. **Today is inclusive**
  (validated 2026-08-21): a handoff late in the day moves today's un-attended
  planned session too; the owner marks attendance first if it already ran.
- Owner-only, same-center validation on the target teacher.
- **No notification on handoff** (validated 2026-08-21): the new teacher sees
  the class in their own lists; no Zalo/in-app message in this plan's scope.

## Goals

| # | Goal | Priority |
|---|------|----------|
| 1 | Owner reassigns a class's teacher; future work moves, history stays | P1 |
| 2 | Import reachable from "Trung tâm" nav group; page button removed | P2 |
| 3 | "⚙ Cài đặt lớp" placed in header before "+ Tạo lớp mới" | P3 |
| 4 | Empty import file rejected with 422 + row-3 guidance; UI warns on all-zero report | P2 |

## Phases

| # | Phase | Status |
|---|-------|--------|
| 1 | [Phase 1: API teacher handoff endpoint](./phase-01-start.md) | Done |
| 2 | [Phase 2: Web class settings teacher handoff](./phase-02-web-class-settings-teacher-handoff.md) | Done |
| 3 | [Phase 3: Web nav and header cleanup](./phase-03-web-nav-and-header-cleanup.md) | Done |
| 4 | [Phase 4: Empty import guard](./phase-04-empty-import-guard.md) | Done |

Dependencies: Phase 2 blocks on Phase 1. Phases 3 and 4 are independent (can ship first).

## Success Criteria

- [x] `PUT /api/v1/classes/:id/teacher` (owner-only) reassigns class, all its
      schedule rows, and future planned sessions in one transaction; 403 for
      members, 422 for a non-member target; idempotent when target == current.
- [x] Held/cancelled sessions and past planned sessions keep the old teacher
      (integration-test-proven); billing/attendance history unchanged.
- [x] Class settings page (owner view) shows current teacher and a member
      picker + "Bàn giao lớp" action; lists refresh after handoff.
- [x] Sidebar "Trung tâm" group shows owner-gated "Nhập từ Excel" → `/students/import`;
      students-page header no longer has the import button.
- [x] Header order: [⚙ Cài đặt lớp (khi có lớp được chọn)] [+ Tạo lớp mới] [+ Thêm học sinh].
- [x] Empty workbook (header + example row only) → 422 with "nhập từ dòng 3"
      guidance on both check and commit; UI never shows success for an
      all-zero report.
- [x] Focused Go + web tests green; whole suites green for touched apps.

## Delivery notes (2026-08-21)

- **Public-contract extension (additive):** Phase 2 needed the class's current
  teacher, which neither `classes.ClassResponse` (Go) nor the web `classSchema`
  exposed. Added `teacher_id` to both — additive, no field removed/renamed — and
  updated every Class test fixture (MSW handlers) so zod parsing stays green.
  Swagger regenerated.
- **Timezone-boundary correction (code review, Finding #1):** the first cut of
  `sessions.ReassignPlanned` filtered future sessions with SQL `CURRENT_DATE`,
  which resolves in the DB session's zone (UTC in deployment) and diverges from
  the rest of the sessions feature, which computes "today" in the teacher's IANA
  zone in Go. Fixed to resolve today in the old teacher's timezone
  (`dateOnly(now().In(loc), UTC)`) and pass it as an explicit `notBefore`
  parameter — matching `ListPending`. Added a boundary unit test; de-flaked the
  integration seed to the teacher-local day. This upholds the "past planned
  sessions keep the old teacher" invariant in the early-morning-VN window.
- **Reviewer notes accepted, not actioned:** target-membership check + no-op
  detection run before the tx/lock (benign TOCTOU, identical posture to
  `imports`); on-purpose account-enumeration avoidance in `IsActiveMember`.

## Open questions

None — button removal and handoff semantics confirmed by user 2026-08-21;
validation session 1 resolved the remaining three (see Validation Log).

## Validation Log

### Session 1 — 2026-08-21
**Trigger:** user selected `/ak:plan validate` after plan creation
**Questions asked:** 3

### Verification Results
- Claims checked: ~24 (file paths, symbols, line anchors, route shapes)
- Verified: 24 | Failed: 0 | Unverified: 0
- Tier: Standard (Fact Checker + Contract Verifier; 3-phase plan at time of run)
- Evidence highlights: router.go:119/150 wiring order; `imports` lock/tx/owner
  gate (`lock.go:24-54`, `service.go:103-197`); sessions statuses + `SessionDate`
  (`sessions/model.go`); `ListMembers` active-only join (`centers/service.go:125`);
  `/classes` gin group `:id` param (`classes/routes.go:8-17`); swagger via
  `go tool swag init` (Makefile:77); `"members" in center` narrowing
  (`students-page.tsx:60`, `roster-import-page.tsx:34`); dashboard-layout
  spread/OVERFLOW_LABELS and test expectation all match claimed positions.

#### Questions & Answers

1. **[Scope]** Root-cause fix from the import debug report (empty workbook →
   silent success) was not in the plan. Add it as Phase 4 (API 422 for a
   zero-row plan + UI all-zero warning)?
   - Options: Thêm Phase 4 (Recommended) | Để plan riêng sau | Bỏ qua
   - **Answer:** Thêm Phase 4
   - **Rationale:** closes the investigated failure mode in the same delivery;
     independent of the handoff phases.
2. **[Assumptions]** Session-move boundary: `session_date >= today` moves
   today's planned session even on a late-day handoff. Inclusive today, or
   only from tomorrow?
   - Options: Gồm hôm nay (Recommended) | Chỉ từ ngày mai
   - **Answer:** Gồm hôm nay
   - **Rationale:** matches "from now on" intuition; if today's session
     already ran, the owner marks attendance before handing off (held
     sessions never move).
3. **[Scope]** Notify the new teacher (Zalo/in-app) on successful handoff?
   - Options: Không, MVP im lặng (Recommended) | Có, thêm thông báo
   - **Answer:** Không, MVP im lặng
   - **Rationale:** keeps notification infra out of the blast radius; the
     class appears in the new teacher's lists organically.

#### Confirmed Decisions
- Phase 4 added: empty-import guard (server 422 + UI warning + page copy) — P2.
- `>= CURRENT_DATE` inclusive-today boundary confirmed as-designed.
- No handoff notification in scope.

#### Action Items
- [x] Scaffold + write `phase-04-empty-import-guard.md`.
- [x] Propagate inclusive-today and no-notification notes to Phase 1.
- [x] Update goals/phases/success criteria in this index.

#### Impact on Phases
- Phase 1: boundary semantics confirmed (no change to the SQL predicate);
  explicit non-goal added for notifications.
- Phase 4: new, independent; can ship alongside Phase 3.

### Whole-Plan Consistency Sweep
Re-read `plan.md` + all four phase files after propagation: no stale claims —
the `>= CURRENT_DATE` predicate appears only in Phase 1 and matches the
confirmed inclusive-today decision; no phase references a notification step;
Phase 4 does not alter the row-2 positional skip that Phases 1–3 assume
nothing about. No unresolved contradictions.

<!-- slug: teacher-handoff-and-import-nav -->
