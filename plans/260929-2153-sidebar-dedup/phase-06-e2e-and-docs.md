---
phase: 6
title: "E2E và docs"
status: pending
priority: P2
effort: "3h"
dependencies: [1, 2, 3, 4, 5]
---

# Phase 6: E2E và docs

## Goal

Phase này đưa các spec Playwright và tài liệu về khớp với sidebar mới và với mô hình bộ điểm mới, rồi chạy toàn bộ e2e trên stack cô lập `teka-e2e`. Đây là cổng kiểm cuối cùng trước khi xin duyệt merge.

## Context

**E2E**

- `apps/web/e2e/secretary-send.spec.ts`:
  - :101, :106 và :174 tìm link hoặc heading "Gửi báo cáo";
  - :119 khớp regex `/^Gửi báo cáo tháng \d+\/\d+ của Thầy Minh$/`, là tên truy cập của link từng kỳ trên `/reports`;
  - :180 đã kiểm "Gửi thông báo →" có count 0, nên khi đổi nhãn nav phải dùng `exact: true` để không va với link "Gửi thông báo →" ở trang Học phí;
  - :41 là nhãn quyền "Quyền Gửi báo cáo học phí", được giữ nguyên.
- `apps/web/e2e/helpers/ux-routes.ts` có ba route đã gỡ: :43 `records`, :46 `classes-recruiting` và :58 `center-class-config`.
- `apps/web/e2e/records-search.spec.ts` chỉ kiểm trang `/records`, trang này bị gỡ ở phase 2.
- `apps/web/e2e/class-staff-read.spec.ts:36-47` dùng bộ chọn lớp trên `/records` để lấy `class_id` và kiểm học sinh của lớp.
- `apps/web/e2e/class-staff-write.spec.ts`:
  - :27-40 có helper `classIdFromRecordsPicker`;
  - :198-201 kiểm Thầy Minh vẫn đọc được học sinh cũ trên `/records?class_id=` sau khi bàn giao lớp.
- Trang thay thế là `apps/web/src/features/roster/pages/students-page.tsx`:
  - tab "Theo lớp" có `?tab=by-class&class_id=`;
  - các lớp hiện thành `role="tab"` trong `role="tablist"` có `aria-label="Lớp"` (:355-362);
  - danh sách học sinh đọc qua `useStudentsInfinite({ class_id })` (:157-167).
- `apps/web/e2e/courses.spec.ts:51` có comment nhắc nhóm "Kho học liệu". Heading ở `library.spec.ts:100` là của trang, không đổi.

**Docs**

- `docs/frontend-guidelines.md`:
  - :37 gọi menu là "Kho học liệu";
  - :152-173 (Navigation) liệt kê các nhóm và nói "Kho học liệu" có ba mục, điều này đã sai từ trước.
- `docs/api-guidelines.md`:
  - :446-447 gọi nhóm là "Giảng dạy" và "Kho học liệu";
  - :477-478 mô tả `score_set` nhưng chưa nói nó được chép vào lớp.
- `docs/architecture.md:62-74` mô tả `classprogram` và thứ tự đăng ký trong router, nhưng chưa có `grading`.

## Key insights

- Các spec đã có dùng redirect nên sẽ không crash, nhưng assert sẽ sai. Vì vậy cần viết lại, không chỉ đổi URL.
- [UNVERIFIED] Chưa rõ `/students?tab=by-class&class_id=` có trả học sinh cho giáo viên đã hết stint sau bàn giao hay không. Trang dùng API `/students` với `class_id`, còn `/records` trước đây đọc lịch sử ghi danh của lớp.
  - Bước 4 phải chạy spec để xác nhận trước khi viết assert cuối.
  - Nếu API không trả, dùng tab "Học viên" của trang chi tiết lớp (`/classes/:id?tab=students`) nếu tab đó đọc được với stint đã kết thúc.
  - Nếu cả hai đều không được thì bỏ assert đọc lịch sử và ghi chú lý do trong spec. Ghi rõ đây là mất khả năng đọc lịch sử, rồi báo lại cho người dùng trước khi merge.

## Requirements

- Mọi spec xanh trên `make e2e-isolated`.
- Không còn spec nào mở `/records`, `/classes/recruiting` hay `/center/class-config` như trang thật.
- Tài liệu Navigation mô tả đúng các nhóm và mục sau thay đổi:
  - nhóm "Học liệu";
  - mục "Gửi thông báo" trong Học phí, sáng cả ở `/notifications/:periodId` nhờ `activePrefixes`;
  - không còn các mục đã gỡ.
- Tài liệu API và kiến trúc nói rõ `score_set` của mẫu được chép vào lớp khi áp dụng và không còn CRUD bộ điểm riêng.

## Related files

**Modify**

- `apps/web/e2e/secretary-send.spec.ts`
- `apps/web/e2e/helpers/ux-routes.ts`
- `apps/web/e2e/class-staff-read.spec.ts`
- `apps/web/e2e/class-staff-write.spec.ts`
- `apps/web/e2e/courses.spec.ts`
- `docs/frontend-guidelines.md`
- `docs/api-guidelines.md`
- `docs/architecture.md`

**Delete**

- `apps/web/e2e/records-search.spec.ts`

**Create**: không có.

## Implementation steps

1. **`secretary-send.spec.ts`.**
   - :101: `getByRole("link", { name: "Gửi thông báo", exact: true }).first()`.
   - :106: heading `"Gửi thông báo"`, lấy từ h1 mới của phase 1.
   - :119: regex `/^Gửi thông báo tháng \d+\/\d+ của Thầy Minh$/`. Trước đó phải đối chiếu với aria-label mà phase 1 đặt trong `reports-page.tsx`.
   - :174: `getByRole("link", { name: "Gửi thông báo", exact: true })` có count 0.
   - Giữ nguyên :41 và :180.
2. **`ux-routes.ts`.** Gỡ ba entry ở :43, :46 và :58. Nếu có spec nào lặp qua danh sách này để kiểm redirect thì không cần thêm, vì redirect đã có unit test ở phase 1, 2 và 4.
3. **`class-staff-read.spec.ts:36-47`.**
   - Thay khối `/records` bằng: `page.goto("/students?tab=by-class")`, rồi `getByRole("tablist", { name: "Lớp" }).getByRole("tab", { name: new RegExp(STAFF_CLASS) }).click()`, rồi `expect(page).toHaveURL(/class_id=/)`, rồi kiểm "Bé An" và "Bé Bình", và cuối cùng đọc `class_id` từ URL.
   - Sửa comment :36-38 thành "The member reads the class's students on the students page's by-class tab".
4. **`class-staff-write.spec.ts`.**
   - Đổi tên helper thành `classIdFromStudentsPage` và dùng cùng cách chọn tab lớp như bước 3. Sửa doc comment :27-31.
   - Ở :198-201: thử `minh.goto(`/students?tab=by-class&class_id=${classId}`)` và kiểm "Bé Phúc".
   - Chạy riêng spec này: `make e2e-isolated E2E_ARGS="class-staff-write.spec.ts"`. Nếu fail vì Minh không còn thấy lớp hoặc học sinh, xử lý theo Key insights và ghi kết quả vào mục Open decisions của phase này.
5. Xoá `records-search.spec.ts`.
6. **`courses.spec.ts:51`.** Sửa comment thành "The nav entry lives in the Học liệu group, next to Lộ trình học." Chỉ sửa comment, không đổi selector nào.
7. **`docs/frontend-guidelines.md`.**
   - :37 đổi menu thành "Học liệu".
   - Viết lại đoạn Navigation (:154-173):
     - danh sách nhóm: Dạy học, Lớp học, Học liệu, Học phí, Trung tâm;
     - bỏ câu về ba mục của Kho học liệu, thay bằng một câu nói `/library` là một trang có tab con và `?tab=` cũ được redirect;
     - thêm một câu về `activePrefixes` của `NavEntry`: một mục có thể sáng trên các route con không nằm dưới `to` của nó, ví dụ "Gửi thông báo" trên `/notifications/:periodId`;
     - thêm một câu nói các đường dẫn cũ `/classes/recruiting`, `/records` và `/center/class-config` được redirect.
   - Giữ nguyên phần cảnh báo về `OVERFLOW_LABELS`.
8. **`docs/api-guidelines.md`.**
   - :446-447: "Six feature modules back the "Lớp học", "Học liệu" and "Dạy học" sidebar groups (`library` serves "Học liệu")". Trước khi viết, đối chiếu danh sách sáu module bên dưới với nhóm sidebar thật.
   - :477-478: thêm một câu: applying a published version copies its `score_set` into the class's score components (group title prefixes the label when there is more than one group); a class that already holds scores keeps its components; there is no separate score-set CRUD.
9. **`docs/architecture.md:64-74`.**
   - Bổ sung rằng `classprogram` còn đặt đầu điểm của lớp qua `grading` khi áp dụng.
   - Thứ tự đăng ký trong router thêm "`grading` before `classprogram`".
10. **Quét cuối.** Chạy các lệnh `rg` ở Verification, sửa mọi chỗ còn sót, rồi chạy toàn bộ e2e.

## Todo

- [ ] `secretary-send.spec.ts`
- [ ] `ux-routes.ts` và xoá `records-search.spec.ts`
- [ ] `class-staff-read.spec.ts` và `class-staff-write.spec.ts`, xác nhận đọc lịch sử sau bàn giao
- [ ] Comment ở `courses.spec.ts`
- [ ] Ba file docs
- [ ] Quét `rg` và chạy e2e toàn bộ

## Verification

- `rg -n "Gửi báo cáo" apps/web/src apps/web/e2e` chỉ còn nhãn quyền "Quyền Gửi báo cáo học phí", ở màn phân quyền và ở `secretary-send.spec.ts:41`.
- `rg -n "Lớp cần tuyển sinh|Hồ sơ học sinh|Cấu hình lớp học|/center/classes\b" apps/web/src apps/web/e2e docs` không trả kết quả nào.
- `rg -n '"/records|/classes/recruiting|/center/class-config' apps/web/src apps/web/e2e` chỉ còn khai báo route redirect và test redirect.
- `rg -n "score-sets|/score-set\b" docs apps/api/internal apps/web/src` chỉ còn khớp ở library.
- `make lint-web && make test-web`
- `make e2e-isolated`. Stack dùng compose `-p teka-e2e`, không đụng container `teka-*` của prod. Các spec statement cần seed mới, và target này đã tự seed.
- Đọc lại ba file docs đã sửa và kiểm mỗi link và mỗi đường dẫn file được nhắc tới đều còn tồn tại.

## Risk assessment

| Rủi ro | Khả năng × Tác động | Giảm thiểu |
|---|---|---|
| Giáo viên hết stint không còn đọc được học sinh cũ trên `/students` | Trung bình × Trung bình | Chạy spec để xác nhận trước. Có phương án thay thế và phải báo người dùng nếu mất tính năng |
| `exact: true` vẫn khớp nhiều link, ví dụ nav desktop và sheet mobile | Trung bình × Thấp | Giữ `.first()` như spec hiện tại. Assert count 0 không bị ảnh hưởng |
| e2e chậm hoặc flaky do tranh tài nguyên | Thấp × Thấp | Chạy riêng, không song song với `make test-api` |
| Docs lệch với code | Thấp × Thấp | Viết docs sau khi phase 1–5 đã xong, rồi đối chiếu với `dashboard-layout.tsx` |

## Security

Phase này không đổi code chạy thật. Các spec phân quyền (`class-staff-*` và `secretary-send`) vẫn kiểm cùng các ranh giới: thành viên không thấy nút ghi, và mục gửi ẩn khi không có `reports.send`.

## Rollback

`git revert` commit của phase. Nếu phase 1–5 bị revert thì phải revert cả phase này, vì spec mới giả định nhãn và route mới.

## Open decisions

Kết quả xác nhận đọc lịch sử học sinh sau bàn giao trên `/students` sẽ được điền vào đây sau bước 4.
