# Restyle "Lớp & học sinh" + "Điểm danh" theo prototype Sổ Lớp

Status: done · Branch: master · Source: claude.ai/design project `4a7e6c77` (`So Lop - Prototype.dc.html`)

## Contract

- **Outcome:** Hai màn hình khớp visual prototype 100% theo token design system đã có trong `src/styles/tokens/`. Bảng học sinh có header cố định (sticky) + thân bảng cuộn, vừa khít viewport (trang không cuộn ở `sm+`).
- **Constraints:** Không sửa backend; giữ nguyên chức năng hiện có (date-range, nhóm ngày, "Cần điểm danh", dialogs, dirty-guard, search học sinh); giữ text/role mà unit + e2e tests bám vào.
- **Non-goals:** Cột NHẬP HỌC / BUỔI T7 (API chưa có dữ liệu — cần aggregation backend, đã chốt bỏ); số lượng học sinh trên tab lớp (ClassResponse thiếu student_count); search lớp "Tìm lớp…" (chỉ hiện khi >5 lớp trong prototype, giữ search học sinh hiện có).
- **Acceptance:**
  1. Bảng học sinh: card bo 20px shadow-md; header `cream-200`, chữ 12px/800/ink-500 tracking .4px, sticky khi cuộn; thân cuộn trong viewport; cột Người liên hệ gộp tên + SĐT (SĐT 12.5px ink-400 dưới tên).
  2. H1 26px Baloo 800 + subtitle ink-500 13.5px ở cả 2 màn.
  3. Điểm danh: danh sách buổi trong card trắng bo 20px shadow-md, label section 12.5px/800/ink-400; hàng buổi radius 12, màu theo trạng thái, selected mint-50 + border mint-300; tab pill press-mint.
  4. Panel điểm danh: bo 28px shadow-lg; header mint-400 (title 19px Baloo, sub 13px); hàng đếm pill mint-50/coral-100 dưới header; hàng học sinh = chip bo 16px viền 2px, vòng ✓/✕ 34px bên trái, nhãn Có mặt/Vắng bên phải (aria-hidden — aria-pressed giữ ngữ nghĩa); danh sách cuộn max-h 430px; nút xác nhận trong footer border-t.
  5. `npm run test`, `lint`, `typecheck` xanh; không đổi public contract.

## Files

- `src/features/roster/pages/students-page.tsx` — restyle + sticky/scroll table.
- `src/features/attendance/pages/sessions-page.tsx` — h1/subtitle, card bọc danh sách, tab pill.
- `src/features/attendance/components/session-list-item.tsx` — style hàng buổi theo prototype.
- `src/features/attendance/pages/attendance-page.tsx` — panel bo 28, hàng đếm, list cuộn.
- `src/features/attendance/components/attendance-row.tsx` — chip layout, mark trái.
- `src/features/attendance/components/confirm-attendance-bar.tsx` — padding/border theo prototype.

## Verification

- `tsc -b --noEmit` sạch, eslint sạch, vitest 24 files / 104 tests pass (sau cả vòng fix hậu review).
- Visual check bằng Playwright (Chrome MCP không truy cập được localhost): sticky header giữ nguyên khi thân bảng cuộn, panel điểm danh render đúng prototype ở desktop 1440px và mobile 390px. Script + ảnh trong scratchpad session.
- Code review (`code-reviewer` subagent): DONE_WITH_CONCERNS → đã fix H1 (focus ring bị shadow-* đè trên tab pill 2 màn), M2 (min-h-[240px] chống card bảng bị bóp khi tab wrap nhiều dòng), M3 (`max-h-[430px]` chỉ áp ở `lg+`, mobile giữ 1 trục cuộn), M4 (gộp header panel trùng lặp thành `PanelHeader`), + aria-label cho ô search, đơn giản hoá điều kiện render card danh sách buổi. Chi tiết: `reports/code-review.md`.
- Giữ nguyên theo prototype (đã đối chiếu source prototype, chờ user quyết nếu muốn đổi): màu tint cả hàng buổi học (`color` trên row — "Sắp diễn ra"/huỷ ink-400 ~3.0:1, đã điểm danh mint-600 ~4.1:1, dưới WCAG AA 4.5:1), SĐT 12.5px ink-400, header bảng ink-500 trên cream-200.

## E2E hiện trạng (không do thay đổi này)

- `roster.spec.ts` fail vì chờ nút "+ Tạo lớp mới" ở route `/classes` đã bị xoá bởi commit `1161903` (remove legacy class management flow) — spec chưa được cập nhật theo refactor đó.
- `attendance.spec.ts` fail vì trạng thái DB dev dùng chung: buổi pending thuộc lớp "Toán Thầy Thược" không có học sinh (roster rỗng → không có `button[aria-pressed]`), và mọi buổi "Sắp diễn ra" trong cửa sổ 14 ngày đã bị các lần chạy trước huỷ hết. Snapshot lỗi xác nhận markup mới render đúng.
