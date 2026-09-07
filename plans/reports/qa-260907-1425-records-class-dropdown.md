# QA trực quan — Hồ sơ học sinh: dropdown lớp + tìm học sinh

- Ngày: 2026-09-07 14:25
- Nhánh: `feat/records-class-dropdown-student-search`
- Stack: `docker compose -p teka-e2e` cách ly (web 55173, api 58080, postgres 55432), seed dev, đăng nhập Cô Lan (chủ trung tâm).
- Ảnh: `qa-260907-1425-records-class-dropdown/` (chụp bằng Playwright chromium, DPR 1).

## Kết quả

| Trạng thái | 1280×800 | 375×812 | Nhận xét |
|---|---|---|---|
| Mặc định | `desktop-01-default.png` | `phone-01-default.png` | Toolbar trắng, trigger `Lý 7 - Chiều Thứ Năm · 2 HS`, ô tìm có `kbd /`, counter `2 học sinh` (desktop trong toolbar, phone dưới bảng cạnh nút CSV). Desktop có nút `Tải danh sách (CSV)` ở header; phone ẩn. |
| Mở dropdown | `desktop-02-dropdown-open.png` | `phone-02-dropdown-open.png` | Desktop: popover listbox rộng bằng trigger, nhãn nhóm `LỚP ĐANG DẠY`, dấu check ở lớp đang chọn, `N HS` bên phải. Phone: HvModal bottom sheet `Chọn lớp`. 3 lớp ⇒ không có ô lọc (đúng ngưỡng >5). |
| Gõ `b` (lớp Toán 8) | `desktop-08-highlight-b.png` | `phone-08-highlight-b.png` | `<mark>` nền sun-200 ở chữ `B` của `Bé Bình`, `Bé An`; counter `2 / 2 học sinh`; nút × thay `kbd`. Phone: hàng 2 dòng `TB — · → Chưa đủ dữ liệu · vắng 0` và nút `Xem`. |
| Gõ `ng` / `Trương` (không khớp) | `desktop-03-search-ng.png`, `desktop-04-search-truong.png` | `phone-03-search-ng.png`, `phone-04-search-truong.png` | Card `Không tìm thấy học sinh nào khớp “…”`, gợi ý, nút `Xoá tìm kiếm`; counter `0 / 2 học sinh`. |
| Đang tải tháng | `desktop-09-loading.png` | `phone-09-loading.png` | 5 hàng shimmer; desktop theo 6 cột, phone 2 thanh/hàng, counter trống. |
| Đổi lớp | `desktop-07-toan8-default.png` | `phone-07-toan8-default.png` | Trigger cập nhật `Toán 8 - Tối Thứ Ba · 2 HS`, bảng đổi học sinh, `q` bị xoá. |
| Trang không đụng | `desktop-06-sessions.png`, `desktop-06-students.png`, `desktop-06-classbook.png` | — | `/sessions` vẫn dùng pill `role=tab`; `/students`, `/classbook` không đổi (git diff không chạm các file này). |

`document.documentElement.scrollWidth`: desktop 1280, phone 375 ⇒ không tràn ngang.

## E2E (Playwright, stack cách ly)

- `class-staff-read.spec.ts` 2/2, `class-staff-write.spec.ts` 2/2, `records-search.spec.ts` 1/1 (375×812, Cô Thu hoc_vu).
- Assertion classbook trong `class-staff-read` chuyển từ `tab` sang combobox `Chọn lớp — đang xem …` vì classbook đã đổi selector ở commit `05d3f50` (spec vốn đỏ trên master, không liên quan phase này).

## Lưu ý

- Chọn lại đúng lớp đang chọn vẫn ghi `class_id` lên URL (giữ hành vi pill cũ mà e2e dựa vào) nhưng không xoá `q`.
- Không thấy lỗi console trong lúc chụp.
