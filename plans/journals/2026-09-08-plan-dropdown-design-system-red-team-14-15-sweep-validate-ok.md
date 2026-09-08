---
title: "Plan dropdown-design-system: red-team 14/15, sweep, validate OK"
date: 2026-09-08
summary: "Lập plan đồng bộ 10 dropdown apps/web theo DS dropdown Lớp (/records); red-team hard mode 29→15 findings, 14 áp dụng; handoff sang cook"
---

# Plan dropdown-design-system: red-team 14/15, sweep, validate OK

## What happened
- Lập plan `plans/260908-0832-dropdown-design-system/` (plan.md + 5 phase): trích `RecordsClassSelect` thành primitive `HvSelect` trong `components/hv/`, chuyển 4 Radix Select + 4 native `<select>` sang cùng component, xoá `components/ui/select.tsx`.
- Red-team hard mode với 3 reviewer (security/contract, assumptions/fact-check, failure/flow): 29 finding thô → 15 sau khử trùng; 14 chấp nhận, 1 từ chối (placeholder không mất đường reset — không test nào chọn `""`).
- Sửa chính sau red-team: `center-page.test.tsx` từ "không đụng" sang Modify (3 `selectOptions` + 3 `findByRole("dialog")` trần); option chuẩn không có separator ` · ` (accessible name = label+meta nối liền) → `classbook-page.test.tsx:444` phải sửa; xoá `ui/select.tsx` dời sang Phase 5 vì `class-select.tsx` (Phase 2) cũng import; `min-w-[230px]` tách khỏi class cơ sở trigger (twMerge không gộp min-w với w); nested `HvModal` chưa có tiền lệ → Phase 1 chứng minh bằng test, mitigation `stopPropagation` bị loại (Radix nghe Esc bằng capture listener trên document); D7 buộc guard `next === value` ở consumer có side effect ghi; e2e đọc giá trị qua `data-value`/text trigger thay `inputValue()`.
- Whole-plan consistency sweep: 5 stale reference reconciled, 0 mâu thuẫn. `ak plan validate` OK. Effort 20h → 23h.

## Decision
- Giữ nguyên markup option của chuẩn `/records` (không thêm ` · ` để chiều test).
- Bàn giao giữ ` (chủ trung tâm)` trong `label`, không chuyển sang `meta` (e2e `ensureClassTeacher` tìm theo chuỗi này).
- Task hydration bỏ qua: session không có công cụ task surface.

## Next steps
- `/ak:cook plans/260908-0832-dropdown-design-system/plan.md` — Phase 1 trước, Phase 2/3/4 song song, Phase 5 gom.
- Khi implement: xác nhận prop `updatePositionStrategy` tồn tại trong `radix-ui` đang cài (session lập plan bị chặn đọc `node_modules`).
- Legacy script `engineer/.agentkit/scripts/set-active-plan.cjs` hỏng (thiếu `ck-config-utils.cjs`); `ak plan use` đã thay thế — không sửa trong scope này.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
