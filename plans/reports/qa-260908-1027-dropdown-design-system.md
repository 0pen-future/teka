# QA thị giác + bàn phím — Dropdown design system (`HvSelect`)

- Plan: `plans/260908-0832-dropdown-design-system/` — Phase 5, bước 4–5.
- Nhánh: `feat/dropdown-design-system` @ `063c449` (+ docs chưa commit).
- Stack: `docker compose -p teka-e2e` (web `:55173`), seed mới; Chromium headless qua Playwright.
- Script: [`assets/qa-260908-dropdown-design-system/qa-dropdowns.mjs`](assets/qa-260908-dropdown-design-system/qa-dropdowns.mjs) (10 vị trí × 1024px/375px + bàn phím), [`assets/qa-260908-dropdown-design-system/qa-empty.mjs`](assets/qa-260908-dropdown-design-system/qa-empty.mjs) (trạng thái rỗng `/records`). Số liệu thô: [`assets/qa-260908-dropdown-design-system/results.json`](assets/qa-260908-dropdown-design-system/results.json).
- Ảnh tổng hợp: [đóng 1024](assets/qa-260908-dropdown-design-system/montage-closed.jpg) · [popover 1024](assets/qa-260908-dropdown-design-system/montage-popover.jpg) · [sheet 375](assets/qa-260908-dropdown-design-system/montage-sheet.jpg).

## Kết luận

**10/10 vị trí khớp chuẩn `/records` — không phải sửa gì trong `hv-select.tsx`.** Computed style của trigger, panel popover và option ở cả 10 vị trí **giống hệt** vị trí 1 (so sánh tự động từ `results.json`); khác biệt duy nhất là các trạng thái có chủ đích (placeholder, chưa chọn, focus) và `max-height` của popover theo chỗ trống thực tế. Bàn phím 3/3 vị trí PASS. E2e 8/8 (+ 4/4 lần chạy lại).

## 1. Ảnh 10 vị trí × 3

| # | Vị trí | 1024 đóng | 1024 popover | 375 sheet | Mục | Ô lọc | Kết quả |
|---|---|---|---|---|---|---|---|
| 1 | Hồ sơ học sinh — Lớp | [đóng](assets/qa-260908-dropdown-design-system/1-closed-1024.jpg) | [popover](assets/qa-260908-dropdown-design-system/1-popover-1024.jpg) | [sheet](assets/qa-260908-dropdown-design-system/1-sheet-375.jpg) | 4 | không | khớp chuẩn — Chuẩn tham chiếu. Nhóm "Lớp đang dạy", meta `· N HS` bên phải, ✓ mint ở lớp đang chọn. |
| 2 | Sổ lớp — Chọn lớp | [đóng](assets/qa-260908-dropdown-design-system/2-closed-1024.jpg) | [popover](assets/qa-260908-dropdown-design-system/2-popover-1024.jpg) | [sheet](assets/qa-260908-dropdown-design-system/2-sheet-375.jpg) | 4 | không | khớp chuẩn — Trigger `Tên · lịch` (meta mờ); vỏ popover/sheet trùng records. Viền 2px thay cho `border-0 shadow-soft-sm` cũ — cố ý theo DS. |
| 3 | Nhật ký — Giáo viên | [đóng](assets/qa-260908-dropdown-design-system/3-closed-1024.jpg) | [popover](assets/qa-260908-dropdown-design-system/3-popover-1024.jpg) | [sheet](assets/qa-260908-dropdown-design-system/3-sheet-375.jpg) | 4 | không | khớp chuẩn — Bốn mục, không ô lọc (≤5). |
| 4 | Nhật ký — Nhóm hành động | [đóng](assets/qa-260908-dropdown-design-system/4-closed-1024.jpg) | [popover](assets/qa-260908-dropdown-design-system/4-popover-1024.jpg) | [sheet](assets/qa-260908-dropdown-design-system/4-sheet-375.jpg) | 19 | có | khớp chuẩn — 19 mục → có ô lọc "Tìm nhóm hành động…" ở cả popover lẫn sheet. |
| 5 | Ghi nhận thanh toán — Hình thức | [đóng](assets/qa-260908-dropdown-design-system/5-closed-1024.jpg) | [popover](assets/qa-260908-dropdown-design-system/5-popover-1024.jpg) | [sheet](assets/qa-260908-dropdown-design-system/5-sheet-375.jpg) | 3 | không | khớp chuẩn — Trong `HvModal` "Ghi nhận thu" (đăng nhập Thầy Minh — kỳ có hoá đơn). Sheet lồng sheet ở 375px hiển thị đúng. |
| 6 | Ghi danh vào lớp — Lớp | [đóng](assets/qa-260908-dropdown-design-system/6-closed-1024.jpg) | [popover](assets/qa-260908-dropdown-design-system/6-popover-1024.jpg) | [sheet](assets/qa-260908-dropdown-design-system/6-sheet-375.jpg) | 4 | không | khớp chuẩn — Placeholder "Chọn lớp…" (`data-placeholder`: `font-bold text-ink-400`). Option `Tên` + meta `lịch · giá/buổi`. Chưa chọn → không có ✓. |
| 7 | Phân quyền — Vai trò | [đóng](assets/qa-260908-dropdown-design-system/7-closed-1024.jpg) | [popover](assets/qa-260908-dropdown-design-system/7-popover-1024.jpg) | [sheet](assets/qa-260908-dropdown-design-system/7-sheet-375.jpg) | 3 | không | khớp chuẩn — Trong dialog Phân quyền; giá trị hiện tại "Giáo viên" có ✓. |
| 8 | Phân quyền — từng quyền | [đóng](assets/qa-260908-dropdown-design-system/8-closed-1024.jpg) | [popover](assets/qa-260908-dropdown-design-system/8-popover-1024.jpg) | [sheet](assets/qa-260908-dropdown-design-system/8-sheet-375.jpg) | 3 | không | khớp chuẩn — Hàng quyền đầu ("Tạo lớp học"); 3 mục inherit/grant/deny; sheet lấy tiêu đề = tên quyền. |
| 9 | Cài đặt lớp — Chọn thành viên | [đóng](assets/qa-260908-dropdown-design-system/9-closed-1024.jpg) | [popover](assets/qa-260908-dropdown-design-system/9-popover-1024.jpg) | [sheet](assets/qa-260908-dropdown-design-system/9-sheet-375.jpg) | 2 | không | khớp chuẩn — Lớp "Lý 7" (còn thành viên gán được). Trigger có viền mint ở ảnh đóng vì picker nhận focus ngay khi bấm "+ Thêm học vụ" — trạng thái focus, không phải lệch token. Placeholder "— Chọn thành viên —". |
| 10 | Cài đặt lớp — Bàn giao cho | [đóng](assets/qa-260908-dropdown-design-system/10-closed-1024.jpg) | [popover](assets/qa-260908-dropdown-design-system/10-popover-1024.jpg) | [sheet](assets/qa-260908-dropdown-design-system/10-sheet-375.jpg) | 2 | không | khớp chuẩn — Card Bàn giao; placeholder "— Chọn giáo viên —"; 2 mục (Cô Thu, Thầy Minh). |

Thêm: [`/records` trạng thái rỗng](assets/qa-260908-dropdown-design-system/1-empty-1024.jpg) (`?class_id=` không tồn tại) — trigger có `data-placeholder`, chữ "Chọn lớp" `font-weight 700`, màu `rgb(134,153,143)` (ink-400) thay cho `800`/ink-900 của bản cũ: **đúng thay đổi cố ý** theo `data-placeholder:*`.

## 2. Token đo được (giống nhau ở 10/10 vị trí)

| Phần | Thuộc tính | Giá trị |
|---|---|---|
| Trigger | min-height / viền / bo góc | `44px` / `2px` `rgb(226, 236, 226)` (line-200) / `14px` |
| Trigger | chữ | `14.5px` / `800` / `rgb(28, 58, 49)` (ink-900), nền trắng, padding-left `14px` |
| Trigger (placeholder) | chữ | `700` / `rgb(134, 153, 143)` (ink-400) — vị trí 6, 9, 10, records rỗng |
| Panel popover | nền / viền / bo góc / padding | trắng / `2px` line-200 / `16px` / `6px` |
| Panel popover | bóng | `rgba(28,58,49,0.28) 0 18px 40px -18px` |
| Panel popover | max-height | theo `--radix-popover-content-available-height` (đo được 355–646px tuỳ vị trí) — class `max-h-(--radix-popover-content-available-height)` **có tác dụng** |
| Option đang chọn | chữ / nền / bo góc / cao | `14px` `700` `rgb(31, 107, 83)` (mint-700) / `rgb(233, 247, 241)` (mint-50) / `11px` / `42px`; có icon ✓ |
| Option thường | chữ / nền | `rgb(39, 67, 59)` (ink-700) / trong suốt |
| ARIA | trigger | `button[role=combobox] aria-haspopup=listbox aria-controls=<id>-listbox`, `data-value`, `data-placeholder` |
| Sheet 375 | | `HvModal` với tiêu đề = `sheetTitle` (Chọn lớp / Giáo viên / Nhóm hành động / Hình thức / Vai trò / Tạo lớp học / Chọn học vụ / Bàn giao cho) |

`aria-invalid:*` và `aria-disabled:*`: không tái hiện được trên seed (không có form lỗi / option disabled ở dữ liệu thật); được phủ bởi unit test `hv-select.test.tsx` và test audit "Tùy chỉnh" (option `aria-disabled`).

## 3. Bàn phím (1024px)

| Vị trí | Tab tới trigger | Enter mở | ↑/↓ di chuyển focus trong listbox | Enter chọn + đóng (trigger) | Esc đóng | Focus về trigger | |
|---|---|---|---|---|---|---|---|
| `/records` — Lớp | ✅ | ✅ | ✅ | ✅ `Lý 7 - Chiều Thứ Năm · 2 HS` | ✅ | ✅ | PASS |
| `/audit` — Giáo viên | ✅ | ✅ | ✅ | ✅ `Cô Lan` | ✅ | ✅ | PASS |
| Phân quyền — override (trong dialog) | ✅ | ✅ | ✅ | ✅ `Cấp riêng` | ✅ | ✅ | PASS |

## 4. E2e (stack cách ly, seed mới)

```
npx playwright test e2e/records-search.spec.ts e2e/class-staff-read.spec.ts \
  e2e/class-staff-write.spec.ts e2e/secretary-send.spec.ts e2e/roster.spec.ts
  → 8 passed (2.0m)
npx playwright test e2e/class-staff-write.spec.ts e2e/secretary-send.spec.ts   (lần 2)
  → 4 passed (1.2m)
```

Không cần baseline master: 0 fail.

## 5. Ghi chú khi chạy script

- Ba vị trí cần cách mở khác lúc đầu: kỳ thu có hoá đơn thuộc Thầy Minh (đăng nhập `0901000002`), nút "Ghi danh vào lớp" chỉ ở tab `?tab=unenrolled`, "+ Thêm học vụ" bị disabled trên lớp Toán 8 (hết thành viên gán được) → dùng lớp Lý 7. Không phải lỗi component.
- Trường `inSheet` trong `results.json` luôn `true` vì Radix Popover content cũng có `role=dialog`; phân biệt popover/sheet dựa vào `panel` (popper wrapper, chỉ có ở 1024) và `sheetTitle` (chỉ có ở 375).
