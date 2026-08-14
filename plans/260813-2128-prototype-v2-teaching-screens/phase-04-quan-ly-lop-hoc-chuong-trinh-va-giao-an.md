---
phase: 4
title: "Quản lý lớp học — chương trình & giáo án"
status: completed
priority: P1
effort: "1d"
dependencies: [1, 3]
---

# Phase 4: Quản lý lớp học — chương trình & giáo án

## Overview

Build the second view tab of `/classbook` (prototype lines 293–392): the CHƯƠNG TRÌNH progress card with curriculum editor, the GIÁO ÁN BUỔI TỚI card with plan editor + submit-for-review, and the SĨ SỐ THEO THÁNG mini chart. This is the teacher half of the lesson-plan review loop that Phase 6 closes.

## Requirements

- Functional — CHƯƠNG TRÌNH card (flex 1.7): eyebrow + course name + `Buổi X/N` mint counter; 10px pill ProgressBar; "Đang học: **<bài>**" + "Buổi tới: <bài>" lines; toggle link to expand the 2-column lesson list (done/current/future styles); `✎ Sửa chương trình` link opening the curriculum editor modal.
- Functional — curriculum editor modal (HvModal, 600px): numbered rows of text inputs, ✕ remove per row (coral), dashed `+ Thêm bài` button, Hủy / `Lưu chương trình` (press-mint). Sub-line: "Sửa tên bài, thêm hoặc bớt buổi — số bài quyết định độ dài khóa."
- Functional — GIÁO ÁN BUỔI TỚI card: eyebrow + status chip (`none|draft|pending|approved|redo`); next-lesson title; when status is `redo`, coral box "**Chủ trung tâm yêu cầu sửa:** <note>" from the store; optional 📎 file line; `✎ Soạn giáo án trực tiếp` (sky) opening the plan editor; dashed file-attach label (accept `.doc,.docx,.pdf`, stores **name only** — no upload, constraint is UI-only); `Nộp duyệt giáo án` press-mint button visible when a draft/redo plan is submittable → sets status `pending` (feeds the Phase 1 nav dot and Phase 6 queue).
- Functional — plan editor modal (580px): "Soạn giáo án — <bài>" title; sub-line noting the owner sees it verbatim in Duyệt giáo án; fields Mục tiêu buổi học (textarea 2), Hoạt động trên lớp (textarea 5, one activity per line), Bài tập về nhà (input); Hủy / `Lưu giáo án` → status `draft` (or keeps `redo` context until resubmitted — mirror the prototype's status machine: save from `redo` stays submittable).
- Functional — SĨ SỐ THEO THÁNG card: last ~5 months of bar columns (count label / mint bar height-scaled / month label) derived from enrollment `started_on`/`ended_on` (both verified on `enrollmentSchema`); mint summary line (`retLine`) about retention. <!-- Updated: Validation Session 1 - enrollment date fields confirmed -->
- Non-functional: curriculum & plan state only in the teaching store; chart math pure and unit-tested; modals keyboard-dismissable and focus-trapped (HvModal is radix Dialog — both built-in). HvModal defaults to `sm:max-w-md` (~448px): pass a className width override for the 580–600px prototype editors.

## Architecture

- Components in `features/teaching/components/`: `curriculum-card.tsx`, `next-plan-card.tsx`, `monthly-headcount-card.tsx`, plus `plan-editor-modal.tsx` and `curriculum-editor-modal.tsx`. The page's view-tab switch (built in Phase 3) just swaps the composed view.
- **Plan status machine lives in the store module** (Phase 1 shape): transitions `draft→pending` (submit), `pending→approved|redo` (owner, Phase 6), `redo→pending` (resubmit), `approved→pending` (reopen, Phase 6). Encode as an explicit transition function with a unit test enumerating legal/illegal moves — both this phase and Phase 6 call it, so the invariant sits in one place instead of two pages.
- Current/next lesson derive from `curriculum.currentIndex`; "Đã dạy xong bài này" style advance (prototype `onToggleCur`) moves the index and re-targets the next-plan card.
- Monthly headcount: pure `lib/monthly-headcount.ts` over enrollment records already fetched for the class (no new endpoints).

## Related Code Files

- Modify: `apps/web/src/features/teaching/pages/classbook-page.tsx` (mount second view)
- Create: `apps/web/src/features/teaching/components/curriculum-card.tsx`, `next-plan-card.tsx`, `monthly-headcount-card.tsx`, `plan-editor-modal.tsx`, `curriculum-editor-modal.tsx`
- Create: `apps/web/src/features/teaching/lib/monthly-headcount.ts` (+ test)
- Modify: `apps/web/src/features/teaching/lib/teaching-store.ts` (plan transition function + tests, if not fully shaped in Phase 1)

## Implementation Steps

1. Implement/finalize the plan status transition function + unit tests in the store module.
2. Build curriculum card + editor modal; wire lesson list edit/add/remove and progress display.
3. Build next-plan card + plan editor modal; wire draft save, file-name attach, submit-to-pending, redo-note display.
4. Build monthly headcount card from pure derivation + retention line.
5. msw/component tests: editing curriculum updates progress; submit flips chip to "Chờ duyệt" and increments the nav pending dot; redo note renders when store holds a redo state.

## Success Criteria

- [x] Both cards + chart match prototype layout, chips, and interaction styles.
- [x] Status machine transitions verified by unit tests; illegal transitions rejected.
- [x] Submit-for-review is visible to Phase 6's queue and the nav dot without a reload.
- [x] Suite green.

## Risk Assessment

- **File attach expectation** — users may assume the file uploads; the UI stores the name only. Keep the prototype's copy, and state the limitation in the phase report + release note. If this is unacceptable product-wise, it becomes an API feature (out of scope by constraint).
- **Status machine divergence between teacher/owner screens** → single transition function shared by both (see Architecture).
- **currentIndex drift after curriculum edits** (removing lessons above the index) → clamp index inside the store update; unit test the edge.
