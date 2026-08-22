---
title: Trang "Tổng quan" theo prototype Sổ Lớp
date: 2026-08-06
summary: "Rebuilt the dashboard home to match the design prototype: merged pending-attendance banner, four stat cards, and a per-class progress grid aggregated from existing endpoints."
---

# Trang "Tổng quan" theo prototype Sổ Lớp

## What happened

Executed `plans/260806-1306-tong-quan-dashboard`: imported the "Tổng quan" screen from the claude.ai/design project via DesignSync and rebuilt `DashboardPage` — time-of-day greeting h1, one merged coral warning banner linking to the first pending session, four stat cards (students, period attendance %, amount due via the side-effect-free preview read, collected totals), and the "Lớp của bạn" grid of class cards deep-linking to `/sessions?class_id=` (new read-only param on the sessions page).

No backend changes: stats aggregate from `/students`, `/classes`, `/classes/:id/sessions`, `/sessions/pending`, `/billing-periods` (+`/preview`), and `/collections/summary` through `useQueries` fan-out, deduped by shared react-query keys.

## Decisions

- Fetch per-class sessions for `[period_start, min(today, period_end)]`, not the whole month — "Thiếu N" must only count taught sessions (mirrors the server's pending predicate) and the endpoint materializes rows for whatever range is requested.
- `GET /preview` directly from the dashboard instead of reusing billing's `getReview`, which POSTs a draft (persists invoices) on open periods — viewing the dashboard must be a pure read.
- Removed `PeriodStatusCard`: not part of the prototype screen; the "Chốt sổ" CTA now lives only in the sidebar. Flagged to the user as a reversible product call.
- Kept prototype semantics verbatim where review suggested otherwise: all-cancelled classes show "Lớp mới"; the student total includes unenrolled students.

## Review findings fixed

Code review returned DONE_WITH_CONCERNS; fixed before finishing: prettier failures (CI gate), future sessions counted as missing, and missing error states — every stat card and class card now renders a coral "Không tải được" instead of an eternal loading ellipsis or a fake 0/0. Also bounded the banner list to 3 + "… và N buổi khác", filtered closed schedule rows out of the timetable label, and switched the class count to `meta.total`.

Final gates: tsc clean, eslint 0 errors, prettier clean, vitest 26 files / 121 tests green. E2E specs updated for the new greeting/banner text but not run (needs the dev stack).
