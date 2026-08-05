# Báo cáo triển khai: Tab "Chưa ghi danh" + luồng Thêm học sinh 2 bước

Nguồn thiết kế: dự án claude.ai/design `So Lop - Prototype.dc.html` (design system
"Học Vui Mỗi Ngày"), đọc qua DesignSync MCP.

## Kết quả

Màn "Lớp & học sinh" đã có đủ hai tính năng theo prototype:

1. **Tab "Chưa ghi danh"** — pill tab cuối dãy (`class_id=none` trên URL), gọi
   `GET /students?unenrolled=true`. Hàng học sinh hiển thị badge sun "Chưa vào
   lớp nào" và nút mint "Ghi danh vào lớp" (cả bảng desktop lẫn card mobile).
2. **Thêm học sinh 2 bước** — nút "+ Thêm học sinh" trên màn roster mở wizard:
   - Bước 1/2 "Thêm học sinh": pill bước, mô tả "Tạo hồ sơ trước — ghi danh vào
     lớp ở bước sau.", nút "Tiếp tục: Ghi danh →".
   - Bước 2/2 "Ghi danh vào lớp": chip học sinh (avatar sky + tên + người liên
     hệ), Select lớp (`Tên — T2 · T5 — 18:00 · 150.000 ₫/buổi`), "Ngày bắt
     đầu", ô "Đơn giá / buổi" read-only viền dashed, ghi chú kế thừa đơn giá.
   - "Để sau" → toast 'Đã lưu hồ sơ — ghi danh sau ở tab "Chưa ghi danh"' và
     chuyển sang tab Chưa ghi danh.
   - Ghi danh thành công → toast "Đã ghi danh {tên} vào {lớp} — tính tiền từ
     buổi có mặt đầu tiên" và chuyển sang tab lớp đó.

## Thay đổi chính

- API Go: `ListFilter.Unenrolled` + mệnh đề `NOT EXISTS` trên open enrollments
  (`repository.go`), parse `unenrolled=true` + swagger (`handler.go`), test tích
  hợp `TestListUnenrolledExcludesOpenEnrollments`; regenerate `apps/api/docs`.
- Web: `students-api.ts` thêm `unenrolled`; `students-page.tsx` (tab + hàng +
  chuỗi wizard, restyle tab theo DS: active mint + shadow-press-mint);
  `student-dialog.tsx` (chế độ wizard, chỉ create-mode); `enroll-student-dialog.tsx`
  (mode "student" thiết kế lại theo Bước 2/2; mode "class" giữ nguyên hành vi
  và toast cũ).
- Test: MSW hỗ trợ `unenrolled`; test mới cho dialog Bước 2/2; e2e bổ sung đoạn
  wizard → "Để sau" → tab Chưa ghi danh → ghi danh từ tab.

## Xác minh

- `go test ./...` (unit) + test tích hợp students (testcontainers): PASS.
- Vitest toàn bộ web: 94/94 PASS; `tsc -b` sạch; eslint 0 error.
- E2E `roster.spec.ts` trên stack dev thật: PASS (bao gồm luồng wizard mới).
- Kiểm live API: `unenrolled=true` trả 12/22 học sinh — filter hoạt động.
- 7 e2e khác (attendance/billing/collections/statement) fail do dữ liệu seed đã
  bị các lần chạy trước tiêu thụ (seeder idempotent không khôi phục trạng thái
  "buổi chưa điểm danh") — không liên quan diff này; các spec đó không đi qua
  code roster/students thay đổi, và `unenrolled=false` giữ nguyên SQL cũ.

## Khác biệt có chủ đích so với prototype

- Bảng giữ cột Ghi chú / SĐT hiện có, không thêm cột NHẬP HỌC / BUỔI T7:
  `StudentResponse` chưa có dữ liệu enrollment/attendance — mở rộng contract API
  nằm ngoài phạm vi được duyệt.
- Tab không hiển thị số đếm (`Chưa ghi danh · N`): cần thêm N query đếm.
- Giữ tab "Tất cả" (prototype không có nhưng là superset hữu ích đang dùng).
- Toast ghi danh ở `StudentDetailPage` (cùng mode "student") đổi theo thông điệp
  design mới; mode "class" ở `ClassDetailPage` giữ thông điệp cũ.
