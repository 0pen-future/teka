---
phase: 4
title: "Native <select> → HvSelect (phân quyền, nhân sự lớp, bàn giao)"
status: completed
priority: P1
effort: "5h"
dependencies: [1]
---

# Phase 4: Native `<select>` → `HvSelect` (phân quyền, nhân sự lớp, bàn giao)

## Overview

Chuyển 4 `<select>` native (vai trò thành viên, override từng quyền, chọn thành viên nhân sự lớp, bàn giao giáo viên) sang `HvSelect`. Đây là nhóm đổi nhiều test nhất vì `user.selectOptions` / Playwright `selectOption` / `toHaveValue` / `inputValue()` đều ném lỗi trên `button[role=combobox]` — mọi cách chọn và cách đọc giá trị trong 5 test file + 2 e2e phải viết lại theo D11, giữ nguyên ý nghĩa assertion (constraint 5).

## Requirements

- Functional:
  - Vai trò (`member-permissions-dialog.tsx` dòng 152): `id="member-role"` cho `<label htmlFor>`; khi `member.role_id === null` → `value=""` + `placeholder="Giáo viên (mặc định)"` (thay option disabled); `disabled={assignRole.isPending}`; `sheetTitle="Vai trò"`; `w-full`. **Guard D7:** `handleRoleChange` (dòng 155) bắt đầu bằng `if (next === (member.role_id ?? "")) return;` — `<select>` không phát `change` khi chọn lại, `HvSelect` thì có; thiếu guard sẽ gửi `PUT …/role` + toast thừa.
  - Override từng quyền (dòng 209): `aria-label={\`Quyền ${permission.label}\`}`; 3 option inherit/grant/deny với nhãn cũ; `className="w-[150px] shrink-0"`; `sheetTitle={permission.label}`; `searchThreshold` mặc định (3 mục → không ô lọc).
  - Nhân sự lớp (`class-staff-section.tsx` dòng 241): `aria-label={\`Chọn ${roleLabel.toLowerCase()}\`}`; `value=""` + `placeholder="— Chọn thành viên —"`; `disabled={assign.isPending}`; `searchNoun="thành viên"`; `sheetTitle={\`Chọn ${roleLabel.toLowerCase()}\`}`.
  - Bàn giao (`class-settings-page.tsx` dòng 344): `id="handoff-teacher"` trong `Field`; placeholder "— Chọn giáo viên —"; **giữ nguyên nhãn** `label: member.is_owner ? \`${member.full_name} (chủ trung tâm)\` : member.full_name`, **không** chuyển sang `meta` (e2e `class-staff-write` dòng 155 và `ensureClassTeacher` dòng 195 tìm option theo đúng chuỗi `Tên (chủ trung tâm)`; accessible name của option `label`+`meta` không có khoảng trắng nên sẽ không khớp); **guard D7:** handler (dòng 347–351) `if (next === targetId) return;` trước `setArming(false)` — chọn lại cùng người không được un-arm; `disabled={reassign.isPending}`; `searchNoun="giáo viên"`; `sheetTitle="Bàn giao cho"`.
  - Nhân sự lớp & override quyền: handler chỉ set state/gọi mutation với giá trị mới; áp guard `if (next === current) return;` ở override quyền (tránh đánh dấu dirty thừa), nhân sự lớp chỉ set `selectedId` nên không cần.
- Non-functional: nghiệp vụ (assign role một chiều, replaceOverrides dirty, arm/confirm) không đổi; test/e2e chỉ đổi **cách chọn** (click combobox + option) và **cách đọc giá trị** (text trigger / `data-value`, D11); assertion phủ định về option phải mở dropdown và scope vào `listbox` (constraint 5).

## Architecture

Không thêm state. Mỗi chỗ map sang `HvSelectOption[]`. Riêng hàng quyền: N `HvSelect` trong một list — mỗi cái tự giữ `open`; chỉ một popover mở tại một thời điểm nhờ Radix DismissableLayer (click ngoài đóng cái trước).

## Related Code Files

- Modify (số dòng tại thời điểm lập plan; kiểm kê lại bằng `grep -rn 'selectOptions\|toHaveValue\|findByRole("dialog")' apps/web/src/features/{center,roster}/__tests__` và `grep -rn 'selectOption\|inputValue' apps/web/e2e`):
  - `apps/web/src/features/center/components/member-permissions-dialog.tsx`, `apps/web/src/features/roster/components/class-staff-section.tsx`, `apps/web/src/features/roster/pages/class-settings-page.tsx`
  - `apps/web/src/features/center/__tests__/center-permissions.test.tsx`: dòng 295 `toHaveValue("")` → `expect(combobox).toHaveTextContent("Giáo viên (mặc định)")` (placeholder = chưa có vai trò); 296, 340–342 `selectOptions` → `click(combobox)` + `click(option)`; `findByRole("dialog")` trần ở 292, 333 → `mockViewport(1024)` trong `beforeEach` (nhánh sheet sẽ tạo `dialog` thứ hai) và query dialog theo tên.
  - `apps/web/src/features/center/__tests__/center-page.test.tsx` (**không** phải "verify unchanged"): 236, 280, 311 `selectOptions` trên select vai trò/override → click; 235, 279, 310 `findByRole("dialog")` trần → `mockViewport(1024)` trong `beforeEach` + query theo tên.
  - `apps/web/src/features/roster/__tests__/class-staff-section.test.tsx`: 4 chỗ `selectOptions` → click; thêm `mockViewport(1024)` trong `beforeEach`.
  - `apps/web/src/features/roster/__tests__/class-settings-handoff.test.tsx`: dòng 72–73 `within(select).queryByRole("option", …)` (assert giáo viên hiện tại **không** có trong danh sách) → mở combobox trước, `const list = within(await screen.findByRole("listbox"))`, assert `list.queryByRole("option", { name: /Cô A/ })` null **và** `list.getAllByRole("option")` có đúng số thành viên còn lại (assertion dương chống phantom); 91, 112 `selectOptions` → click.
  - `apps/web/e2e/class-staff-write.spec.ts`: dòng 155 `selectOption({ label })` → click `getByRole("combobox", { name: "Bàn giao cho" })` + `page.getByRole("option", { name })` (option portal ra `body`, **không** scope trong `card`); dòng 195 `ensureClassTeacher` (chạy trong `afterEach` để trả lớp về owner) sửa cùng cách — nếu bỏ sót, `afterEach` ném lỗi và DB e2e dùng lại bị lệch state cho spec sau.
  - `apps/web/e2e/secretary-send.spec.ts`: dòng 34–41 `selectOption(target)` → click combobox `Quyền Gửi báo cáo học phí` + `page.getByRole("option", { name })` với bảng value→nhãn `{ grant: "Cấp riêng", inherit: "Theo vai trò", deny: "Chặn riêng" }`; dòng 36–37 assert-then-set đọc `inputValue()` → `await combobox.getAttribute("data-value")` (D11) rồi so với `target`; giữ logic idempotent (chỉ đổi khi khác).
- Verify unchanged: `apps/web/src/features/roster/__tests__/class-config-page.test.tsx` (không chạm 4 select này)

## Implementation Steps

1. `member-permissions-dialog.tsx`: thay 2 `<select>`; xoá class tự viết; giữ `HvBadge` và mô tả; thêm guard D7 ở `handleRoleChange` và override.
2. `class-staff-section.tsx`: thay `<select>`; hàng `flex-wrap` giữ nguyên, `HvSelect className="min-w-[230px] max-sm:w-full"` (chiều rộng consumer tự truyền, D10).
3. `class-settings-page.tsx`: thay `<select>` trong `Field`; nhãn giữ ` (chủ trung tâm)`; tính `targetName` từ `targets` như cũ; guard D7 trước `setArming(false)`.
4. Sửa 4 unit test (center-permissions, center-page, class-staff-section, class-settings-handoff) theo mẫu chung: `mockViewport(1024)` trong `beforeEach`, `findByRole("dialog", { name })`, click combobox + option, đọc giá trị qua text trigger; helper cục bộ `pickOption(user, comboboxName, optionName)` trong từng test file nếu lặp >2 lần (4 file cần → cân nhắc `src/test/pick-option.ts` dùng chung, quyết định lúc làm, ghi lại).
5. Sửa 2 e2e theo mẫu click + option (kể cả `ensureClassTeacher` ở `class-staff-write`); `secretary-send` dùng `dialog.getByRole("combobox", { name: "Quyền Gửi báo cáo học phí" })`, đọc `data-value`, rồi `page.getByRole("option", { name })` theo bảng value→nhãn (option nằm trong portal ngoài dialog).
6. `npx vitest run src/features/center src/features/roster`; `npm run lint && npm run typecheck`; commit `refactor(web): member, staff and handoff pickers on HvSelect`.

## Success Criteria

- [x] `grep -rn "<select" apps/web/src` = 0; `grep -rn 'selectOptions' apps/web/src` = 0; `grep -rn 'selectOption(' apps/web/e2e` = 0. (Grep gốc `toHaveValue`/`inputValue` bắt nhầm 8 ô text không liên quan — `Tên cột điểm`, `Tên lớp`, `Giờ học khung`, link mời trong `invite-accept.spec.ts` — nên thu hẹp về matcher của `<select>`.)
- [x] `center-permissions`, `center-page`, `class-config-page`, `class-staff-section`, `class-settings-handoff` test xanh; ý nghĩa assertion nghiệp vụ không đổi; test handoff có assertion dương về số option.
- [x] Test mới hoặc mở rộng: chọn lại đúng vai trò đang chọn → **không** gọi `PUT …/role`; chọn lại đúng giáo viên đang arm → vẫn arm (D7).
- [x] Hộp thoại phân quyền với ~20 quyền: mở/đóng từng dropdown mượt, chỉ một popover mở, cuộn hộp thoại không kẹt.
- [x] Bàn giao: chọn giáo viên → dòng "Bàn giao lớp cho X?" hiện, đổi người → un-arm.

## Risk Assessment

- **Hàng quyền: N popover trong dialog `overflow-y-auto`:** Popover portal ra body nên không bị clip; Radix cập nhật vị trí khi cuộn. Tín hiệu vỡ: popover trôi khỏi trigger khi cuộn dialog → thêm `updatePositionStrategy="always"` ở `Popover.Content` trong `HvSelect`. **Chưa kiểm chứng** prop này tồn tại trong bản `radix-ui` đang cài (session lập plan không đọc được `node_modules`): bước đầu khi làm là `grep updatePositionStrategy` trong `node_modules/@radix-ui/react-popper`; nếu không có, fallback: đóng popover khi dialog cuộn (`onScroll` capture ở `HvModal` body → `setOpen(false)`).
- **`center-page.test.tsx` từng bị coi là "không đụng":** file này gọi `selectOptions` 3 lần và `findByRole("dialog")` trần 3 lần → chắc chắn đỏ; đã đưa vào Modify. Tín hiệu vỡ tương tự ở file khác: `grep` ở Related Code Files chạy trước khi bắt đầu.
- **e2e trên DB dùng lại:** `class-staff-write` restore trong `afterEach` và `secretary-send` assert-then-set là hai chỗ giữ idempotency; sửa cả hai cùng lúc với chỗ chọn chính, chạy spec 2 lần liên tiếp để chứng minh.
- **Placeholder disabled "Giáo viên (mặc định)" mất ngữ nghĩa "không chọn lại được":** với `value=""` + placeholder, option rỗng không tồn tại nên vẫn không chọn lại được — đúng ý cũ. Tín hiệu vỡ: test center-permissions kỳ vọng option disabled → sửa test sang kỳ vọng placeholder (assertion về nghiệp vụ "một chiều" không đổi).
- **e2e `selectOption` thay bằng click trong dialog:** Playwright strict mode có thể thấy 2 option cùng tên khi hai popover mở → luôn đóng (Esc) trước khi mở cái khác trong spec.
