---
phase: 3
title: "Web nav and header cleanup"
status: done
priority: P2
effort: "2h"
dependencies: []
---

# Phase 3: Web nav and header cleanup

## Overview

Move the import entry point into the sidebar's "Trung tâm" group and tidy the
Lớp & học sinh header: import button removed, "⚙ Cài đặt lớp" promoted into the
header before "+ Tạo lớp mới". Independent of Phases 1–2.

## Requirements

- Functional:
  - Sidebar group "Trung tâm" gains owner-gated entry "Nhập từ Excel" →
    `/students/import`, placed inside the existing `isResolved && isOwner`
    spread next to "Duyệt giáo án" (`dashboard-layout.tsx:88-97`); order:
    Duyệt giáo án · Nhập từ Excel · Cài đặt trung tâm.
  - Add "Nhập từ Excel" to `OVERFLOW_LABELS` (`dashboard-layout.tsx:110`) so
    the <md bottom bar keeps five slots and the entry lives in the Thêm sheet.
  - `students-page.tsx`: delete the owner-gated "Nhập từ Excel" `HvButton`
    (lines 189-199); move the "⚙ Cài đặt lớp" `Link` (lines 255-262) into the
    header row before "+ Tạo lớp mới", still rendered only when
    `selectedClassId` is set; drop `ml-auto` (header uses `flex-1` on the h1).
  - Remove now-unused imports/vars from students-page (`useCenter`/`isOwner`,
    `useNavigate`/`navigate`) ONLY if nothing else on the page uses them —
    verify with tsc/eslint, do not guess.
- Non-functional: `/students/import` route and its own owner gate are
  untouched — deep links keep working for owners; members hitting the route
  still get the existing "chỉ chủ trung tâm" card.

## Architecture

Pure presentational move; no data-layer change. Icon: pick a lucide icon
consistent with neighbors (e.g. `FileSpreadsheetIcon` or `UploadIcon`) —
match the import set already used in `dashboard-layout.tsx`.

## Related Code Files

- Modify: `apps/web/src/layouts/dashboard-layout.tsx`
- Modify: `apps/web/src/features/roster/pages/students-page.tsx`
- Modify: `apps/web/src/layouts/__tests__/dashboard-layout.test.tsx`
  (owner-view group expectation at line 76 area + member-view absence)
- Modify: students-page tests if they assert the import button/settings link
  position (scout `apps/web/src/features/roster/__tests__/` first)

## Implementation Steps

1. Scout tests referencing "Nhập từ Excel" and "Cài đặt lớp" to know the
   assertion surface before touching markup.
2. Edit `dashboard-layout.tsx` (entry + OVERFLOW_LABELS).
3. Edit `students-page.tsx` (remove button, relocate settings link, prune
   dead imports).
4. Update layout + page tests: owner sees the nav entry, member does not;
   header renders settings link only with a selected class, in the new order.
5. `tsc -b --noEmit` + focused vitest, then full web suite.

## Success Criteria

- [ ] Owner sidebar: Trung tâm = Duyệt giáo án · Nhập từ Excel · Cài đặt trung tâm;
      member sidebar unchanged.
- [ ] Header order: [⚙ Cài đặt lớp?] [+ Tạo lớp mới] [+ Thêm học sinh]; no
      import button on the page.
- [ ] No dead imports/vars; lint + typecheck + web suite green.

## Risk Assessment

- Mobile bottom bar overflow: forgetting `OVERFLOW_LABELS` puts six items in
  the bar at 360px — the label must be added in the same commit.
- Settings link relies on `selectedClassId`; in the header it must not render
  a dead link when the page is on the "Chưa ghi danh" tab (same guard as today).
