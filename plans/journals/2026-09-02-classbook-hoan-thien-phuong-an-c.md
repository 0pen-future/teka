---
title: Classbook hoàn thiện Phương án C — nhãn lớp kèm khung giờ
date: 2026-09-02
summary: "Dropdown chọn lớp hiện 'Toán 8 · Tối Thứ Ba' in đậm kèm 'N HS · giáo viên', month stepper thành một card, thanh CÓ MẶT cùng dòng; review bắt aria-label che tên lớp và thanh bị ẩn trên điện thoại"
---

# Classbook hoàn thiện Phương án C — nhãn lớp kèm khung giờ

## What happened

Sau pass toolbar (plan `260902-1639`), `/classbook` còn bốn chỗ lệch mock
"Sổ lớp mở rộng tại chỗ" (plan
`plans/260902-1718-GH-260902-classbook-option-c-complete/plan.md`):

- Nhãn lớp trong dropdown phải là **`Toán 8 · Tối Thứ Ba`** in đậm (user yêu
  cầu). Thêm `formatScheduleLabel` vào `features/roster/lib/roster-format.ts`:
  buổi (Sáng < 12h ≤ Chiều < 18h ≤ Tối) + thứ; một ngày viết đầy đủ "Thứ Ba",
  nhiều ngày rút gọn "Tối T2-T4-CN" (nối bằng "-" để không đụng dấu " · " ngăn
  tên lớp), nhiều khung giờ nối ", " theo giờ bắt đầu tăng dần. Roster vẫn là
  chủ sở hữu định dạng lịch; teaching chỉ import từ `features/roster/index.ts`.
- Dòng phụ `14 HS · Cô Lan` trên trigger: headcount lấy từ enrollments đã có
  trên trang, giáo viên từ `useClassStaff` (endpoint staff sẵn có, lọc
  `giao_vien` còn hiệu lực). Không thêm API.
- Trigger bỏ viền input, thành card trắng `shadow-soft-sm` như `.sel` của mock;
  month stepper gom hai mũi tên và nhãn vào một card; ô CÓ MẶT đưa số và thanh
  tiến độ lên một dòng.

## Review findings và cách xử lý

- **H1 — mất ô tìm lớp.** Reviewer thấy `class-select.tsx` từ modal có search
  chuyển sang Radix Select không search, và hai trang Điểm danh, Hồ sơ vẫn
  có search. Đây là quyết định user đã chọn ở plan `260902-1639` ("dropdown
  thay modal, typeahead đủ"), không phải hồi quy của pass này. Giữ nguyên,
  ghi vào non-goals của plan kèm giới hạn: typeahead chỉ khớp tiền tố, gõ "9b"
  không ra "Anh 9B".
- **M1 — `aria-label="Chọn lớp"` che tên lớp đang chọn** với screen reader.
  Sửa thành `Chọn lớp — đang xem {nhãn}`, test đổi matcher sang `/^Chọn lớp/`.
- **M2 — thanh CÓ MẶT `hidden sm:block`** làm mất `aria-label="Có mặt N%"` và
  màu cảnh báo < 70% trên điện thoại, jsdom không bắt được vì không áp CSS.
  Đổi sang `w-12 sm:w-16`, thanh luôn hiện.
- Nhẹ: hoist `mondayFirst` dùng chung hai formatter; thêm test cho giờ bắt
  đầu không parse được (rớt phần buổi, giữ thứ); stepper dùng `--radius-sm`
  và `--ease-out` thay số cứng; sửa ví dụ thứ tự khung giờ trong plan.

## Verification

`make test-web` 78 file, 583 pass, 3 skip. `make lint-web` exit 0. Chưa xem
trên trình duyệt thật: popper với 10+ lớp, truncate trigger trên màn hẹp.

## Lessons

- Khi viết lại một component, đối chiếu với HEAD chứ không chỉ với bản đang
  sửa dở: reviewer đọc diff so với HEAD nên thấy cả thay đổi của pass trước.
- `hidden` trên phần tử mang `aria-label` là hồi quy a11y mà test jsdom không
  phát hiện; cần đọc class thay vì tin test.
