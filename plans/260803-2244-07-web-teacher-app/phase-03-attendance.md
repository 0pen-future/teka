---
phase: 3
title: "Attendance: Sessions, One-Touch Điểm Danh, Past Edits"
status: completed
priority: P2
effort: "1.5d"
dependencies: [1]
---

# Phase 3: Attendance — Sessions, One-Touch Điểm Danh, Past Edits

## Overview

Build the screen the whole product depends on. PRD §2 names attendance-within-24h
the North Star (G4): if this screen is slow or annoying, every downstream number
is worthless. The hard budget is PRD R2 AC 1 — a 30-student class with 2
absentees must cost **at most 3 interactions**: tick absent #1, tick absent #2,
confirm.

That budget dictates the design: everyone is present by default, no save button
per row, no confirmation dialog on the happy path, no scroll-to-submit.

## Requirements

- [x] Session list at `/sessions` grouped by date, unconfirmed past sessions
      first and visually flagged (PRD §5 story 7).
- [x] Attendance screen defaults every enrolled student to present; the teacher
      only marks absentees (PRD R2).
- [x] Confirming is one interaction from anywhere on the screen — the confirm
      button is fixed to the bottom of the viewport, not at the end of a
      30-row list.
- [x] Absent count is visible on the confirm button at all times
      ("Xác nhận · 2 vắng").
- [x] Same-name siblings in one class are visually distinct via `display_note`
      (`docs/schema_design.sql:109`); when two students in the roster share a
      `full_name`, the note is promoted to a badge rather than muted suffix.
- [x] A confirmed session reopens and remains editable; saving recomputes fees
      (PRD R2 AC 2).
- [x] Editing attendance for a session inside a **closed** period shows a
      warning that the difference becomes an adjustment in the next period
      (PRD R4 AC 2; `invoice_adjustments.source_session_id`,
      `docs/schema_design.sql:344`).
- [x] Cancelling a session is possible with a reason and bills nobody
      (PRD §5 edge case; `class_sessions.status = 'cancelled'`,
      `docs/schema_design.sql:201`).
- [x] Only students whose enrollment covers `session_date` appear in the roster
      for that session — the server decides this; the UI must not filter.

## Architecture

`apps/web/src/features/attendance` owns sessions and attendance records
together: they are never used apart, and the attendance screen is a session
detail view.

**Interaction budget, concretely.** The screen holds a local `Set<string>` of
absent student ids. Tapping a row toggles membership — no network call, no
optimistic mutation, no per-row spinner. Only "Xác nhận" issues a request,
sending the absent id list. The server writes `present` rows for everyone else
and stamps `attendance_confirmed_at` (`docs/schema_design.sql:204`,
`:223-225`). This is why the UI can be one-touch while the DB still stores a row
per student.

```
AttendancePage
  useSessionRoster(sessionId)  -> GET /sessions/:id/attendance
      -> { session, students[], existing_records[] | null }
  local state: absentIds: Set<string>   (seeded from existing_records)
  tap row       -> toggle in Set                     (0 requests)
  tap "Xác nhận" -> PUT /sessions/:id/attendance      (1 request)
      body { absent_student_ids: string[] }
      -> invalidate: session detail, /sessions/pending, dashboard, billing review
```

**Why `PUT` with the full absent list rather than per-row PATCH:** it is
idempotent, it makes editing a past session identical to first-time entry (same
request, same handler), and a dropped connection never leaves a half-marked
session. A half-marked session is worse than an unmarked one because it looks
done.

**Dirty-state guard.** Because taps do not persist, leaving the screen with
unsaved toggles must warn. Use a `useBlocker` from react-router on
`absentIds`-vs-`existing_records` divergence. This is the one place where the
"no save button" design creates a data-loss path.

**Closed-period warning.** The session payload carries
`period_status: "open" | "closed" | null`. When `closed`, render a persistent
warning banner above the roster and change the confirm button copy to
"Lưu và tạo điều chỉnh". The adjustment itself is server-side; the UI only has
to make the consequence legible before the teacher commits.

**Assumed API contract** (reconcile with plans 03 and 04):

| Method | Path | Notes |
|---|---|---|
| GET | `/sessions?from&to&class_id&status` | list for the sessions page |
| GET | `/sessions/pending` | shared with phase 1's dashboard |
| GET | `/sessions/:id/attendance` | `{ session, students[{ id, full_name, display_note, enrollment_id }], records[] \| null, period_status }` |
| PUT | `/sessions/:id/attendance` | `{ absent_student_ids }` → confirmed session |
| POST | `/sessions/:id/cancel` | `{ cancel_reason }` |
| POST | `/sessions` | ad-hoc extra session `{ class_id, session_date, start_time }` |

## Design Spec (prototype `attend` screen)

The prototype's attendance layout is a two-pane desktop screen whose right
panel is explicitly "mô phỏng thao tác trên điện thoại" — i.e. the panel **is**
the phone attendance screen. Implementation: at `lg+` (desktop), `/sessions`
renders list + panel side by side (panel ~400px); under `lg` (phone **and**
tablet — a 768px split would leave both panes too narrow) the same panel is
the standalone `/sessions/:id/attendance` page, centered at `--w-content` on
tablet. One component tree, two mounts.

**Session list (left pane / `/sessions` on mobile).**

- Class pill tabs above the list (same recipe as the roster tabs: active
  `--mint-400` + `shadow-press-mint`, idle white + line border).
- Session rows inside an `HvCard flat` list: date + weekday `font-display`
  700, time 13px `--ink-400`; status text — pending "Chưa điểm danh"
  `--coral-600` 700, done "N có mặt · M vắng" `--mint-600`, cancelled "Đã huỷ —
  {lý do}" `--ink-400`. Selected row (`lg+` two-pane only): `--mint-50` bg +
  2px `--mint-300` border. Row min-height ≥48px, whole row tappable.

**Attendance panel (prototype right panel).**

- Header block `--mint-400` bg, white text, rounded top `--radius-xl`: class
  name `font-display` 700 18px, session date/time 13px at 90% opacity.
- Count pills row: "N có mặt" (white bg, `--mint-600`) and "M vắng" (white bg,
  `--coral-600`), `--radius-pill`, `font-display` 700, updating live as rows
  toggle.
- `AttendanceRow`: white bg, `--line-200` separators, min-height 52px, name
  15px 700; right-aligned 34px circular mark — present: `--mint-50` bg
  `--mint-600` "✓"; absent: `--coral-400` bg white "✕". Absent rows tint the
  whole row `--coral-100`. Duplicate-name `display_note` renders as a sky
  `HvBadge` beside the name; edge-case hints (e.g. "mới vào 15/07 — tính từ
  buổi này") render 12px `--sun-600` under the name.
- `ConfirmAttendanceBar`: `HvButton variant="primary" size="lg" block` label
  "XÁC NHẬN BUỔI HỌC" (uppercase `font-display`, count appended: "· M vắng"),
  sticky bottom with safe-area inset; chunky press-mint shadow is the tactile
  confirmation cue.
- Closed-period state: `ClosedPeriodWarning` as a `--sun-100` banner with
  `--sun-600` text; confirm button switches to `variant="reward"` "Lưu và tạo
  điều chỉnh" (mirrors prototype `modalWarn`'s reward-variant action).
- Cancelled session: centered 🚫 glyph + "Buổi học đã huỷ" `font-display` 700,
  reason line, note "Không tính tiền cho học sinh nào" 13px `--ink-400`.
- Save toast via `HvToast` (ink-900 pill, bottom-center, auto-dismiss).

## Related Code Files

**Create**

- `apps/web/src/features/attendance/api/attendance-api.ts` — `getSessions`,
  `getSessionRoster`, `saveAttendance`, `cancelSession`, `createSession`.
- `apps/web/src/features/attendance/schemas/attendance-schemas.ts` —
  `sessionSchema`, `sessionRosterSchema`, `attendanceRecordSchema`.
- `apps/web/src/features/attendance/types/attendance-types.ts`
- `apps/web/src/features/attendance/hooks/use-sessions.ts` — `sessionsKeys`,
  `useSessionsList`, `useSessionRoster`, `useSaveAttendance`,
  `useCancelSession`.
- `apps/web/src/features/attendance/components/session-list-item.tsx`
- `apps/web/src/features/attendance/components/attendance-row.tsx`
- `apps/web/src/features/attendance/components/confirm-attendance-bar.tsx`
- `apps/web/src/features/attendance/components/closed-period-warning.tsx`
- `apps/web/src/features/attendance/components/cancel-session-dialog.tsx`
- `apps/web/src/features/attendance/pages/sessions-page.tsx`
- `apps/web/src/features/attendance/pages/attendance-page.tsx`
- `apps/web/src/features/attendance/routes.tsx`
- `apps/web/src/features/attendance/index.ts`
- `apps/web/src/features/attendance/__tests__/attendance-page.test.tsx`
- `apps/web/src/features/attendance/__tests__/sessions-page.test.tsx`
- `apps/web/e2e/attendance.spec.ts`

**Modify**

- `apps/web/src/app/router.tsx` — mount `attendanceRoutes` (`:31`).
- `apps/web/src/test/msw/handlers.ts` — a 30-student class fixture plus a
  same-name sibling pair.

**Delete**

- None.

## Implementation Steps

1. Write `attendance-schemas.ts` against the DB shapes: session `status` is
   `'planned' | 'held' | 'cancelled'` (`docs/schema_design.sql:201`), record
   `status` is `'present' | 'absent' | 'excused'` (`:233`). Parse `excused` but
   never offer it in the UI — it is reserved for P1 (`docs/schema_design.sql:234`).
2. Write `attendance-api.ts` following the users API module shape
   (`apps/web/src/features/users/api/users-api.ts:7`).
3. Write `use-sessions.ts` with a `sessionsKeys` factory. `useSaveAttendance`
   invalidates the session roster, the sessions list, the dashboard pending
   query, and the billing review key exported from `features/billing`
   (phase 1) — attendance changes are exactly what the review table reads.
4. Build `SessionsPage`: a date-range control defaulting to the last 14 days
   through today, the class **pill tabs** from the Design Spec as the class
   filter, and sessions grouped by date under sticky date headers. Unconfirmed
   past sessions render the `--coral-600` "Chưa điểm danh" treatment and sort
   to the top in a dedicated group. At `lg+`, selecting a row renders the
   attendance panel beside the list (Design Spec two-pane layout) while the
   URL still moves to `/sessions/:id/attendance` so deep links work at every
   width; under `lg` the same URL renders the standalone panel page.
5. Build `SessionListItem`: class name, time, status badge, and — for confirmed
   sessions — the absent count. The entire row is the link target so it is
   thumb-friendly.
6. Build `AttendanceRow` as a full-width `button` (not a checkbox with a
   separate label): tapping anywhere on the row toggles absence, styled per the
   Design Spec (34px ✓/✕ circle mark, coral-100 row tint when absent),
   `aria-pressed` carrying the state. Minimum row height 52px per the
   prototype (≥48px touch target holds).
7. Promote `display_note` to a visible badge when the roster contains a
   duplicate `full_name`; compute the duplicate set once per roster render. When
   the name is unique, show `display_note` as a muted suffix. This is the
   mis-tick guard from PRD §5.
8. Build `ConfirmAttendanceBar`: `sticky bottom-0` with a safe-area inset,
   showing "Xác nhận · N vắng" and a pending state during the request. It stays
   visible while scrolling a 30-row list — this is what keeps the interaction
   count at 3 rather than 4.
9. Build `AttendancePage`: header (class name, date, weekday), optional
   `ClosedPeriodWarning`, the roster list, and the confirm bar. Seed `absentIds`
   from `records` when the session was already confirmed, so reopening shows the
   previous marks rather than resetting everyone to present.
10. Add the `useBlocker` dirty guard with a confirm dialog: "Chưa lưu điểm danh.
    Rời khỏi trang?".
11. On successful save, toast "Đã điểm danh {N} có mặt, {M} vắng" and navigate
    back to the session list. Do not stay on the page — the teacher's next
    action is always another session or leaving the app.
12. Build `CancelSessionDialog` with a required reason (`cancel_reason` is free
    `TEXT`, `docs/schema_design.sql:202`) and copy stating nobody is billed for
    a cancelled session.
13. Add an "Thêm buổi học" action on `SessionsPage` for ad-hoc make-up sessions
    (class, date, time). Keep it secondary — the normal path is server-generated
    sessions from the class schedule.
14. Write `routes.tsx` (`sessions`, `sessions/:id/attendance`) with lazy imports
    and register in `apps/web/src/app/router.tsx:31`.
15. Add msw fixtures: a class with 30 students including two named "Nguyễn Văn
    An" distinguished by `display_note`, one unconfirmed past session, one
    confirmed session with 2 absentees, and one session inside a closed period.
16. Write `attendance-page.test.tsx` asserting: (a) all students render as
    present initially and no request fires on mount beyond the roster GET;
    (b) two row taps plus one confirm tap produce exactly one PUT carrying the
    two absent ids — this is the executable form of R2 AC 1; (c) reopening a
    confirmed session pre-marks the previous absentees; (d) a closed-period
    session shows the adjustment warning and the alternate button copy;
    (e) duplicate names render their `display_note` badge.
17. Write `sessions-page.test.tsx` covering the unconfirmed-first grouping and
    the "Chưa điểm danh" badge.
18. Write `apps/web/e2e/attendance.spec.ts`: open a pending session from the
    dashboard, mark two absentees, confirm, and assert the dashboard warning
    clears and the session shows "2 vắng".
19. Run typecheck, lint, and vitest.

## Success Criteria

- [x] Marking 2 absentees in a 30-student class and confirming takes exactly 3
      taps, asserted by a test that counts `user-event` interactions.
- [x] The confirm button is reachable without scrolling at any scroll position
      on a 375×667 viewport.
- [x] Reopening a session confirmed days ago shows the earlier absentees and
      re-saving succeeds.
- [x] A session inside a closed period shows the adjustment warning before the
      teacher commits.
- [x] Two same-name students in one class are visually distinguishable.
- [x] Cancelling a session records the reason and the session no longer counts
      toward the pending warning.
- [x] After confirming, the dashboard's pending alert no longer lists that
      session.
- [x] The panel matches the prototype `attend` recipe: mint-400 header, live
      count pills, ✓/✕ circle marks, coral-100 absent rows, uppercase lg
      confirm button with press-mint shadow, sun-toned closed-period state,
      and the cancelled-session treatment — all via DS tokens (no raw hex in
      `features/attendance`).
- [x] typecheck, lint, vitest, and `attendance.spec.ts` pass.

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Unsaved toggles lost by navigation or a phone lock | Medium | High | `useBlocker` dirty guard; save is a single fast request so the exposure window is seconds, not minutes. |
| Teacher mis-ticks a same-name sibling | Medium | High | `display_note` promoted to a badge on duplicates; the confirm bar shows the absent count so a wrong count is noticeable before committing. |
| Fat-finger toggles on a scrolling list | Medium | Medium | 56px rows, `aria-pressed` state, and a visible absent count; toggling is free and reversible before confirm. |
| PUT overwrites a concurrent edit from another device | Low | Medium | Out of scope for V1 (single-teacher accounts). Note it; do not build locking. |
| Server roster includes students whose enrollment ended | Low | High | The UI trusts the server list; the e2e spec asserts an ended enrollment disappears from the next session's roster. |

**Rollback:** additive feature folder plus one route registration. Reverting
leaves phase 1 and 2 fully functional; no persisted client state and no schema
change.
