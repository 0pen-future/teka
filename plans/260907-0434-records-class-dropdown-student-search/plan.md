---
title: "Hồ sơ học sinh: dropdown chọn lớp + tìm học sinh (Phương án A)"
description: "Thay dãy pill lớp trên /records bằng thanh bộ lọc: dropdown lớp kèm sĩ số và ô tìm học sinh bỏ dấu, bám 100% mockup Phương án A."
status: completed
priority: P1
effort: "3d"
tags: [web, teaching, records, ui]
created: 2026-09-07
blockedBy: []
blocks: []
brainstorm: ../reports/brainstorm-260907-1100-records-class-dropdown-student-search.html
---

# Hồ sơ học sinh: dropdown chọn lớp + tìm học sinh (Phương án A)

Status: completed (2026-09-07, cook --auto) · Branch: `feat/records-class-dropdown-student-search` (8 commit `2e39587`…`f1d3515`, gồm 2 commit sửa theo review; chưa push/PR) · Nguồn thiết kế: `plans/reports/brainstorm-260907-1100-records-class-dropdown-student-search.html` (Phương án A, artifact <https://claude.ai/code/artifact/acebf215-2b90-4ad0-8cd0-b045d9285821>).

## Overview

Trang `/records` (`apps/web/src/features/teaching/pages/records-page.tsx`) hiện chọn lớp bằng dãy pill `role="tablist"` và không có cách tìm học sinh. Plan này thay khối pill bằng **thanh bộ lọc** (toolbar) gồm dropdown lớp (tên + sĩ số, ô lọc lớp khi >5 lớp, bottom sheet dưới `sm`) và ô tìm học sinh (lọc live, bỏ dấu tiếng Việt, tô đậm phần khớp, bộ đếm `n / tổng`, trạng thái rỗng có lối thoát). Mọi phần khác của trang — tiêu đề, nút CSV, cột bảng, trang chi tiết `/records/:studentId`, pill lớp ở Điểm danh và Lớp & học sinh — giữ nguyên.

## Contract

- **Outcome:** Người dùng vào Hồ sơ học sinh chọn lớp từ dropdown (thấy sĩ số từng lớp, lọc được khi nhiều lớp, cuộn thoải mái trên điện thoại) và gõ tên để thu hẹp bảng ngay lập tức, kể cả gõ không dấu. URL phản ánh cả `class_id` lẫn `q` để chia sẻ/refresh giữ nguyên trạng thái; đổi lớp bắt đầu lại (xoá `q`).
- **Constraints:** (1) Bám 100% mockup Phương án A: kích thước, token màu, bo góc, chữ, trạng thái mở/gõ/rỗng/tải, mobile <768 xếp dọc + hàng 2 dòng, bottom sheet dưới `sm`. (2) Không phá UI hiện có: tiêu đề + phụ đề, nút "Tải danh sách (CSV)", 6 cột bảng desktop, màu điểm/xu hướng, `/records/:studentId`, pill lớp ở `sessions-page` và `students-page`, `ClassSelect` của classbook. (3) Chỉ dùng token DS (`apps/web/src/styles/tokens/*.css`) và primitive sẵn có (`components/ui/select.tsx` pattern, `HvModal`, `radix-ui`, `lucide-react`). (4) Test hiện có trong `records-pages.test.tsx` phải xanh không sửa assertion (chỉ bổ sung). (5) Không thêm dependency mới.
- **Non-goals:** Đổi pill sang dropdown ở Điểm danh / Lớp & học sinh; chip lọc nhanh (Cần lưu ý / Vắng nhiều) của Phương án B; tìm học sinh xuyên lớp; lưu query vào localStorage; ngày sinh.
- **Acceptance criteria:** xem mục *Success Criteria* dưới; từng phase có tiêu chí riêng.

## Quyết định thiết kế đã chốt (đã xác nhận với người dùng 2026-09-07)

| # | Quyết định | Mặc định trong plan | Phương án thay thế |
|---|-----------|---------------------|--------------------|
| D1 | Nguồn sĩ số `28 HS` trên trigger và từng mục | ✅ **Đã chốt.** Backend thêm trường **additive** `student_count` (số ghi danh đang học) vào `ClassResponse` (Phase 2). Sĩ số trigger = tổng dòng bảng. | Chỉ frontend: trigger lấy `rows.length` của lớp đang chọn, các mục **không** có sĩ số (lệch mockup). Nếu chọn phương án này, bỏ Phase 2 và bỏ `{n} HS` trong mục. |
| D2 | Đổi lớp có giữ `?q=` không | ✅ **Đã chốt: XOÁ `q` khi đổi lớp.** `selectClass` set `class_id` và delete `q` trong cùng một `setSearchParams` (replace) → ô tìm trống, bảng đầy đủ, bộ đếm `N / N`. | (Bị loại) Giữ `q` như mockup gốc. |
| D3 | Popover dưới `sm` | ✅ **Đã chốt.** **Bottom sheet** bằng `HvModal` (size md, tiêu đề "Chọn lớp"), cùng list body với popover. Cần hook `useMediaQuery`. | Dùng popover ở mọi breakpoint (lệch ghi chú 8 của mockup). |
| D4 | Component dropdown | **Component mới** `RecordsClassSelect` dựng trên `Popover` primitive của `radix-ui` + `role="listbox"` tự quản phím. Lý do: Radix `Select` chiếm typeahead và focus của item nên **không đặt được ô lọc bên trong** `SelectContent` một cách ổn định; popover + listbox tái tạo đúng markup mockup (`button[aria-haspopup=listbox]` → `div[role=listbox]` → `button[role=option]`). `ClassSelect` của classbook không đụng. | Mở rộng `ClassSelect` dùng chung: rủi ro đổi UI classbook, bị D4 loại. |
| D5 | Ô lọc lớp trong popover | Tái dùng **`useClassSearch`** nguyên trạng (ngưỡng >5, substring không phân biệt hoa thường) đúng ghi chú 3 của mockup; chỉ đổi kiểu dáng ô nhập theo `.pf`. Bỏ dấu tiếng Việt chỉ áp dụng cho **tìm học sinh**. | Bỏ dấu cả ô lọc lớp: đổi `useClassSearch` → ảnh hưởng 2 trang khác, bị ràng buộc (2) loại. |

## Phases

| # | Phase | Status | Phụ thuộc | Ước lượng |
|---|-------|--------|-----------|-----------|
| 1 | [Fold tiếng Việt, lọc học sinh, `?q=` trên URL](./phase-01-search-helpers-url-state.md) | Done | — | 3h |
| 2 | [API: `student_count` trên danh sách lớp](./phase-02-class-student-count-api.md) | Done | — (D1) | 4h |
| 3 | [`RecordsClassSelect`: popover listbox + bottom sheet](./phase-03-records-class-select.md) | Done | 2 (kiểu `Class.student_count`), 1 (không bắt buộc) | 6h |
| 4 | [`RecordsToolbar` + nối vào trang](./phase-04-records-toolbar-page-wiring.md) | Done | 1, 3 | 4h |
| 5 | [Bảng: tô đậm, trạng thái rỗng/tải, hàng mobile](./phase-05-table-highlight-states-mobile.md) | Done | 1, 4 | 4h |
| 6 | [Test tích hợp, e2e, docs, QA thị giác](./phase-06-tests-e2e-docs-qa.md) | Done | 1–5 | 4h |

Phase 1 và 2 độc lập (có thể chạy song song: khác thư mục `apps/web/src/lib` + `teaching/lib` vs `apps/api` + `roster/schemas`). Phase 3–5 tuần tự vì cùng chạm `records-page.tsx` và `student-records-table.tsx`.

## Files (tóm tắt — chi tiết trong từng phase)

- Mới: `apps/web/src/lib/utils/vietnamese.ts`, `apps/web/src/lib/hooks/use-media-query.ts`, `apps/web/src/features/teaching/lib/student-search.ts`, `apps/web/src/features/teaching/components/records-class-select.tsx`, `apps/web/src/features/teaching/components/records-toolbar.tsx`, `apps/web/e2e/records-search.spec.ts`, test đi kèm.
- Sửa: `records-page.tsx`, `student-records-table.tsx`, `apps/web/src/lib/utils/index.ts`, `zalo-friend-picker.tsx` (chỉ import helper chung), `components/hv/hv-animations.css` (keyframe shimmer), `roster-schemas.ts` + `roster-handlers.ts` (`student_count`), `records-pages.test.tsx`, `e2e/class-staff-read.spec.ts`, `e2e/class-staff-write.spec.ts`, `apps/api/internal/features/classes/{dto,repository,service,handler}.go` + test.
- Không đụng: `features/roster/pages/*`, `features/attendance/pages/sessions-page.tsx`, `teaching/components/class-select.tsx`, `teaching/pages/classbook-page.tsx`, `student-record-page.tsx`, `components/ui/select.tsx`.

## Success Criteria

- [x] `/records` không còn `role="tablist"`; thanh bộ lọc trắng bo 20px hiện giữa tiêu đề và bảng với hai nhãn LỚP / TÌM HỌC SINH (và bộ đếm ở desktop) đúng kích thước/token trong mockup.
- [x] Trigger lớp hiện `Tên lớp · N HS`, mở popover (≥`sm`) hoặc bottom sheet (<`sm`) với nhóm "LỚP ĐANG DẠY", dấu ✓ mint ở lớp đang chọn, ô "Tìm lớp…" chỉ khi >5 lớp, note `Không có lớp nào khớp "q"` khi không khớp; ↑↓ Enter Esc hoạt động; chọn lớp cập nhật `?class_id=` (replace).
- [x] Gõ "nguyen" hiện "Nguyễn Văn An" với phần khớp bọc `<mark>` nền sun-200; bộ đếm `1 / N học sinh` cập nhật `aria-live=polite`; `?q=nguyen` trên URL; `/` focus ô tìm; × xoá query.
- [x] Không khớp → trong thẻ bảng hiện tiêu đề Baloo `Không tìm thấy học sinh nào khớp “q”`, dòng gợi ý và nút "Xoá tìm kiếm"; lớp không có học sinh vẫn hiện `Lớp chưa có học sinh đang học.` như cũ.
- [x] Đang tải → khung bảng với 5 hàng skeleton shimmer (tôn trọng reduced motion) thay cho dòng chữ; vẫn có status sr-only.
- [x] <768px: toolbar xếp dọc (select full-width, ô tìm dưới), bảng ẩn header, mỗi hàng = tên + `TB 8.6 · ↗ Tăng · vắng 0` + nút "Xem" bên phải, không cuộn ngang; bộ đếm + nút "CSV" dưới bảng; nút CSV trên header ẩn.
- [x] Tiêu đề, phụ đề, nội dung CSV, cột/màu bảng desktop, trang chi tiết, `sessions-page`, `students-page`, classbook không đổi (test hiện có xanh, chụp màn hình so sánh).
- [x] `npm run lint`, `typecheck`, `test`, `build` (apps/web) và `go test ./...` (apps/api, nếu Phase 2) xanh; e2e `class-staff-*.spec.ts` + spec mobile mới xanh trên stack e2e cách ly.

## Risks

- **Radix Select không chứa được ô lọc** → đã chọn Popover + listbox (D4). Tín hiệu vỡ: phím ↑↓ không di chuyển được giữa option sau khi gõ ô lọc → xử lý trong `onKeyDown` của list body, không quay lại `Select`.
- **Test jsdom không áp CSS**: nếu render cả biến thể desktop lẫn mobile bằng class ẩn/hiện, `getByText("8.8")` trong test hiện có sẽ trùng 2 phần tử. Giải pháp: nhánh **JS** theo `useMediaQuery` (mock `matchMedia` trong `test/setup.ts` trả `matches:false` → test cũ luôn ở nhánh desktop).
- **`student_count` lệch tổng dòng bảng** nếu predicate "đang học" khác `useEnrollmentsList({active:true})` → Phase 2 phải sao chép đúng predicate của repository enrollments và có test so khớp.
- **Kiểu `Class` thêm trường bắt buộc** làm fixture TS ở test khác không compile → dùng `.default(0)` trong zod và cập nhật mọi fixture typed `Class` (grep trong Phase 2).
- **e2e hiện có click `role="tab"`** trên `/records` → cập nhật 2 spec trong Phase 6 cùng lúc với code, không để CI đỏ giữa chừng.

## Rollback

Mỗi phase là commit riêng, không migration DB (Phase 2 chỉ thêm truy vấn COUNT và trường JSON additive). Revert commit của Phase 4 là trang quay về dãy pill cũ; Phase 2 có thể giữ lại độc lập vì additive.

<!-- slug: records-class-dropdown-student-search -->

## Kết quả thực thi (2026-09-07)

| Gate | Kết quả |
|---|---|
| apps/web lint / typecheck / vitest / build | 0 lỗi (5 warning có sẵn) / OK / 84 file, 628 pass, 3 skip / OK |
| apps/api build / test / scopelint / integration `TestStudentCounts*` | OK / OK / OK / 2 pass (Docker) |
| e2e (stack cách ly) | class-staff-read 2/2, class-staff-write 2/2, records-search 1/1 |
| QA thị giác | [qa-260907-1425](../reports/qa-260907-1425-records-class-dropdown.md) |

Lệch so với plan (chi tiết ở mục *Kết quả* từng phase): `HvModal.onCloseAutoFocus` (additive); skeleton compact 2 thanh; nhãn xu hướng thật; chọn lại cùng lớp vẫn ghi `class_id`, chỉ xoá `q` khi đổi lớp; assertion classbook e2e đổi sang combobox (đỏ sẵn trên master).

### Review và sửa sau review

[code-review-260907](../reports/code-review-260907-records-class-dropdown.md): DONE_WITH_CONCERNS, 8 finding. Đã sửa trong 2 commit:

| Finding | Sửa | Commit |
|---|---|---|
| F1 `student_count` đếm center-wide, vượt phạm vi đọc enrollments | `readScopedEnrollments` trong repo classes (own + stint + `enrollments.view_all`), test tích hợp member chỉ đếm lớp có stint | `a13f07d` |
| F2 Space trên option bị nuốt khi >5 lớp | `isPrintableKey` bỏ `" "`, test Space chọn option | `f1d3515` |
| F3 phím `/` của trang tranh chấp với picker đang mở | `isTypingTarget` bail khi focus trong `[role=listbox]`/`[role=dialog]`, test | `f1d3515` |
| F4 bộ đếm chỉ chờ sessions | bộ đếm và skeleton chờ cả query enrollments | `f1d3515` |

Chưa sửa (Low/Info, để đợt sau): F5 wording "Lớp chưa có học sinh" khi chưa có lớp nào; F6 `mockViewport` không có reset; F7 hai `role=status` lúc đang tải; F8 chưa thử IME telex trên Android thật.
Tester: [tester-260907-1428](../reports/tester-260907-1428-records-class-dropdown.md) (trước sửa) và [tester-260907-rerun](../reports/tester-260907-rerun-records-class-dropdown.md) (sau sửa).
