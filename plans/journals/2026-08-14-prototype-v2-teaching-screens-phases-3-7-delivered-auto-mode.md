---
title: Prototype v2 teaching screens — phases 3-7 delivered (auto mode)
date: 2026-08-14
summary: "Four teaching screens shipped UI-only in apps/web on the HV design system; 336/336 tests, eslint/tsc/build clean"
---

# Prototype v2 teaching screens — phases 3-7 delivered (auto mode)

## What happened

Executed phases 3–7 of `plans/260813-2128-prototype-v2-teaching-screens/` in
`--auto` mode. Four screens now live in `apps/web`: Quản lý lớp học (stats,
sessions table, session detail panel, CSV), Chương trình & giáo án (curriculum
editor + plan submit flow), Hồ sơ học sinh (+ per-student detail with score
chart and inline notes), and owner-gated Duyệt giáo án (approve / yêu cầu sửa /
mở lại / nhắc). UI-only: real APIs for lớp/học sinh/buổi/điểm danh/ghi danh;
scores, giáo án, curriculum, nhận xét in the client-side teaching store
(`src/features/teaching/lib/teaching-store.ts`, localStorage per center name).

Gates: 336/336 tests (53 files), eslint 0 errors, `tsc -b` + build clean, DS
token grep clean. Build initially failed on `vi.fn(() => "blob:x")` mocks —
`tsc -b` type-checks test files, unlike `tsc --noEmit` on src; fixed with
`vi.fn<(blob: Blob) => string>()` generics.

## Decision

- Added `redo:reopen→pending` to the plan status machine (9 legal / 16 illegal)
  — the prototype's `showReopen` covers redo too; phase-02's original table was
  incomplete. Owner withdrawing their own redo request is legal.
- Next lesson derives from held-session count via `nextLessonIndex()` shared by
  teacher and owner views, so both write/read the same `lessonPlanKey` —
  `currentIndex` in the store is not display-driving.
- Toast/subtitle copy stays honest where the prototype fakes it (no "đã gửi
  Zalo", no fabricated timestamps).
- Review queue table refactored to the card-internal `overflow-x-auto` +
  `min-w-[440px]` pattern used by sessions-table, so narrow viewports scroll
  the table, not the page.

## Next steps

- Manual 3-breakpoint responsive pass in a real browser (code-inspected only).
- LÃI/LỖ cost stays the `SESSION_COST_VND` 300.000đ constant until product
  decides on a center setting.
- Device-local review loop (teacher↔owner same browser) is the accepted
  trade-off until a backend for teaching data exists.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
