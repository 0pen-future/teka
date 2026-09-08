---
title: "Đồng bộ mọi dropdown trong apps/web theo DS của dropdown Lớp (Hồ sơ học sinh)"
description: "Trích dropdown Lớp ở /records thành primitive HvSelect trong hv kit rồi thay 10 dropdown còn lại (4 Radix Select, 4 native select) bằng cùng một component: trigger, popover, option, ô lọc, bottom sheet dưới sm."
status: completed
priority: P2
effort: "23h"
branch: master
tags: [frontend, refactor, design-system, web]
blockedBy: []
blocks: []
created: 2026-09-08
---

# Đồng bộ mọi dropdown trong apps/web theo DS của dropdown Lớp

Nguồn chuẩn: `apps/web/src/features/teaching/components/records-class-select.tsx` (dropdown **Lớp** trên trang **Hồ sơ học sinh** `/records`, sinh từ plan [260907-0434](../260907-0434-records-class-dropdown-student-search/plan.md), D4). Chế độ plan: `hard` (không cần researcher ngoài, toàn bộ bằng chứng nằm trong repo).

## Overview

`apps/web` hiện có **3 hệ dropdown** cùng tồn tại: (a) popover-listbox + bottom sheet của `/records` (chuẩn DS), (b) Radix `Select` bọc shadcn trong `components/ui/select.tsx` (4 chỗ), (c) `<select>` native với class tự viết (4 chỗ). Kiểu chữ, bo góc, viền, màu hover/selected, cách lọc, hành vi trên điện thoại khác nhau ở từng chỗ. Plan này trích (a) thành primitive dùng chung **`HvSelect`** trong `components/hv/` (đúng quy tắc "new shared primitives belong here" của `docs/frontend-guidelines.md`), rồi chuyển toàn bộ (b) và (c) sang `HvSelect`, xoá `components/ui/select.tsx` ở Phase 5 khi không còn consumer.

## Contract

- **Outcome:** Mọi dropdown chọn-một-giá-trị trong `apps/web` trông và hoạt động y hệt dropdown Lớp ở Hồ sơ học sinh: trigger bo 14px viền 2px `line-200`, chữ 14.5px extrabold `ink-900`, chevron xoay khi mở; popover bo 16px viền 2px shadow-soft-lg; option cao 42px bo 11px, dấu ✓ mint ở mục đang chọn, hover `cream-100`, selected `mint-50/mint-700`; nhãn nhóm uppercase 11px; ô "Tìm …" xuất hiện khi >5 mục; dưới `sm` mở thành bottom sheet `HvModal`; ↑↓ Home End Enter Space Esc và gõ-để-lọc như chuẩn.
- **Constraints:** (1) Một primitive duy nhất, prop-driven; không copy class Tailwind sang từng trang. (2) Chỉ dùng token DS và primitive sẵn có (`HvModal`, `radix-ui` Popover, `useMediaQuery`, `lucide-react`); không thêm dependency. (3) Không đổi dữ liệu hiển thị của từng dropdown (classbook vẫn `Tên · lịch` trên trigger, enroll vẫn `lịch · giá/buổi`, audit vẫn "Tất cả …", bàn giao vẫn `Tên (chủ trung tâm)`); chỉ đổi vỏ và hành vi. Riêng **option** theo chuẩn `/records` hiện `label` và `meta` ở hai span cạnh nhau, **không** có dấu `·` (dấu `·` chỉ có trên trigger) — xem D4. (4) Nhãn form (`FieldLabel`, `<label>`) và bố cục xung quanh giữ nguyên. (5) Test/e2e hiện có giữ nguyên **ý nghĩa** assertion nghiệp vụ nhưng được phép viết lại **cách quan sát**: locator/role, cách chọn (`selectOptions`/`selectOption` → click combobox + click option), và cách đọc giá trị đang chọn (`toHaveValue`/`inputValue()` → text trigger theo D11). Assertion phủ định về option ("X không có trong danh sách") phải mở dropdown trước và scope vào `listbox` đang mở, kèm một assertion dương (ví dụ số option) để không thành phantom test. (6) `npm run lint`, `typecheck`, `test`, `build` và e2e liên quan xanh trên stack cách ly.
- **Non-goals:** `ModeToggle` (`DropdownMenu` hành động, hiện **không được mount** ở đâu); ba danh sách tìm-kiếm-luôn-mở `contact-picker`, `zalo-friend-picker`, `enroll-existing-student-dialog` (combobox có ô nhập, không có trigger — không phải dropdown); dãy pill lớp ở Điểm danh và Lớp & học sinh (`role=tablist`, đã là non-goal của plan trước); bỏ dấu tiếng Việt trong ô lọc (chuẩn hiện tại không bỏ dấu); multi-select; typeahead khi ô lọc ẩn.
- **Acceptance criteria:** mục *Success Criteria* dưới + tiêu chí từng phase.

## Kiểm kê dropdown (10 vị trí / 8 file)

| # | Màn hình | File | Hiện tại | Ghi chú dữ liệu |
|---|----------|------|----------|-----------------|
| 1 | Hồ sơ học sinh — Lớp | `features/teaching/components/records-class-select.tsx` | **Chuẩn** (Popover + listbox + sheet) | `Tên · N HS`, nhóm "Lớp đang dạy", lọc khi >5 |
| 2 | Sổ lớp — Chọn lớp | `features/teaching/components/class-select.tsx` | Radix Select, `border-0 shadow-soft-sm` | `Tên · lịch`; `aria-label="Chọn lớp — đang xem …"` |
| 3 | Nhật ký hoạt động — Giáo viên | `features/audit/components/audit-filters.tsx` | Radix Select `w-[180px]` | "Tất cả giáo viên" + thành viên |
| 4 | Nhật ký hoạt động — Nhóm hành động | `features/audit/components/audit-filters.tsx` | Radix Select `w-[180px]` | option `disabled` "Tùy chỉnh" |
| 5 | Ghi nhận thanh toán — Hình thức | `features/collections/components/record-payment-dialog.tsx` | Radix Select trong `HvModal`, `aria-invalid` | 3 hình thức |
| 6 | Ghi danh vào lớp — Lớp | `features/roster/components/enroll-student-dialog.tsx` | Radix Select trong `HvModal`, placeholder | `Tên — lịch · giá/buổi` |
| 7 | Phân quyền — Vai trò | `features/center/components/member-permissions-dialog.tsx` | native `<select>`, disabled khi pending | placeholder disabled "Giáo viên (mặc định)" |
| 8 | Phân quyền — từng quyền (N hàng) | `features/center/components/member-permissions-dialog.tsx` | native `<select>` `px-2 text-[13px]` | 3 giá trị inherit/grant/deny |
| 9 | Cài đặt lớp — Chọn thành viên (nhân sự) | `features/roster/components/class-staff-section.tsx` | native `<select>` | "— Chọn thành viên —" + thành viên |
| 10 | Cài đặt lớp — Bàn giao cho | `features/roster/pages/class-settings-page.tsx` | native `<select>` trong `Field` | "— Chọn giáo viên —" + `(chủ trung tâm)` |

## Quyết định thiết kế đã chốt

| # | Quyết định | Lý do / phương án bị loại |
|---|-----------|---------------------------|
| D1 | Primitive **`HvSelect`** mới trong `components/hv/hv-select.tsx`, export từ `components/hv/index.ts`. `RecordsClassSelect` bị **xoá** (records-toolbar dùng thẳng `HvSelect`); `ClassSelect` của classbook giữ file làm wrapper mỏng chỉ tính nhãn. | Copy class sang từng chỗ: vi phạm DRY, lệch dần. Mở rộng `records-class-select.tsx` tại chỗ: primitive nằm trong feature, vi phạm quy tắc hv kit. |
| D2 | Trigger có **`role="combobox"`** + `aria-haspopup="listbox"` + `aria-expanded` + `aria-controls` (**luôn** phát, không chỉ khi mở). Lý do chính là **nhất quán**: 8/10 vị trí đang được test/e2e định vị bằng `combobox`, chỉ records dùng `button`. Lưu ý trung thực: đây là biến thể *combobox mở popup listbox* với **roving focus** (focus DOM di chuyển vào option, không dùng `aria-activedescendant`) — không phải mẫu APG select-only combobox thuần; không viện dẫn APG khi review a11y. Hệ quả: 2 unit test + 3 e2e của records đổi `button` → `combobox` (Phase 2, số dòng theo grep). | Giữ `role=button`: phải sửa 5 test/e2e khác. Chuyển sang `aria-activedescendant`: viết lại toàn bộ điều hướng của chuẩn, ngoài scope. |
| D3 | Tên truy cập theo 3 cách, không trộn: `labelId` → `aria-labelledby="{labelId} {triggerTextId}"` (giữ "Lớp Toán 6A · 28 HS" của records); `aria-label` → dùng nguyên (classbook giữ "Chọn lớp — đang xem …"); `<label htmlFor={id}>` → tên = nhãn (như native select, test "Vai trò"/"Lớp" giữ nguyên). | Luôn ghép nhãn + giá trị: vỡ test `{ name: "Vai trò" }` và `{ name: "Lớp" }` exact. |
| D4 | Model option `{ value, label, meta?, disabled? }`. **Trigger** hiện `label · meta` (meta chữ 12.5px bold `ink-400`, có ` · `). **Option** giữ đúng markup chuẩn: `<span class="flex-1">label</span><span class="text-[12px] font-bold text-ink-400">meta</span>` — **không** separator, meta căn phải. Hệ quả cho test: accessible name / textContent của option là `label` nối liền `meta` (ví dụ `Toán 6BTối Thứ Ba`), nên test/e2e tìm option bằng regex trên `label` (`/Toán 6B/`) và **không** assert chuỗi `label · meta` trên option (`classbook-page.test.tsx:444` phải sửa, Phase 2). Records: meta `N HS`; classbook: meta = lịch; enroll: meta = `lịch · giá/buổi`; bàn giao: **không** meta, giữ ` (chủ trung tâm)` trong `label` để e2e/`ensureClassTeacher` không đổi nhãn; các chỗ khác không meta. | Thêm ` · ` vào option: đổi vỏ của chính chuẩn `/records` (option thành `Toán 6A · 28 HS`), trái mục tiêu. Render-prop `renderOption`: linh hoạt thừa, mỗi consumer tự vẽ lại → lệch DS. |
| D5 | Ô lọc: prop `searchNoun` ("lớp", "thành viên"…) sinh placeholder `Tìm {noun}…`, aria-label `Tìm {noun}`, note `Không có {noun} nào khớp "q"`. Hiện khi số option **> `searchThreshold`** (mặc định 5, giống chuẩn và `useClassSearch`). Lọc substring không phân biệt hoa thường trên `label`. Hook nội bộ `useOptionSearch` trong `hv-select.tsx`; **không đụng** `useClassSearch`/`ClassSearchInput` (pill Điểm danh, Lớp & học sinh còn dùng). | Tái dùng `useClassSearch`: bị khoá vào `{id,name}` và chữ "lớp". |
| D6 | Dưới `sm` (`useMediaQuery("(min-width: 640px)")` false) mở **bottom sheet `HvModal` size md** với `sheetTitle` (prop bắt buộc, ví dụ "Chọn lớp", "Hình thức", "Vai trò"). Cùng list body với popover, focus ban đầu vào ô lọc → option đang chọn → option đầu; đóng trả focus về trigger. | Popover ở mọi breakpoint: lệch chuẩn. |
| D7 | `onValueChange(value)` **luôn** được gọi khi chọn, kể cả chọn lại giá trị hiện tại (records cần để ghim `class_id` vào URL). **Consumer có side effect ghi hoặc đổi trạng thái** (assign role → `PUT …/role` + toast; bàn giao → `setArming(false)`; classbook → `onSelect`) **bắt buộc** guard `if (next === value) return;` — `<select>` native không phát `change` khi chọn lại, nên thiếu guard là đổi hành vi (ghi thừa, un-arm bất ngờ). Consumer chỉ set state thuần (audit filter, form field) không cần guard. | Guard trong primitive: vỡ hành vi records đã chốt. |
| D8 | Xoá `components/ui/select.tsx` ở **bước đầu Phase 5** (sau khi Phase 2 **và** 3 đã merge — `class-select.tsx` của Phase 2 cũng import file này, nên xoá ở Phase 3 sẽ vỡ khi Phase 2/3 chạy song song). Gói `radix-ui` giữ nguyên (Popover/Dialog vẫn dùng). | Xoá ở Phase 3: bản dựng đỏ khi merge Phase 2 + 3 mà không gate phase nào bắt được; revert Phase 2 sau Phase 3 cũng vỡ. Giữ file chết: mời gọi dùng lại → 2 hệ dropdown quay lại. |
| D9 | Shim `matchMedia` mặc định trong test trả `false` → `HvSelect` mở **sheet** trong jsdom, tức một `dialog` **thứ hai** (kèm nút "Đóng hộp thoại" thứ hai) đè lên form. Mọi test file chạm tới consumer **bắt buộc** gọi `mockViewport(1024)` trong `beforeEach` (không ở module scope — `viewportWidth` trong `test/viewport.ts` là biến module, không tự reset giữa các test) trừ test cố ý kiểm sheet. Danh sách file có query `findByRole("dialog")` trần sẽ ném "multiple elements" ở nhánh sheet: `center-page.test.tsx` (235, 279, 310), `center-permissions.test.tsx` (292, 333), `record-payment-dialog.test.tsx`, `enroll-student-dialog.test.tsx`. Option tìm bằng `screen` (portal ngoài dialog form), không `within(dialog)`. Phase 1 có test `HvSelect` lồng trong `HvModal` cho cả hai nhánh để chứng minh nested dialog trước khi consumer nào dùng. | Đổi shim mặc định: ảnh hưởng test khác đang dựa vào nhánh hẹp. |
| D10 | Kích thước: một cỡ (min-h-11 = 44px hit area theo quy tắc hv kit). Class cơ sở của trigger **không** chứa `min-w-[230px] max-sm:w-full` (twMerge không gộp `min-w-*` với `w-*`, và CSS `min-width` thắng `width` → `w-[180px]` sẽ thành 230px nếu để trong cơ sở); records tự truyền `className="min-w-[230px] max-sm:w-full"`. Chiều rộng consumer khác qua `className` (`w-full` trong form, `w-[180px]` audit, `w-[150px] shrink-0` hàng quyền). Không thêm `size="sm"`. | Hàng quyền dùng cỡ nhỏ: lệch DS, thêm biến thể chưa cần. |
| D11 | **Đọc giá trị đang chọn** (thay `toHaveValue`/`inputValue()` vốn chỉ chạy trên form element): text của trigger — span `id={triggerTextId}` chứa `label` (và ` · meta`), placeholder khi rỗng. Unit test: `expect(combobox).toHaveTextContent("Cấp riêng")`; e2e: `await expect(combobox).toHaveText(/Cấp riêng/)` hoặc `(await combobox.textContent())`. Trigger phát thêm `data-value={value}` để e2e so sánh value thô khi cần (secretary-send so `"grant"`/`"inherit"`). | Chỉ so text: e2e phải mang bảng value→nhãn; `data-value` rẻ và deterministic. |
| D12 | `div[role=listbox]` lấy `aria-label` từ `sheetTitle` (chuẩn hiện hardcode "Chọn lớp"). Ô lọc: `aria-label="Tìm {searchNoun}"`. Note không khớp render **inline** trong `hv-select.tsx` (không import `ClassSearchEmptyNote` từ `@/features/roster` — hv kit không được phụ thuộc feature). | Import từ feature: đảo chiều layering `docs/frontend-guidelines.md`, tạo vòng import features↔components mà eslint hiện không chặn. |

## Phases

| # | Phase | Status | Phụ thuộc | Ước lượng |
|---|-------|--------|-----------|-----------|
| 1 | [Primitive `HvSelect` trong hv kit](./phase-01-hv-select-primitive.md) | Completed | — | 7h |
| 2 | [Teaching: records + classbook dùng `HvSelect`](./phase-02-teaching-consumers.md) | Completed | 1 | 4h |
| 3 | [Radix Select → `HvSelect` (audit, thanh toán, ghi danh)](./phase-03-radix-select-consumers.md) | Completed | 1 | 3h |
| 4 | [Native `<select>` → `HvSelect` (phân quyền, nhân sự lớp, bàn giao)](./phase-04-native-select-consumers.md) | Completed | 1 | 5h |
| 5 | [Xoá `ui/select.tsx`, verify toàn bộ, e2e cách ly, docs, QA thị giác](./phase-05-verify-docs-qa.md) | Completed | 2, 3, 4 | 4h |

Phase 2, 3, 4 độc lập về file sau Phase 1 (teaching / audit+collections+roster-enroll / center+roster-staff+roster-settings) → có thể chạy song song **với điều kiện** không phase nào xoá file dùng chung: `components/ui/select.tsx` chỉ xoá ở Phase 5 (D8). Mỗi phase chạy vitest thư mục của mình; Phase 5 chạy toàn bộ.

## Files (tóm tắt)

- **Tạo:** `apps/web/src/components/hv/hv-select.tsx`, `apps/web/src/components/hv/__tests__/hv-select.test.tsx`.
- **Sửa:** `components/hv/index.ts`; `teaching/components/{records-toolbar,class-select}.tsx`; `audit/components/audit-filters.tsx`; `collections/components/record-payment-dialog.tsx`; `roster/components/{enroll-student-dialog,class-staff-section}.tsx`; `roster/pages/class-settings-page.tsx`; `center/components/member-permissions-dialog.tsx`; test: `teaching/__tests__/{records-toolbar,records-pages,classbook-page}.test.tsx`, `center/__tests__/{center-permissions,center-page}.test.tsx`, `roster/__tests__/{class-settings-handoff,class-staff-section,enroll-student-dialog}.test.tsx`, `collections/__tests__/record-payment-dialog.test.tsx`, `audit/__tests__/audit-page.test.tsx`; e2e: `records-search`, `class-staff-read`, `class-staff-write`, `secretary-send`; `docs/frontend-guidelines.md`. Có thể sửa thêm (chỉ khi test nested ở Phase 1 chứng minh cần, bước additive): `components/hv/hv-modal.tsx` (thêm prop `onEscapeKeyDown` forward xuống `Dialog.Content`).
- **Xoá:** `teaching/components/records-class-select.tsx`, `teaching/__tests__/records-class-select.test.tsx` (case chuyển sang hv test) ở Phase 2; `components/ui/select.tsx` ở Phase 5.
- **Không đụng:** `roster/hooks/use-class-search.ts`, `roster/components/class-search.tsx` (note không khớp được inline trong hv-select, không tái dùng), `attendance/pages/sessions-page.tsx`, `roster/pages/students-page.tsx`, `components/shared/mode-toggle.tsx`, `components/ui/dropdown-menu.tsx`, ba picker tìm kiếm, `useMediaQuery`, `test/setup.ts` (shim matchMedia), backend.

## Success Criteria

- [x] `grep -rn "components/ui/select\|<select" apps/web/src` không còn kết quả ngoài test fixture; `components/ui/select.tsx` đã xoá.
- [x] 10 vị trí trong bảng kiểm kê đều render `HvSelect`: trigger `role=combobox`, popover `role=listbox` ≥ `sm`, `dialog` tiêu đề `sheetTitle` < `sm`; chụp màn hình desktop + mobile từng vị trí giống dropdown Lớp ở `/records` (cùng token, cùng cỡ).
- [x] Ô lọc chỉ hiện ở dropdown > 5 mục (audit **Nhóm hành động** luôn hiện vì có 18+ mục — `Tìm nhóm hành động…`; audit Giáo viên khi nhiều thành viên; Chọn lớp khi nhiều lớp…), note không khớp đúng mẫu `Không có {noun} nào khớp "q"` với `searchNoun` riêng từng chỗ, không rơi về mặc định "mục".
- [x] `grep -rn "@/features" apps/web/src/components/hv` = 0 (hv kit không phụ thuộc feature); `listbox` có tên = `sheetTitle` ở mọi vị trí (không còn "Chọn lớp" ở Vai trò/Hình thức/Nhóm hành động).
- [x] Phím: ↑↓ Home End di chuyển, Enter/Space chọn, Esc đóng và trả focus về trigger, gõ chữ khi ô lọc hiện → nhảy vào ô lọc; option `disabled` bị bỏ qua khi di chuyển.
- [x] Trạng thái: placeholder `ink-400`; `disabled` nền `cream-200` chữ `ink-300` không mở; `aria-invalid` viền `coral-400`; danh sách dài cuộn trong popover (max-height theo viewport).
- [x] Hành vi nghiệp vụ không đổi: records ghim `class_id` + xoá `q` khi đổi lớp; classbook không gọi `onSelect` khi chọn lại; audit filter áp ngay; record-payment/enroll-student submit đúng giá trị; member role/override, staff assign, handoff arm/confirm giữ nguyên; **chọn lại đúng vai trò/giáo viên đang chọn không gửi request và không un-arm** (D7 guard).
- [x] Trigger bàn giao vẫn hiện `Tên (chủ trung tâm)`; e2e `class-staff-write` (kể cả `ensureClassTeacher` trong `afterEach`) và `secretary-send` (assert-then-set qua `data-value`/text trigger) chạy idempotent trên DB e2e dùng lại.
- [x] `apps/web`: `npm run lint`, `npm run typecheck`, `npm run test`, `npm run build` xanh; e2e `records-search`, `class-staff-read`, `class-staff-write`, `secretary-send`, `roster` xanh trên stack cách ly.
- [x] `docs/frontend-guidelines.md` liệt kê `HvSelect` trong mục hv kit; không còn nhắc `components/ui/select`.

## Risks (tổng hợp — chi tiết ở từng phase)

- **Popover portal bên trong `HvModal` (dialog modal):** Radix DismissableLayer xếp lớp theo thứ tự mount và FocusScope xếp chồng nên popover trong dialog vẫn nhận focus (Radix Select hiện tại đã chứng minh ở record-payment/enroll). Phase 1 có test `HvSelect` trong `HvModal` ở `mockViewport(1024)`. Tín hiệu vỡ: click option trong dialog không chọn được / focus bật về dialog → chuyển `PopoverPrimitive.Root modal` hoặc render sheet cả trong dialog; quyết định ở Phase 1, không để đến Phase 3.
- **Dialog lồng dialog (sheet mở từ form modal trên điện thoại):** repo **chưa có** chỗ nào lồng `HvModal` trong `HvModal`, nên Phase 1 phải chứng minh bằng test (Esc chỉ đóng sheet, form còn mở, focus về trigger, hai `dialog` cùng lúc). Radix DismissableLayer chỉ dismiss layer trên cùng nên kỳ vọng là đúng sẵn. Tín hiệu vỡ: Esc đóng cả form → **không** dùng `stopPropagation` (Radix lắng nghe Esc bằng capture listener trên `document`, `stopPropagation` vô hiệu); phản ứng đã định: thêm prop additive `onEscapeKeyDown` cho `HvModal` (forward xuống `Dialog.Content`) như một bước tường minh của Phase 1, ghi lại trong plan.
- **Test đang mock viewport hẹp mặc định (D9):** quên `mockViewport` → sheet là `dialog` thứ hai, `findByRole("dialog")` trần ném "multiple elements", `within(dialog form)` không thấy option. Xử lý theo D9 (danh sách file đã liệt kê), kiểm tra ngay khi chạy từng test file.
- **Chọn lại giá trị hiện tại (D7):** `<select>` native không phát `change`, `HvSelect` luôn gọi `onValueChange` → consumer có side effect ghi phải guard; Phase 4 có tiêu chí kiểm.
- **Danh sách dài (audit Giáo viên với trung tâm đông):** chuẩn hiện tại không có max-height → Phase 1 thêm `max-h-(--radix-popover-content-available-height) overflow-y-auto` cho popover và `max-h-[60dvh]` cho body sheet. Ô lọc >5 giảm nhu cầu cuộn.
- **Blast radius test:** kiểm kê bằng grep, không bằng số dòng cứng: `grep -rn '"button", { name: /\^Lớp' apps/web/src apps/web/e2e` (records: 2 + 5 dòng unit, 3 + 1 + 1 dòng e2e), `grep -rn 'selectOptions\|toHaveValue' apps/web/src/features/{center,roster}` (5 file), `grep -rn 'selectOption\|inputValue' apps/web/e2e` (2 spec). Mỗi phase chạy lại grep của mình trước khi commit; Phase 5 chạy toàn bộ.

## Rollback

Mỗi phase là commit riêng trên nhánh `feat/dropdown-design-system`. Phase 2/3/4 độc lập nên revert từng commit trả lại Radix/native select tương ứng — nhưng nếu commit xoá `ui/select.tsx` của Phase 5 đã vào, phải revert commit đó **trước** khi revert Phase 2 hoặc 3 (hai phase này trả lại consumer của file). Phase 1 là additive (chưa consumer nào phụ thuộc cho tới Phase 2). Không migration, không đổi API.

## Red Team Review

| Session | Ngày | Reviewer | Findings | Accepted | Rejected |
|---------|------|----------|----------|----------|----------|
| 1 | 2026-09-08 | Assumption Destroyer (Fact Checker + Scope Auditor), Failure Mode Analyst (Flow Tracer), Security Adversary (Contract Verifier) | 29 thô → 15 sau khử trùng | 14 | 1 |

Báo cáo: [assumptions](../reports/red-team-260908-assumptions-dropdown-design-system.md), [failure](../reports/red-team-260908-failure-dropdown-design-system.md), [security](../reports/red-team-260908-security-dropdown-design-system.md).

Đã áp dụng (tóm tắt): `center-page.test.tsx` chuyển sang Modify (Phase 4); e2e `secretary-send` đọc state qua `data-value`/text trigger (D11); bàn giao giữ ` (chủ trung tâm)` trong `label` (D4); D4 sửa đúng markup option không separator, `classbook-page.test.tsx:444` vào danh sách sửa; xoá `ui/select.tsx` dời sang Phase 5 (D8); kiểm kê locator bằng grep; D7 buộc guard ở consumer có side effect; nested dialog chứng minh bằng test Phase 1, mitigation `onEscapeKeyDown` thành bước additive của `HvModal`; constraint (5) nới cho phép đổi cách quan sát; note không khớp inline + listbox tên theo `sheetTitle` (D12); `min-w-[230px]` tách khỏi class cơ sở (D10); D2 nêu lý do nhất quán thay vì APG; audit Nhóm hành động có `searchNoun`; thêm test `aria-invalid` cho Hình thức thanh toán.

Từ chối: "placeholder làm mất đường reset về rỗng ở bàn giao/nhân sự" — không test/e2e nào chọn lại `""`; chọn người khác là đường sửa; nút xác nhận vẫn gate theo `targetId`. Consumer vẫn có thể truyền option `value=""` nếu sản phẩm cần, không cần prop mới.

Chưa kiểm chứng được trong session: prop `updatePositionStrategy` của Radix `Popover.Content` (hook chặn đọc `node_modules`) — Phase 4 ghi bước xác nhận.

### Whole-Plan Consistency Sweep
- Files reread: plan.md, phase-01-hv-select-primitive.md, phase-02-teaching-consumers.md, phase-03-radix-select-consumers.md, phase-04-native-select-consumers.md, phase-05-verify-docs-qa.md
- Decision deltas checked: 14 (D2 lý do nhất quán + `aria-controls` luôn phát; D4 markup option; D7 guard consumer; D8 xoá `ui/select.tsx` ở Phase 5; D9 `beforeEach` + danh sách dialog trần; D10 tách `min-w`; D11 đọc giá trị; D12 listbox/inline note; constraint (5); Phase 3 title/steps; Phase 4 `center-page`, handoff label, e2e; Phase 5 bước xoá; effort 20h → 23h; nested-dialog mitigation)
- Reconciled stale references: 5 (Overview + Rollback về thời điểm xoá `ui/select.tsx`; Phase 5 docs không gọi "select-only combobox"; Phase 2/3 title & overview; Phases table effort)
- Unresolved contradictions: 0 (một mục chưa kiểm chứng, không phải mâu thuẫn: prop `updatePositionStrategy` — Phase 4 Risk ghi bước xác nhận)

## Kết quả (2026-09-08)

- Nhánh `feat/dropdown-design-system`: 5 commit theo phase (`ba5c9df` primitive, `0de60b7` teaching, `6f321e4` audit/thanh toán/ghi danh, `88e3f9e` phân quyền/nhân sự/bàn giao, `063c449` xoá `ui/select.tsx`) + commit chốt review/docs/plan.
- Gate `apps/web`: typecheck, lint (0 lỗi, 5 warning có sẵn), vitest 85 file / 660 pass / 3 skip, format:check, build — xanh.
- e2e trên stack cách ly `teka-e2e`: 8 passed (2.0m); chạy lại `class-staff-write` + `secretary-send`: 4 passed (1.2m); stack đã `down -v`.
- QA thị giác: 10/10 vị trí × 3 ảnh (đóng 1024 / popover 1024 / sheet 375) khớp chuẩn, không cần sửa `hv-select.tsx` — [báo cáo](../reports/qa-260908-1027-dropdown-design-system.md), ảnh ở `plans/reports/assets/qa-260908-dropdown-design-system/`. Phím tắt 3/3 PASS.
- Review: [Phase 1](../reports/code-review-260908-phase1-hv-select.md), [Phase 2–4](../reports/code-review-260908-phase2-4-consumers.md), [toàn nhánh](../reports/code-review-260908-branch-dropdown-design-system.md). M1 (test "D7" ở override picker không chứng minh gì) đóng bằng cách bỏ guard thừa — consumer thuần state không cần guard, test giữ lại làm regression "chọn lại không làm bẩn form". L2 (`searchNoun` cho vai trò / chế độ / hình thức) và L4 (câu chữ docs) đã sửa.
- Low ghi nhận, không sửa trong plan này: L1 `indexOf` −1 khi focus nằm trên chính listbox; L3 popover Radix mang `role="dialog"` (đặc tính của chuẩn gốc); L5 `sheetTitle` theo tên quyền; L6 options rebuild mỗi render ở audit/enroll (danh sách nhỏ); L7 `useOptionSearch` trùng `use-class-search.ts`; L8 thiếu test đúng 5 mục và lọc trong sheet.
- Chưa kiểm chứng: prop `updatePositionStrategy` (Phase 4 Risk) — không cần tới vì QA dialog lồng nhau đạt.

<!-- slug: dropdown-design-system -->
