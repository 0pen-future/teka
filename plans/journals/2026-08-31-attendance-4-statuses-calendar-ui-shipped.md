---
title: Attendance 4 statuses + calendar UI shipped
date: 2026-08-31
summary: "Completed 5-phase plan: late/excused statuses, marks API, trio picker + month calendar; fixed 2 pre-existing master regressions along the way"
---

# Attendance 4 statuses + calendar UI shipped

## What happened

Executed plan `plans/260831-2034-attendance-4-statuses-calendar-ui/` end-to-end (--auto --tdd). Attendance expanded from present/absent to 4 statuses (present/late/absent/excused, all billable=true), API takes `marks: [{student_id, status, note?}]` with deprecated `absent_student_ids` back-compat, session DTOs carry `attendance_summary`, web got a 4-column status radio grid plus a 3-card session trio picker (TRƯỚC/HÔM NAY/KẾ TIẾP) with a month-calendar modal.

Deviations from plan text: migration landed as `000021_attendance_status_late` (000020 was taken); off-roster marks return 422 not 404; migrations_test count 16→17.

## Root causes worth remembering

- **PR #39 route-policy regression**: five `/billing-periods/:id/notifications*` routes had gained a route-level `perm(reports.send)` gate that pre-empted complete in-service authorization (`AuthorizeClassSend`, `ReportsOversight`, own-period scoping). It 403'd the class secretary's bulk send and a teacher's own-period ledger reads — 8 e2e tests red on master (CI run 33367983547). Fixed by adding a `PolicyService` kind to the route-policy manifest (`apps/api/internal/server/route_policy.go`): policy layer guarantees authenticated live member, the feature service's frozen gates are the authorization and fail closed. Reviewer independently verified all five service paths fail closed.
- **Reviewer High finding (fixed via TDD)**: three reporting queries still filtered exact `'present'`/`'absent'`, so late/excused fell into no bucket — parents saw "0 buổi vắng" on a charged excused session, dashboards under-counted attendance for late. Fixed by folding: present_count = `IN ('present','late')`, absent_count = `IN ('absent','excused')` in attendance `TallyByEnrollment` and centers class-overview/session-stats. Billable counts untouched; `TestFourStatusConfirmPersistsAndKeepsBillableTally` extended to pin the fold.
- Other out-of-plan fixes: `seeds/seed.go` now assigns giao_vien role on membership (matched runtime OpenMembership); `billing/close.go` returns non-nil empty warnings slice on period-last-day close.

## Decision

Excused counts as an absence ("vắng") in parent-facing statement counts and reports — user confirmed. Late counts as present. Money invariant (billable_count × unit_price) provably unchanged. The sessions `attendance_summary` aggregate intentionally keeps 4 separate buckets.

Rejected reviewer High 2 (±45-day window materializing future sessions) with evidence: the plan explicitly chose the materializing endpoint for the trio/calendar; pending feed cuts at before=today; billing close clamps blockingSessions to=today, so nothing blocks.

## Gates

`make test-api` green (one collections-integration Docker cgroup flake, green on re-run); web typecheck + 465 vitest + lint 0 errors; Playwright 32/32 on isolated `teka-e2e` stack (torn down after).

## Next steps

- Follow-ups (reviewer Medium/Low, not blockers): trio-picker anchor outside ±45d window; can't page past window start; `todayIso()` UTC vs UTC+7; radiogroup roving tabindex; seed `ensureMember` NULL role guard; MSW fixture impossible shape; `ATTENDANCE_STATUSES.find(...)!` assertion.
- No ticket yet for removing deprecated `absent_student_ids`.
- `docs/adding-permissions.md` + api-guidelines.md hunk left uncommitted (unrelated RBAC leftover, user to handle).

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
