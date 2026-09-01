---
title: "Sync-back verification: Điểm danh 4 trạng thái + chọn buổi/lịch"
plan: plans/260831-2034-attendance-4-statuses-calendar-ui/
status: completed
date: 2026-08-31
---

# Sync-back Verification Report

## Verdict
Plan sync-back verified. All 5 phases marked `completed`, no unchecked boxes
in plan.md or any phase file. Fixed 2 stragglers in phase-01 (see below).
Cross-checked 2 deviations and 2 out-of-plan fixes against actual code —
all confirmed present.

## Straggler fixes applied this pass
- `phase-01-db-attendance-api.md`: "Related Code Files" and Implementation
  Step 1 still said migration `000020`; corrected to `000021` (matches
  actual file `migrations/000021_attendance_status_late.{up,down}.sql`;
  `000020` was already taken by `retire_legacy_scope_keys`). plan.md itself
  already had the correct `000021` reference — only phase-01 body text was
  stale.
- `phase-01-db-attendance-api.md`: requirement line said off-roster student
  in `marks` → 404; actual behavior (verified in `handler.go` swag comment
  + `handler_test.go:266,277,281` using `http.StatusUnprocessableEntity`)
  is 422. Corrected text, noted as intentional deviation from the original
  404 assumption for consistency with the rest of `marks` validation.

## Shipped per phase
1. **DB + attendance API** — migration 000021 adds `late` to
   `attendance_records.status` CHECK; `POST /sessions/:id/attendance`
   accepts `marks: [{student_id, status, note?}]`, legacy
   `absent_student_ids` still works (mutually exclusive with `marks`, both →
   400); off-roster → 422; dup student_id → 400; billable=true unchanged
   for all 4 statuses, integration test proves billing tally unaffected.
2. **Sessions attendance_summary** — `GET /classes/:id/sessions`,
   `GET /sessions/:id`, `GET /sessions/pending` return
   `attendance_summary {present, late, absent, excused}` (null pre-confirm),
   single aggregate query, soft-deleted records excluded.
3. **Web attendance table** — 4-column radiogroup table (mint/sun/coral/sky),
   Map-based state, status count chips, note flow for excused, dynamic
   confirm bar, a11y roles, mobile 390px / desktop 1440px per artboard.
4. **Web session picker + calendar** — 3-card TRƯỚC/HÔM NAY/KẾ TIẾP with
   ‹ › navigation, month calendar modal with status dots, badge summary
   from attendance_summary, responsive horizontal(mobile)/vertical(desktop).
5. **E2E + docs + verification** — Playwright specs updated for 4-status
   flow + card/calendar navigation, swagger regen clean, gates run.

## Gate results (per team-lead context, spot-checked)
- `make test-api`: green (1 integration flake re-run green; coverage >60% floor)
- web: typecheck clean, 465 vitest tests green, lint 0 errors
- Playwright e2e: 32/32 green on isolated `teka-e2e` stack

## Deviations from plan text (both confirmed against code, now reflected in phase files)
- Migration numbered 000021, not 000020 (000020 was already used by an
  unrelated prior migration).
- Off-roster marks → 422, not 404 as originally assumed.
- `migrations_test.go` migration-count assertions use 17 (was 16 pre-change).

## Out-of-plan fixes shipped (verified in code, not reverted)
1. `seeds/seed.go` — assigns `giao_vien` role on membership, matching
   runtime `OpenMembership` behavior (seed drift fix).
2. `billing/close.go` — returns non-nil empty warnings slice on
   period-last-day close; regression test added.
3. `internal/server/route_policy.go` — 5 notifications routes
   reclassified to `PolicyService` kind (confirmed at lines 221-225);
   pre-existing master regression from PR #39 that blocked class-hoc_vu
   sends and own-period ledger reads. Comment at line 27-33 documents intent.
4. Post-review fix — 3 reporting queries (attendance `TallyByEnrollment`,
   centers class overview, session stats) now count `late` as present and
   `excused` as absent for display purposes; billing counts untouched.
   Regression test `TestFourStatusConfirmPersistsAndKeepsBillableTally`
   added.

## Open follow-ups (not blocking, noted in plan Open Questions / phase risk sections)
- Trio picker anchor session outside the ±45-day query window is a known
  edge case (window nudges on prev/next, per phase-04 risk note).
- Month-calendar "window-start" dead-end when navigating before any
  fetched window — same root cause as above.
- `todayIso()` uses UTC convention for "today" comparisons — timezone edge
  case not addressed, low risk for single-timezone deployment.
- Radiogroup roving tabindex not implemented (arrow-key nav is
  focusable-buttons only, per phase-03 non-functional note — meets stated
  a11y bar, not full ARIA radiogroup pattern).
- Seed role-subquery guard: seed fix (#1 above) has no negative test for
  the subquery falling through silently.
- MSW fixture shape for `attendance_summary` — confirm consumers (mobile
  session cards, calendar dots) all handle `null` pre-confirm state
  consistently; no report of drift found but not exhaustively audited here.

## Unresolved questions
1. Parent-facing statement text shows `excused` as "vắng" (absent) — this
   is implemented per plan Open Question #2 (excused stays billable=true,
   no separate wording), but the specific parent-facing label choice was
   not explicitly re-confirmed with the user/product owner after
   implementation. Flagging for a sign-off pass.
2. Timing for removing deprecated `absent_student_ids` support — plan
   Open Question #3 proposed "one release then remove" but no removal
   ticket/date has been created yet.
