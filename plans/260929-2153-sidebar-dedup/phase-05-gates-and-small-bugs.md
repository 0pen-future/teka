---
phase: 5
title: "Gate và lỗi nhỏ"
status: completed
priority: P2
effort: "3h"
dependencies: [1]
---

# Phase 5: Gate và lỗi nhỏ

## Goal

Phase này sửa các lỗi nhỏ mà bản audit tìm thấy ngoài sidebar:

- "Thêm học viên" chỉ hiện cho chủ trung tâm.
- Nút "Tạo lớp" của Sổ lớp không còn dẫn tới 404.
- Tab Buổi học không còn hai link cùng href.
- Tab thông tin lớp không còn lối tắt tới hai trang đã gỡ.
- Comment và gợi ý ở thẻ lớp của trang Tổng quan được sửa cho đúng.

## Context

- `apps/web/src/features/roster/components/class-students-tab.tsx:40-45`: link "Thêm học viên" tới `/students?class_id=` hiện với mọi người xem tab. Theo quyết định ngày 2026-09-29, giáo viên không được ghi danh. Trang Học sinh cũng gate thao tác bằng `canManageStudents = isOwner` (`students-page.tsx:92`). `enrollments.create` là quyền mặc định của giáo viên, nên không dùng được để gate.
- `apps/web/src/features/roster/components/class-info-tab.tsx`:
  - :106 là lối tắt "Hồ sơ học sinh" tới `/records?class_id=`, trang này bị gỡ ở phase 2;
  - :122-126 là lối tắt "Cấu hình lớp học" tới `/center/class-config`, trang này bị gỡ ở phase 4.
- `apps/web/src/features/roster/components/class-sessions-tab.tsx:99-110`: hai link "Sổ đầu bài" và "Điểm danh & nhận xét →" cùng trỏ `/classbook?class_id=`. Trang `/sessions` hiểu `?class_id=` (`attendance/pages/sessions-page.tsx:66-70`).
- `apps/web/src/features/teaching/pages/classbook-page.tsx:370-374`: nút "Tạo lớp" gọi `navigate("/center/classes")`, nhưng route này không tồn tại.
  - Trang `/classes` gate nút tạo bằng `canCreate = has("classes.create")` (`roster/pages/class-list-page.tsx:57`).
  - Hộp tạo lớp mở bằng state cục bộ `creating` (:58, :126, :180).
  - `use-class-list-url-state.ts:51-60` chép toàn bộ query hiện có khi ghi, nên param `create` không bị mất.
- `apps/web/src/features/dashboard/components/class-overview-cards.tsx`:
  - comment ở :21 và :36-37 nói trang roster chỉ dành cho chủ trung tâm và sẽ đẩy người khác về Tổng quan, điều này sai từ khi `/students` gate bằng `students.list`;
  - comment ở :39-40 còn nhắc "roster";
  - gợi ý ở :119-121 nhắc mục "Quản trị học sinh", mục này không còn tồn tại.
- Test hiện có:
  - `roster/__tests__/class-detail-page.test.tsx`: :172-181 kiểm hai lối tắt sắp gỡ, :270 kiểm "Cấu hình lớp học" ẩn với người không phải chủ, :285-288 kiểm "Thêm học viên", :349-352 kiểm "Điểm danh & nhận xét →";
  - `teaching/__tests__/classbook-page.test.tsx` chưa có test cho nút "Tạo lớp".

## Key insights

- Gate "Thêm học viên" bằng `isOwner` khớp với trang đích. Nếu giáo viên vẫn thấy link, họ tới trang Học sinh mà không có nút thao tác nào.
- Ẩn thẻ "Lớp mới" với người không phải chủ trung tâm vẫn đúng. Lý do thật là thẻ đó dẫn tới việc thêm học sinh, việc chỉ chủ trung tâm làm được, chứ không phải chuyện redirect. Phase này chỉ sửa comment và không đổi hành vi.
- `?create=1` là cách nhỏ nhất để Sổ lớp mở hộp tạo lớp mà không phải nhân bản `ClassDialog`. Cách này theo đúng mẫu `?edit=1` của `class-detail-page.tsx:44-48` và :135-136.

## Requirements

- "Thêm học viên" chỉ render khi `isOwner`.
- Tab thông tin lớp:
  - lối tắt học sinh đổi sang `/students?class_id=${klass.id}` với nhãn "Học sinh của lớp";
  - bỏ lối tắt "Cấu hình lớp học".
- Tab Buổi học: link thứ hai đổi thành "Điểm danh" và trỏ `/sessions?class_id=${klass.id}`.
- Nút "Tạo lớp" ở Sổ lớp:
  - hiện khi `has("classes.create")`;
  - dẫn tới `/classes?create=1`.
- `/classes?create=1` mở `ClassDialog` ở chế độ tạo khi người dùng có `classes.create`. Đóng hộp thì xoá param bằng `replace`. Không có quyền thì bỏ qua param.
- Comment và gợi ý ở `class-overview-cards.tsx` phản ánh đúng hành vi. Gợi ý đổi thành 'tạo lớp đầu tiên ở mục "Danh mục lớp".', khớp nhãn sidebar ở `dashboard-layout.tsx:98`.

## Related files

**Modify**

- `apps/web/src/features/roster/components/class-students-tab.tsx`
- `apps/web/src/features/roster/components/class-info-tab.tsx`
- `apps/web/src/features/roster/components/class-sessions-tab.tsx`
- `apps/web/src/features/roster/pages/class-list-page.tsx`: phase 1 đã sửa file này, nên phase 5 phải chạy sau
- `apps/web/src/features/roster/__tests__/class-detail-page.test.tsx`
- `apps/web/src/features/roster/__tests__/class-list-page.test.tsx`: phase 1 cũng đã sửa file này
- `apps/web/src/features/teaching/pages/classbook-page.tsx`
- `apps/web/src/features/teaching/__tests__/classbook-page.test.tsx`
- `apps/web/src/features/dashboard/components/class-overview-cards.tsx`

**Create** và **Delete**: không có.

## Implementation steps

1. **`class-students-tab.tsx`.** Lấy thêm `isOwner` từ `useCenterContext()` (đã import ở :4) và bọc `<Link>` "Thêm học viên" trong `isOwner ? … : null`. Thêm comment một dòng: giáo viên không ghi danh học sinh, nên chỉ chủ trung tâm thấy lối vào trang Học sinh để thêm.
2. **`class-info-tab.tsx`.**
   - Đổi lối tắt ở :106 thành `/students?class_id=${klass.id}` với nhãn "Học sinh của lớp". Trang Học sinh mở tab "Theo lớp" khi có `class_id` (`students-page.tsx:34-61`).
   - Xoá khối `isOwner ? <li>…Cấu hình lớp học…</li>` ở :122-126. Nếu sau đó `isOwner` chỉ còn được dùng cho `ClassProgramCard` thì vẫn giữ.
3. **`class-sessions-tab.tsx`.** Đổi link thứ hai thành `to={`/sessions?class_id=${klass.id}`}` với nhãn "Điểm danh".
4. **`class-list-page.tsx`.**
   - Đọc `useSearchParams()`. Tính `creating = canCreate && (localCreating || searchParams.get("create") === "1")`, hoặc đơn giản hơn là khởi tạo state từ param.
   - Khi `onOpenChange(false)`: nếu param `create` đang có thì xoá nó bằng `setSearchParams(next, { replace: true })`, theo cách `setEdit` ở `class-detail-page.tsx:44-48`.
   - Chọn một nguồn sự thật duy nhất là URL: nút "+ Lớp học" cũng set `create=1`, để không có hai state song song.
5. **`classbook-page.tsx`.** Đổi điều kiện ở :371 từ `isOwner` sang `has("classes.create")`. Hiện :84 chỉ lấy `{ centerId, isOwner }` từ `useCenterContext()`, nên cần lấy thêm `has`; nếu `isOwner` không còn chỗ dùng nào khác thì bỏ nó khỏi destructuring. Sau đó đổi `navigate("/center/classes")` thành `navigate("/classes?create=1")`.
6. **`class-overview-cards.tsx`.**
   - :21 đổi thành một comment tiếng Anh (như các comment khác trong file) có ý: chỉ chủ trung tâm ghi danh học sinh, nên chỉ họ thấy thẻ "Lớp mới". Ví dụ: `/** Only the owner enrolls students, so only they see the 'Lớp mới' card. */`.
   - :36-37 viết lại theo cùng lý do: thẻ lớp chưa có buổi mở trang Học sinh để thêm học sinh, việc chỉ chủ trung tâm làm được.
   - :39-40 thay "roster" bằng "the students page".
   - :119-121 đổi gợi ý sang mục "Danh mục lớp", comment :119 cũng sửa theo.
7. **Test.**
   - `class-detail-page.test.tsx`:
     - :172-181: assert "Học sinh của lớp" có href `/students?class_id=…`, và "Cấu hình lớp học" không còn;
     - :270: bỏ assert đã vô nghĩa, hoặc giữ ở dạng `queryByRole(...)` là null;
     - :285: tách thành hai ca, chủ trung tâm thấy "Thêm học viên" và giáo viên không thấy, dùng cách dựng context giáo viên đang có ở ca :265;
     - :349-352: assert link "Điểm danh" có href `/sessions?class_id=…` và chỉ còn một link tới `/classbook?class_id=`.
   - `class-list-page.test.tsx`: `/classes?create=1` mở hộp tạo lớp cho người có `classes.create`, đóng hộp thì URL không còn `create`; người không có quyền thì không mở.
   - `classbook-page.test.tsx`: khi không có lớp, người có `classes.create` thấy nút "Tạo lớp" và bấm vào thì tới `/classes?create=1`; người không có quyền thì không thấy nút.

## Todo

- [x] Gate "Thêm học viên"
- [x] Sửa lối tắt ở tab thông tin lớp
- [x] Sửa link trùng ở tab Buổi học
- [x] `/classes?create=1` và nút "Tạo lớp" của Sổ lớp
- [x] Sửa comment và gợi ý ở `class-overview-cards`
- [x] Cập nhật và thêm test

## Verification

Chạy trong `apps/web`:

- `npx vitest run src/features/roster src/features/teaching src/features/dashboard`
- `npm run typecheck && npm run lint && npm run format:check`
- `rg -n '/center/classes|/records\?|/center/class-config|Quản trị học sinh' src` không trả kết quả nào, trừ file redirect của phase 2 và phase 4.

## Risk assessment

| Rủi ro | Khả năng × Tác động | Giảm thiểu |
|---|---|---|
| Xung đột với phase 1 trong `class-list-page.tsx` và test của nó | Trung bình × Thấp | Phase 5 chạy sau phase 1 trên cùng nhánh |
| `create=1` còn sót lại trong URL làm hộp mở lại khi người dùng bấm back | Thấp × Thấp | Xoá param bằng `replace` khi đóng hộp |
| Giáo viên từng dùng "Thêm học viên" giờ không thấy nữa | Thấp × Thấp | Đây là quyết định sản phẩm đã có. Server cũng không cho giáo viên ghi danh |

## Security

Các gate ở phase này chỉ là gate giao diện. Quyền thật vẫn do API kiểm tra: `classes.create` cho việc tạo lớp và quyền ghi danh ở enrollment. Phase không mở thêm hành động nào.

## Rollback

`git revert` commit của phase. Phase không đụng dữ liệu hay API.

## Open decisions

- Gate "Thêm học viên" theo `isOwner`. Phương án khác là theo `has("students.create")` nếu sau này giao quyền này cho vai trò khác.
- Nút "Tạo lớp" của Sổ lớp dẫn tới `/classes?create=1` để mở hộp tạo ngay. Phương án đơn giản hơn là chỉ dẫn tới `/classes`.
- Lối tắt học sinh ở tab thông tin lớp: đề xuất đổi sang `/students?class_id=`. Phương án khác là bỏ hẳn, vì tab "Học viên" của chính lớp đã có danh sách.
- Gợi ý ở Tổng quan đổi sang mục "Danh mục lớp".
