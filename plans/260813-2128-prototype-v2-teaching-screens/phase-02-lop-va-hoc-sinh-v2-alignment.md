---
phase: 2
title: "Lớp & học sinh v2 alignment"
status: completed
priority: P1
effort: "0.5d"
dependencies: [1]
---

# Phase 2: Lớp & học sinh v2 alignment

## Overview

Align the existing roster students page (`/students`) with the Prototype v2 "Lớp và học sinh" screen (extract lines 216–259) without changing roster behavior or API usage.

## Requirements

- Functional (per prototype): header row = Baloo-2 26px title + `+ Tạo lớp mới` (HvButton secondary) + `+ Thêm học sinh` (HvButton primary); data-minimization subtitle "Chỉ lưu: họ tên · ngày nhập học · lớp · người liên hệ. Không thu thập gì thêm."; `CHỌN LỚP` eyebrow label; class pill tabs + `Tìm lớp…` pill search shown when class count warrants it; `⚙ Cài đặt lớp` outline-pill button (right-aligned) linking to the existing `classes/:id/settings` route for the selected class.
- Functional: table card (radius 20, `--shadow-md`, sticky `--cream-200` header) with columns HỌC SINH / NGƯỜI LIÊN HỆ (name + phone stacked) / NHẬP HỌC / BUỔI THÁNG NÀY / actions (status badges + mint press-shadow `Ghi danh vào lớp` button where applicable). Prototype's "BUỔI T7" is its current-month session count — render the current month dynamically. Source (validated): one `listClassSessions(selectedClassId, current month)` query for the selected class, query key shared with the classbook feature so React Query dedupes; per-student count = sessions in the month falling inside the student's enrollment window (`started_on`/`ended_on`) — no per-session roster fan-out. <!-- Updated: Validation Session 1 - BUỔI T7 single-query source -->

- Non-functional: keep all existing roster capabilities (create student/class flows, enrollment, links to detail) and keep existing roster tests passing — this is a re-skin/re-layout, not a rewrite.

## Architecture

- This phase edits `apps/web/src/features/roster/pages/students-page.tsx` (404 lines) in place; no new feature code. Extract subcomponents inside the roster feature only where the page's existing structure already suggests a boundary (e.g. a `ClassTabs` shared later by classbook/records is tempting, but classbook/records live in `teaching` — build the shared pill-tab component in `components/hv/` if and only if Phase 3 confirms the same markup; otherwise keep local. YAGNI first, dedupe in Phase 7 if three usages materialize).
- Data stays on existing hooks (`listClasses`, students queries, enrollments) plus one `listClassSessions` query for the selected class + current month (validated decision — see plan.md Validation Log Q3). No other new fetches; never N+1 per student.

## Related Code Files

- Modify: `apps/web/src/features/roster/pages/students-page.tsx`
- Modify: roster page tests (`apps/web/src/features/roster/__tests__/…`) for changed structure/labels
- Possibly create: `apps/web/src/components/hv/pill-tabs.tsx` (only if shared with Phase 3 — see Architecture)

## Implementation Steps

1. Read the current students page + its tests fully; map every existing behavior to a slot in the v2 layout before editing.
2. Rebuild header + CHỌN LỚP row per prototype markup (tokens/hv components only).
3. Restyle the table to the v2 grid (`2fr 2fr 1.1fr 1fr 1.6fr`), stacked contact cell, badges + Ghi danh action with `--press-mint` interaction.
4. Wire `⚙ Cài đặt lớp` to the selected class's settings route; hide when no class is selected.
5. Update tests: assert new column headers, subtitle, settings link target; keep behavioral tests (add student, enroll) untouched and green.

## Success Criteria

- [x] Screen matches v2 layout (header, subtitle, tabs, table grid, action styles) using tokens only.
- [x] All pre-existing roster tests pass (updated only where markup/labels changed).
- [x] Only one new query (`listClassSessions` for the selected class + current month); no per-student request fan-out.

## Risk Assessment

- **Hidden coupling in the 404-line page** (selection state, dialogs) → step 1's full read before editing; refactor minimally.
- **BUỔI T7 counting nuance** — enrollment-window counting shows scheduled sessions, not attended ones; that matches the prototype's roster-page intent (workload/coverage), and attendance detail lives on the classbook/records screens.
