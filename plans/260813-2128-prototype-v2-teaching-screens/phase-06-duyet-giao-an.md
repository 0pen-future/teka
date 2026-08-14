---
phase: 6
title: "Duyệt giáo án"
status: completed
priority: P1
effort: "0.5d"
dependencies: [1, 4]
---

# Phase 6: Duyệt giáo án

## Overview

Build the owner-only `/lesson-plans` review screen (prototype lines 581–644): the review queue across all classes and the detail panel with approve / request-changes / reopen / remind actions. Closes the review loop whose teacher half is Phase 4.

## Requirements

- Functional — queue table (grid `110px 150px 1fr 110px`): LỚP / GIÁO VIÊN / GIÁO ÁN BUỔI TỚI / TRẠNG THÁI chip. One row per class: its next lesson + current plan status from the store (`none` renders as "Chưa nộp"). Row click selects into the detail panel. Sub-line under the h1 summarizes pending count (prototype `plans.sub`).
- Functional — detail panel (sky-300 header: class + teacher): plan eyebrow + status chip; title, goal, bullet activities, BTVN box, optional `draftBy` line and 📎 file name; when status `redo`, coral "Yêu cầu sửa" box with the note.
- Functional — states/actions per prototype status machine:
  - `none`/`draft` (not submitted): cream info box "Chưa có giáo án để duyệt…" + `Nhắc giáo viên nộp qua Zalo` outline-sky button → hvToast confirmation only (no real Zalo send — UI-only constraint; toast copy must not claim delivery, e.g. "Đã tạo lời nhắc").
  - `pending`: NHẬN XÉT CỦA CHỦ TRUNG TÂM textarea + `Duyệt giáo án` (press-mint) and `Yêu cầu sửa` (coral outline). Approve stores the comment as `ownerComment`; **Yêu cầu sửa requires a non-empty comment** (prototype behavior) — disabled state + hint otherwise; sets `redo` + `redoNote`.
  - `approved`/`redo`: chip reflects state; `Mở lại để duyệt lại` link (sky) → back to `pending`.
- Functional — teacher name per class: from real data when the web app already exposes class-teacher assignment; otherwise from the plan's `submittedBy` (captured at submit in Phase 4), falling back to "—". No new endpoints.
- Non-functional: all transitions go through the shared store transition function (Phase 4); nav pending dot decrements live on approve/redo; owner gate from Phase 1 verified here with tests.

## Architecture

- `lesson-plans-page.tsx` composes `review-queue-table.tsx` + `plan-review-panel.tsx`; the plan body rendering (title/goal/activities/BTVN/file) is the same visual block Phase 3's detail "Giáo án" tab uses — extract `plan-summary.tsx` into `features/teaching/components/` and reuse in both (three usages by now: classbook tab, next-plan card variant, review panel — dedupe is justified).
- Queue derives from `listClasses` × store plan slices; selection state is local (first actionable row preselected, like the prototype).
- Actions are thin handlers over the store transition function; illegal transitions are unreachable from the UI but still guarded in the store (defense in one layer, not scattered `if`s).

## Related Code Files

- Modify: `apps/web/src/features/teaching/pages/lesson-plans-page.tsx`
- Create: `apps/web/src/features/teaching/components/review-queue-table.tsx`, `plan-review-panel.tsx`, `plan-summary.tsx` (extracted; refactor Phase 3/4 usages)
- Reference: `apps/web/src/features/teaching/lib/teaching-store.ts` (transition function), `apps/web/src/components/hv/` (StatusPill, HvButton, hvToast)

## Implementation Steps

1. Extract `plan-summary.tsx` from the Phase 3/4 plan-rendering blocks; verify their tests still pass.
2. Build queue table with per-class status chips and selection.
3. Build review panel: three action states, comment requirement on Yêu cầu sửa, reopen link, remind toast.
4. Tests: owner sees the page, member is redirected (msw center shapes); approve/redo/reopen round-trip through the store and update chips + nav dot; redo without comment is blocked; teacher-side redo note appears on `/classbook` after owner action (integration-style test across the two pages sharing the store).

## Success Criteria

- [x] Screen matches prototype layout for all four status states.
- [x] Yêu cầu sửa requires a comment; approve/reopen/remind behave per prototype; toast copy does not claim a real Zalo message was sent.
- [x] Full loop verified: submit (Phase 4) → pending here → redo note visible to teacher → resubmit → approve.
- [x] Owner gating tested at route level; suite green.

## Risk Assessment

- **Remind button implies real messaging** → honest toast copy (see above) and a note in the phase report; a real Zalo reminder is backend scope, explicitly out.
- **Store-only "teacher" attribution may be wrong for multi-teacher centers** → prefer real assignment data when present; `submittedBy` fallback is labeled as such in code review notes, not silently authoritative.
