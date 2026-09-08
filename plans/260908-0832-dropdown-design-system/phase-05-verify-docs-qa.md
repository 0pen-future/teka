---
phase: 5
title: "Xoá ui/select.tsx, verify toàn bộ, e2e cách ly, docs, QA thị giác"
status: completed
priority: P1
effort: "4h"
dependencies: [2, 3, 4]
---

# Phase 5: Xoá `ui/select.tsx`, verify toàn bộ, e2e cách ly, docs, QA thị giác

## Overview

Xoá `components/ui/select.tsx` khi Phase 2 **và** 3 đã merge (D8), chạy toàn bộ gate của `apps/web`, e2e liên quan trên stack cách ly, cập nhật `docs/frontend-guidelines.md`, và chụp màn hình 10 vị trí ở desktop + mobile để so với chuẩn `/records`. Kết thúc bằng review code (cross-module, đổi primitive dùng chung).

## Requirements

- Functional: 10 vị trí đều là `HvSelect`; không còn `ui/select.tsx`, không còn `<select>`.
- Non-functional: toàn bộ gate xanh; docs đúng với source; báo cáo QA có ảnh trước/sau.

## Architecture

Không đổi code sản phẩm ngoài sửa lỗi phát hiện ở gate. Stack e2e cách ly: `docker compose -p teka-e2e` với override cổng/URL như đã ghi ở memory dự án (không chạy trên `teka-*` production containers).

## Related Code Files

- Modify: `docs/frontend-guidelines.md` (mục *hv kit* → thêm dòng `HvSelect` (combobox mở listbox với roving focus: popover ≥ `sm`, bottom sheet < `sm`, ô lọc khi >5 mục; đọc giá trị qua text trigger/`data-value`); mục *Testing* → một câu: dropdown chọn bằng click combobox + option, dùng `mockViewport(1024)` cho nhánh popover)
- Create: `plans/reports/qa-<YYMMDD-HHmm>-dropdown-design-system.md` (ảnh + checklist), `plans/reports/code-review-<YYMMDD-HHmm>-dropdown-design-system.md`
- Delete: `apps/web/src/components/ui/select.tsx` (chỉ sau khi `grep -rn "components/ui/select" apps/web/src` = 0)
- Read: toàn bộ file đã đổi ở Phase 1–4

## Implementation Steps

1. `grep -rn "components/ui/select" apps/web/src` = 0 → `git rm apps/web/src/components/ui/select.tsx`; kiểm `eslint.config.js` và `components.json` không tham chiếu file (đã xác nhận không có lúc lập plan). Commit riêng `chore(web): drop unused shadcn select`.
2. `cd apps/web && npm run lint && npm run typecheck && npm run test && npm run build`.
3. Dựng stack e2e cách ly; `npx playwright test e2e/records-search.spec.ts e2e/class-staff-read.spec.ts e2e/class-staff-write.spec.ts e2e/secretary-send.spec.ts e2e/roster.spec.ts`.
4. QA thị giác (Playwright screenshot hoặc trình duyệt): với mỗi vị trí 1–10, chụp trigger đóng, popover mở (1024px) và sheet mở (375px); riêng `/records` chụp thêm trạng thái rỗng (placeholder "Chọn lớp" nay là `font-bold text-ink-400` thay vì `font-extrabold text-ink-900` của bản cũ — thay đổi cố ý theo `data-placeholder:*`); xác nhận bằng mắt CSS của `data-placeholder:*`, `aria-invalid:*`, `aria-disabled:*`, `max-h-(--radix-popover-content-available-height)` (review Phase 1 không đọc được `dist`); đặt cạnh ảnh `/records` chuẩn; ghi khác biệt token nếu có → sửa ở `hv-select.tsx` (không sửa từng consumer).
5. Kiểm tra bàn phím ở 3 vị trí đại diện (records, audit Giáo viên, override quyền trong dialog): Tab tới trigger, Enter mở, ↑↓, Enter chọn, Esc đóng, focus về trigger.
6. Cập nhật `docs/frontend-guidelines.md`; đối chiếu từng câu với source.
7. Spawn `code-reviewer` cho diff toàn nhánh; sửa finding High/Medium; ghi báo cáo.
8. Cập nhật mục *Kết quả* của `plan.md` (gate, lệch so với plan) và đóng plan bằng CLI.

## Success Criteria

- [x] `components/ui/select.tsx` không còn; `grep -rn "ui/select" apps/web/src` = 0.
- [x] 4 gate `apps/web` xanh; 5 spec e2e xanh trên stack cách ly (ghi số pass/skip vào plan); `class-staff-write` và `secretary-send` chạy 2 lần liên tiếp đều xanh (idempotent).
- [x] Báo cáo QA có 10 × 3 ảnh, mỗi vị trí ghi "khớp chuẩn" hoặc khác biệt đã sửa.
- [x] `docs/frontend-guidelines.md` nhắc `HvSelect`, không nhắc `components/ui/select`.
- [x] Code review không còn finding High/Medium mở.

## Risk Assessment

- **e2e đỏ sẵn trên master (plan trước ghi assertion classbook e2e đỏ trước khi sửa):** chạy spec trên master trước để có baseline; chỉ nhận trách nhiệm với spec đổi từ xanh sang đỏ.
- **Khác biệt pixel do font/OS khi so ảnh:** so token/kích thước bằng DevTools thay vì pixel-diff; ảnh chỉ để người xem duyệt.
- **Gate `simplify`/`workflow_artifact_gate` của repo yêu cầu artifact:** báo cáo QA + review đặt ở `plans/reports/` là artifact; không tạo thêm.
