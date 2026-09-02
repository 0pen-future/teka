---
phase: 5
title: "Test, dọn dẹp, docs, verify"
status: pending
priority: P1
effort: "0.5d"
dependencies: [4]
---

# Phase 5: Test, dọn dẹp, docs, verify

## Overview
Cập nhật test classbook theo selector mới mà không giảm hành vi được phủ, thêm
test cho phần mới (stepper, chọn lớp, chip trạng thái, cột VIỆC, guard đổi
tháng), dọn code chết, cập nhật docs nếu có mô tả trang, chạy đủ gate.

## Requirements
- Functional (test `classbook-page.test.tsx`):
  - Stats: `statCard` → `kpi(label)` truy vấn dải KPI (SĨ SỐ / CHUYÊN CẦN /
    ĐIỂM TB / LÃI/LỖ T8) với value và sub mới ("tái tục 100%", "2/2 lượt",
    "0 buổi", "thu 300.000đ · chi 600.000đ", "-300.000đ" có class coral).
  - Bảng: hàng held "1/1", chip "Chưa có", chip "0/1"; hàng hủy "Nghỉ lễ" +
    "Buổi hủy"; hàng dự kiến "1 dự kiến"; 26/08 ngoài cửa sổ; ghi chú doanh thu.
  - Lưu nhận xét: sau khi mở hàng, textarea `getByLabelText("NHẬN XÉT CHUNG CỦA BUỔI")`,
    toast, "Đã lưu ✓", chip hàng đổi thành "Đã có" (thay cho text note trong hàng).
  - Lưu điểm chung: không còn click tab "Điểm buổi"; input "Điểm Nguyễn Văn An"
    hiện ngay; sau lưu, hàng có "7,5" (định dạng phẩy) và KPI ĐIỂM TB "7,5" / "1 buổi".
  - Guard đổi buổi / đổi lớp: chọn lớp qua nút chọn lớp → modal "Chọn lớp" →
    nút "Toán 6B"; guard "Còn 1 ô chưa lưu"; "Bỏ thay đổi" → nút chọn lớp hiện
    "Toán 6B", không PUT.
  - Mới: guard đổi tháng ("Tháng trước" khi dirty → guard); stepper ghi
    `?month=2026-07` và bảng trống hiện "Chưa có buổi học nào trong tháng 7.";
    `?month=2026-08` từ URL đọc đúng; footer "1 ô chưa lưu"; Escape đóng hàng;
    ↑↓ đổi focus giữa nút hàng; không có lớp → "Chưa có lớp đang hoạt động".
  - Xóa test "opens the detail panel as a sheet on narrow viewports"; thay bằng
    test cột VIỆC render chip "Nhận xét" cho hàng held (class `sm:hidden`, kiểm
    tra tồn tại trong DOM).
  - `classbook-course.test.tsx`: chuyển view qua `getByRole("tab", { name: "Chương trình & giáo án" })`.
- Non-functional: `npm run test`, `npm run typecheck`, `npm run lint`,
  `npm run build` (nếu có script) xanh; không `eslint-disable` mới ngoài cái đã có.

## Architecture
- Test utils giữ nguyên (`renderWithProviders`, fixtures roster/teaching).
- Docs: kiểm tra `docs/` và `apps/web/README.md` có mô tả trang Quản lý lớp
  học / panel chi tiết / `useMediaQuery` không (`grep -rn "classbook\|Quản lý lớp\|useMediaQuery\|SessionDetailPanel" docs apps/web/README.md`);
  cập nhật câu mô tả nếu có, không thêm mục mới.

## Related Code Files
- Modify: `apps/web/src/features/teaching/__tests__/classbook-page.test.tsx`
- Modify: `apps/web/src/features/teaching/__tests__/classbook-course.test.tsx`
- Modify (nếu tham chiếu): `docs/frontend-guidelines.md`, `apps/web/README.md`, `docs/*` liên quan.
- Verify deleted: `class-stat-cards.tsx`, `use-media-query.ts` + test, `session-detail-panel.tsx` (đã đổi tên).

## Implementation Steps
1. Sửa/bổ sung test theo danh sách trên; chạy `npm run test -- classbook`.
2. `grep` code chết: `ClassStatCards`, `SessionDetailPanel`, `useMediaQuery`,
   `variant="sheet"`, `isMobile`.
3. Docs sweep theo grep; cập nhật tối thiểu.
4. Chạy `npm run lint`, `npm run typecheck`, `npm run test` toàn bộ trong `apps/web`.
5. So kết quả với Success Criteria trong `plan.md`; ghi nhận sai lệch có chủ
   ý (không có tên giáo viên trong nút chọn lớp).

## Success Criteria
- [x] Toàn bộ gate xanh.
- [x] Không còn tham chiếu code chết.
- [x] Mọi mục Success Criteria ở `plan.md` được tick hoặc ghi rõ sai lệch.

## Risk Assessment
- Test roving tabindex với `userEvent.keyboard("{ArrowDown}")` cần focus ban
  đầu ở nút hàng → `heldRow.focus()` trước.
- MSW month-marks handler lọc theo tháng: kiểm tra `teaching-handlers.ts` hỗ
  trợ `month=2026-07` trả rỗng thay vì 404.
