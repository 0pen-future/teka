---
phase: 3
title: "Quản lý lớp học — buổi học & nhận xét"
status: completed
priority: P1
effort: "1d"
dependencies: [1]
---

# Phase 3: Quản lý lớp học — buổi học & nhận xét

## Overview

Build the default view of `/classbook` (prototype lines 263–292, 393–481): class tabs, 5 stat cards, the sessions table, the session detail panel with Nhận xét / Giáo án / Điểm tabs, and the class CSV export.

## Requirements

- Functional — page chrome: title + subtitle ("Sĩ số, điểm trung bình, doanh thu và giáo án từng buổi…"), `Tải dữ liệu lớp (CSV)` outline button, class search + pill tabs, view tabs `Buổi học & nhận xét` / `Chương trình & giáo án` (underline-tab style on a 1.5px `--line-200` border).
- Functional — stat cards (5-col grid, radius 20, `--shadow-md`): SĨ SỐ (active enrollments), CHUYÊN CẦN (% present over confirmed sessions), TÁI TỤC (retention: students continuing month-over-month from enrollment data), ĐIỂM TB (mean of stored scores), LÃI/LỖ (sum over confirmed sessions of present students' real enrollment `unit_price` − `SESSION_COST_VND`; validated — see plan.md Validation Log Q2). <!-- Updated: Validation Session 1 - revenue uses enrollment.unit_price --> Each card = label / Baloo value / sub line.
- Functional — sessions table (grid `78px 92px 62px 64px 104px 1fr`): BUỔI (label + date), GIÁO ÁN (StatusPill-style chip from plan status machine `none|draft|pending|approved|redo`), SĨ SỐ (present/total once confirmed), ĐIỂM TB (session mean from store), DOANH THU (mint positive / coral negative), NHẬN XÉT CHUNG (ellipsized store note). Footnote line quoting the 300.000đ cost rule verbatim from the prototype. Row click opens the detail panel.
- Functional — detail panel (mint header card, `--shadow-lg`): title + chips (sĩ số, điểm TB, doanh thu), segmented tabs in a `--cream-200` pill: **Nhận xét** (textarea + `Lưu nhận xét` press button + saved status), **Giáo án** (read-only plan: chip, title, goal, bullet activities, BTVN box — reads the same store slice Phase 4 writes), **Điểm** (per-present-student 0–10 step-0.5 numeric inputs, `Lưu điểm buổi` + status; absent students show a mark, not an input).
- Functional — CSV export: BOM + semicolon-separated (prototype's exact encoding), one row per session with stats + note, downloaded via blob; filename includes class name.
- Non-functional: sessions/roster come from real APIs — `listClassSessions(classId, {from,to})` (mind the 400-day cap; query a bounded window such as the last 6 months) and `getSessionRoster` for the selected session. Scores/notes never hit the network. React Query for server data, teaching store for local data; no duplication of server state into the store.

## Architecture

- `classbook-page.tsx` owns: selected class id (URL search param, so refresh/deep-link keeps context), selected view tab, selected session id. Subcomponents in `features/teaching/components/`: `class-stat-cards`, `sessions-table`, `session-detail-panel`.
- Stats derivation is pure functions in `lib/classbook-stats.ts` (input: sessions + rosters + store slice → output: the five stat values and per-session rows). Pure module = unit-testable without msw; the page stays a thin composition. This is the same separation the billing feature uses for period math.
- Per-session roster fan-out trade-off: the table needs SĨ SỐ per session. Confirmed attendance counts should come from the cheapest available source — check whether the sessions list payload already carries attendance summary; if not, fetch rosters lazily (React Query per session, cached) only for sessions in the current view window rather than eagerly for all. Cap concurrency via query dedupe; document the choice in code only if a constraint is non-obvious.
- CSV: small `lib/csv.ts` helper (BOM, semicolons, quoting) shared by Phases 3 and 5.

## Related Code Files

- Modify: `apps/web/src/features/teaching/pages/classbook-page.tsx`
- Create: `apps/web/src/features/teaching/components/class-stat-cards.tsx`, `sessions-table.tsx`, `session-detail-panel.tsx`
- Create: `apps/web/src/features/teaching/lib/classbook-stats.ts`, `apps/web/src/features/teaching/lib/csv.ts` (+ unit tests)
- Reference: `apps/web/src/features/attendance/api/attendance-api.ts`, `apps/web/src/features/roster/api/classes-api.ts`, `apps/web/src/components/hv/`

## Implementation Steps

1. Implement `csv.ts` + `classbook-stats.ts` with unit tests first (pure logic: attendance %, retention, revenue, means).
2. Build page chrome: class tabs from `listClasses`, view-tab state, URL param sync.
3. Build stat cards + sessions table from real session data joined with store slices.
4. Build detail panel with the three tabs; wire store writes (note, scores) with saved-status feedback per prototype.
5. CSV export button producing the BOM/semicolon file.
6. msw tests: page renders stats and rows from mocked classes/sessions/rosters; score save round-trips through the store; export produces expected content (assert blob text, not download).

## Success Criteria

- [x] View matches prototype layout/styling; plan-status chips render all five states.
- [x] Stats and revenue math verified by unit tests, including the −300.000đ rule and negative (coral) rendering.
- [x] Detail tabs read/write the store; data survives reload.
- [x] No unbounded request fan-out; suite green.

## Risk Assessment

- **Roster fan-out for SĨ SỐ/attendance stats** → lazy per-session queries + windowed session range; verified against msw handler call counts in tests.
- **Store/API join bugs on session ids** → stats module takes ids as opaque strings; unit tests cover missing-score and unconfirmed-session cases.
- Fee source verified: `enrollmentSchema.unit_price` (int đồng, `roster-schemas.ts:262-272`) via `listEnrollments` — the earlier degrade-to-"—" contingency is removed. <!-- Updated: Validation Session 1 - fee source confirmed -->
