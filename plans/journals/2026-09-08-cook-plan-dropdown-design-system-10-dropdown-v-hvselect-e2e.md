---
title: "Cook plan dropdown-design-system: 10 dropdown về HvSelect, e2e cách ly, QA 10/10"
date: 2026-09-08
summary: "Thực thi 5 phase plan 260908-0832: primitive HvSelect + 9 consumer, xoá ui/select.tsx, e2e trên stack teka-e2e, QA thị giác 30 ảnh, review nhánh đóng M1"
---

# Cook plan dropdown-design-system: 10 dropdown về HvSelect, e2e cách ly, QA 10/10

## What happened
- Thực thi plan `plans/260908-0832-dropdown-design-system/` ở chế độ cook interactive: Phase 1 primitive `HvSelect` (`apps/web/src/components/hv/hv-select.tsx`), Phase 2/3/4 chạy song song bằng 3 fullstack-developer với file ownership tách biệt, Phase 5 xoá `components/ui/select.tsx`, gate, e2e, docs, QA.
- Gate `apps/web`: typecheck, lint (0 lỗi / 5 warning có sẵn), vitest 85 file / 660 pass / 3 skip, format:check, build xanh.
- e2e phải chạy trên stack cách ly `docker compose -p teka-e2e` (port 55432/58080/55173) vì `teka-*` là production: 8 passed, rerun `class-staff-write` + `secretary-send` 4 passed (idempotent). Đã `down -v`.
- QA thị giác bằng script Playwright riêng (`plans/reports/assets/qa-260908-dropdown-design-system/qa-dropdowns.mjs`): 10 vị trí × 3 ảnh, tất cả khớp chuẩn dropdown Lớp ở `/records`. Lỗi lần đầu ở 3 vị trí là do seed (kỳ thu của Cô Lan không có hoá đơn, "Ghi danh" chỉ ở `?tab=unenrolled`, "+ Thêm học vụ" disabled ở Toán 8) — đổi sang Thầy Minh / lớp Lý 7, không phải bug UI.
- Review nhánh: M1 — test "(D7)" ở override picker của dialog phân quyền không chứng minh guard vì `dirty` so với giá trị server. Đóng bằng cách bỏ guard (plan đã nói consumer thuần state không cần guard; guard chỉ che tập con vì chọn khác rồi chọn lại vẫn làm `modes` non-null), giữ test làm regression hành vi. L2 `searchNoun` cho vai trò/chế độ/hình thức, L4 câu chữ docs đã sửa; L1/L3/L5–L8 ghi vào plan.

## Decision
- `HvSelect` là dropdown chọn giá trị duy nhất trong `apps/web`; `dropdown-menu.tsx` (menu hành động, chưa mount) vẫn tồn tại — docs gọi HvSelect là "select-style dropdown", không phải "the only dropdown".
- Radix Popover content mang `role="dialog"` là đặc tính của chuẩn gốc, giữ nguyên; trigger vẫn `aria-haspopup="listbox"`.

## Next steps
- Mở PR `feat/dropdown-design-system` → `master` (push cần user duyệt; gh chỉ pull-only).
- Nếu muốn dọn Low: L7 gộp `useOptionSearch` với `use-class-search.ts`, L8 thêm test đúng 5 mục và lọc trong sheet.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
