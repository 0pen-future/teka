---
title: "Red team (Assumption Destroyer) — plan 260908-0832-dropdown-design-system"
role: FACT CHECKER + SCOPE AUDITOR
plan: ../260908-0832-dropdown-design-system/plan.md
date: 2026-09-08
findings: 10
---

# Red team: Assumption Destroyer — dropdown design system

Mọi finding dưới đây được kiểm chứng bằng grep/read trên source thật trong
`apps/web/src`, `apps/web/e2e` và `docs/`. Không sửa file sản phẩm nào.

## Finding 1: `center-page.test.tsx` bị xếp nhầm vào "Verify unchanged" nhưng chắc chắn đỏ

- **Severity:** Critical
- **Location:** Phase 4, mục "Related Code Files" → "Verify unchanged"
- **Flaw:** Plan viết `center-page.test.tsx` chỉ cần "chạy để xác nhận không đụng
  tới 4 select này". Thực tế file này thao tác đúng vào `<select>` override quyền
  (vị trí #8 trong bảng kiểm kê) ở 3 chỗ.
- **Failure scenario:** Phase 4 thay `<select>` bằng `HvSelect`; `user.selectOptions`
  ném lỗi vì phần tử không còn là `<select>`. Ba test đỏ sau khi Phase 4 đã commit,
  và người thực thi dễ hiểu nhầm là regression nghiệp vụ chứ không phải churn locator.
- **Evidence:**
  - `apps/web/src/features/center/__tests__/center-page.test.tsx:236-239`,
    `:280-283`, `:311-314` — `await user.selectOptions(await within(dialog).findByRole("combobox", { name: "Quyền Gửi báo cáo học phí" }), "grant"|"inherit")`.
  - Đích của locator đó là `apps/web/src/features/center/components/member-permissions-dialog.tsx:209`
    `<select aria-label={\`Quyền ${permission.label}\`}>`.
  - Cùng file test, `:235`, `:279`, `:310` dùng `await screen.findByRole("dialog")`
    không định danh → ở viewport mặc định (hẹp) sheet của `HvSelect` là dialog thứ hai
    và query này ném "found multiple elements".
  - Plan phase-04: "Verify unchanged: `center-page.test.tsx`, `class-config-page.test.tsx`
    (grep cho thấy có `option`/`combobox` — chạy để xác nhận không đụng tới 4 select này)".
- **Suggested fix:** Chuyển `center-page.test.tsx` sang danh sách Modify của Phase 4,
  thêm `mockViewport(1024)` vào `beforeEach`, và cộng effort tương ứng.

## Finding 2: e2e `secretary-send` đọc trạng thái bằng `inputValue()` — plan không có đường thay thế

- **Severity:** Critical
- **Location:** Phase 4, "Related Code Files" và Implementation Step 5
- **Flaw:** Plan chỉ mô tả đổi `selectOption(target)` thành click + option theo nhãn.
  Nhưng spec đọc giá trị hiện tại bằng `inputValue()` để chọn nhánh assert-then-set.
  `HvSelect` là `<button>`, nên `inputValue()` ném
  `Error: Node is not an <input>, <textarea> or <select> element`.
- **Failure scenario:** Spec chết ngay tại dòng điều kiện, trước cả bước chọn. Vì DB e2e
  dùng lại giữa các lần chạy (ghi rõ trong comment ngay trên hàm), mất nhánh "đã đúng thì
  đóng dialog" nghĩa là phải thiết kế lại cách đọc state — không phải đổi locator.
- **Evidence:**
  - `apps/web/e2e/secretary-send.spec.ts:34` `const reportsSend = dialog.getByRole("combobox", { name: "Quyền Gửi báo cáo học phí" })`
  - `:36` `const target = granted ? "grant" : "inherit";` — so sánh theo **value** thô,
    trong khi plan bảo click theo **nhãn** "Cấp riêng"/"Theo vai trò".
  - `:37` `if ((await reportsSend.inputValue()) === target) {`
  - `:41` `await reportsSend.selectOption(target);`
  - Plan phase-04: "`secretary-send.spec.ts` (dòng 34–41 `selectOption(target)` → click +
    option theo nhãn …)" — không nói gì về dòng 37.
- **Suggested fix:** Định nghĩa cách đọc trạng thái mới trước khi làm: cho `HvSelect`
  phát `data-value` trên trigger, hoặc đọc `HvBadge` "Cấp riêng"/"Từ vai trò" đã có sẵn
  trong dialog. Ghi quyết định vào Phase 4.

## Finding 3: D7 biến "chọn lại giá trị cũ" thành một lần ghi thật ở 2 consumer native

- **Severity:** High
- **Location:** D7 trong `plan.md`; Phase 4 "Non-functional: nghiệp vụ … không đổi"
- **Flaw:** `<select>` native không phát `change` khi người dùng chọn lại đúng option
  đang chọn. `HvSelect` theo D7 **luôn** gọi `onValueChange`. Hai consumer gắn thẳng
  side effect vào `onChange`.
- **Failure scenario:**
  1. Phân quyền: mở dropdown Vai trò, bấm đúng vai trò đang có → gửi thêm một
     `PUT /centers/me/members/:id/role` và hiện toast "Đã đổi vai trò" cho một thao tác
     không đổi gì. Đây là một lần ghi có đặc quyền, không phải no-op vô hại.
  2. Bàn giao: người dùng đã bấm "Bàn giao lớp" (arming), mở lại dropdown để xác nhận
     mình chọn đúng người rồi bấm đúng giáo viên đó → `setArming(false)`, nút
     "Xác nhận bàn giao" biến mất không lời giải thích.
- **Evidence:**
  - `apps/web/src/features/center/components/member-permissions-dialog.tsx:155`
    `onChange={(event) => handleRoleChange(event.target.value)}`
  - `apps/web/src/features/roster/pages/class-settings-page.tsx:347-351`
    `setTargetId(event.target.value); … setArming(false);`
  - Plan D7: "`onValueChange(value)` **luôn** được gọi khi chọn, kể cả chọn lại giá trị
    hiện tại"; Phase 4: "nghiệp vụ (assign role một chiều, replaceOverrides dirty,
    arm/confirm) không đổi".
- **Suggested fix:** Ghi rõ trong Phase 4 rằng hai consumer này phải tự guard
  `if (next !== value) …`, giống guard mà classbook đã có.

## Finding 4: `classbook-page.test.tsx` không thể "xanh không sửa" — mâu thuẫn giữa D4 và markup được port

- **Severity:** High
- **Location:** Phase 2 "Requirements → Non-functional" và Success Criteria; D4 trong `plan.md`
- **Flaw:** Test khẳng định textContent của **option** là `"Toán 6B · Tối Thứ Ba"`.
  Markup option port từ `records-class-select.tsx` render `label` và `meta` trong hai
  span cạnh nhau, không có dấu `·` và không có khoảng trắng giữa hai node. Ngược lại,
  nếu thêm `·` vào option theo D4 thì option của `/records` thành "Toán 6A · 28 HS",
  khác chuẩn mà Phase 1 (so chuỗi class) và Phase 5 (so ảnh) yêu cầu giữ nguyên.
- **Failure scenario:** Không có cách triển khai nào thoả đồng thời ba điều: option có
  separator `·` (cho classbook), option giống hệt `/records` hôm nay, và không sửa
  `classbook-page.test.tsx`. Người thực thi sẽ phát hiện điều này ở cuối Phase 2, sau khi
  Phase 1 đã chốt markup.
- **Evidence:**
  - `apps/web/src/features/teaching/__tests__/classbook-page.test.tsx:444-446`
    `expect(within(picker).getByRole("option", { name: /Toán 6B/ })).toHaveTextContent("Toán 6B · Tối Thứ Ba")`
  - Markup nguồn `apps/web/src/features/teaching/components/records-class-select.tsx:147-148`
    `<span className="flex-1">{klass.name}</span>` rồi
    `<span className="text-[12px] font-bold text-ink-400">{klass.student_count} HS</span>`
    — không có `·`, không có whitespace giữa hai span.
  - Phase 2 Risk chỉ xét dòng 451 (trigger) và kết luận "vẫn chứa 'Toán 6B', pass";
    dòng 444 không được nhắc tới.
- **Suggested fix:** Chốt dứt khoát định dạng option (`label` + separator + `meta`),
  thừa nhận đây là thay đổi thị giác so với `/records`, và đưa `classbook-page.test.tsx`
  vào danh sách file phải sửa.

## Finding 5: Số dòng locator bị đếm thiếu và một symbol e2e không tồn tại

- **Severity:** High
- **Location:** Phase 2, "Related Code Files" và Implementation Step 4
- **Flaw:** Plan liệt kê "2 unit test + 3 e2e" kèm dòng cụ thể, ngụ ý mỗi file sửa một
  dòng. Thực tế nhiều hơn hẳn, và một symbol được viện dẫn không tồn tại trong repo.
- **Failure scenario:** Người thực thi sửa đúng các dòng plan liệt kê, chạy test, rồi
  gặp một loạt `Unable to find role="button"` ở những dòng plan không nhắc tới. Ước
  lượng 4h của Phase 2 dựa trên con số sai.

| File | Plan nói | Thực tế |
|---|---|---|
| `records-toolbar.test.tsx` | dòng 43 | dòng 43 và 96 |
| `records-pages.test.tsx` | dòng 329 | dòng 302, 321, 329, 340, 365 |
| `records-search.spec.ts` | dòng 28, 32 | dòng 28, 32, và regex dòng 33 |
| `class-staff-read.spec.ts` | `classIdFromRecordsPicker` | symbol không tồn tại; code thật ở dòng 39 trong `assertStaffReadJourney` |

- **Evidence:**
  - `grep -n '\^Lớp' apps/web/src/features/teaching/__tests__/records-pages.test.tsx`
    → 302, 321, 329, 340, 365.
  - `apps/web/src/features/teaching/__tests__/records-toolbar.test.tsx:43` và `:96`.
  - `apps/web/e2e/records-search.spec.ts:28`, `:32`, `:33`.
  - `grep -rn "classIdFromRecordsPicker" apps/web` → không có kết quả;
    `apps/web/e2e/class-staff-read.spec.ts:39` `await page.getByRole("button", { name: /^Lớp/ }).click();`
- **Suggested fix:** Thay danh sách dòng cứng bằng lệnh grep tự kiểm
  (`grep -rn '"button", { name: /\^Lớp/'`) và sửa lại ước lượng Phase 2.

## Finding 6: Cơ chế thoát hiểm cho dialog-lồng-dialog không tồn tại trong `HvModal`

- **Severity:** High
- **Location:** Phase 3 Risk Assessment ("Sheet trong dialog"); `plan.md` Risks
- **Flaw:** Phản ứng dự phòng khi Esc đóng cả form là
  `onEscapeKeyDown={(e) => e.stopPropagation()}` "ở sheet". `HvModalProps` không có prop
  đó và component không spread props lạ xuống `Dialog.Content`. Đồng thời `plan.md`
  liệt kê `HvModal` trong mục "Không đụng".
- **Failure scenario:** Ở 375px, mở "Ghi nhận thanh toán", mở sheet Hình thức, bấm Esc.
  Nếu sự kiện lan lên, cả form đóng và mất dữ liệu đang nhập. Lúc đó mitigation trong
  plan không chạy được: phải sửa `HvModal`, tức phá ràng buộc "không đụng" và chạm tới
  mọi modal khác — một quyết định chưa được cân nhắc ở đâu trong plan.
- **Evidence:**
  - `apps/web/src/components/hv/hv-modal.tsx:114-139` — `HvModalProps` chỉ có `open`,
    `onOpenChange`, `title`, `description`, `children`, `footer`, `size`, `className`,
    `onOpenAutoFocus`, `onCloseAutoFocus`.
  - `:141-152` — destructure đúng bấy nhiêu prop, không có rest spread xuống
    `HvModalContent`.
  - `plan.md` → "Không đụng: … `HvModal`, `useMediaQuery`, backend."
- **Suggested fix:** Đưa "mở rộng `HvModalProps` với `onEscapeKeyDown`" thành một bước
  tường minh, additive của Phase 1, hoặc gỡ `HvModal` khỏi danh sách không đụng.

## Finding 7: Sheet lồng trong form modal tạo dialog thứ hai — chưa có tiền lệ trong repo

- **Severity:** High
- **Location:** `plan.md` Risks ("Dialog lồng dialog") và D9
- **Flaw:** Plan coi nested Dialog là rủi ro đã kiểm soát ("Radix hỗ trợ; Esc đóng sheet
  trước"). Nhưng hôm nay **không có** chỗ nào trong `apps/web` mount `HvModal` bên trong
  `HvModal`, nên giả định này chưa từng được chứng minh trên stack này. Hệ quả cụ thể
  hơn: `HvModal` luôn tự render overlay và nút `"Đóng hộp thoại"`, nên khi sheet mở sẽ
  có 2 dialog và 2 nút cùng tên.
- **Failure scenario:** Mọi test dùng `screen.findByRole("dialog")` không định danh trong
  khi sheet đang mở sẽ ném "found multiple elements". Ở viewport mặc định của jsdom
  (`matches: false` → nhánh sheet) đây là đường đi mặc định, không phải trường hợp biên.
  D9 xử lý bằng `mockViewport(1024)` nhưng plan chỉ áp cho `center-permissions`,
  `audit-page`, `enroll-student-dialog` — bỏ sót `center-page.test.tsx` (Finding 1) và
  chỉ nói "chỉ nếu đỏ" cho `record-payment-dialog.test.tsx`.
- **Evidence:**
  - `grep -rn "HvModal" apps/web/src` → không có trường hợp `HvModal` lồng `HvModal`.
  - `apps/web/src/components/hv/hv-modal.tsx:64` overlay luôn render; `:82-95` nút
    `"Đóng hộp thoại"` luôn render khi `showCloseButton` mặc định.
  - Query dialog trần: `center-page.test.tsx:235,279,310`;
    `center-permissions.test.tsx:292,333`; `roster.spec.ts:83,102`.
  - Shim mặc định `matches: false` tại `apps/web/src/test/setup.ts:32-44`, và nó được
    đặt ở module scope (không phải `beforeEach`), nên sau lần `mockViewport` đầu tiên
    trong một file, "mặc định false" không còn đúng cho các test sau trong file đó.
- **Suggested fix:** Trước Phase 3, dựng một spike nhỏ `HvModal` lồng `HvModal` để xác
  nhận Esc, focus và overlay; hoặc chốt luôn phương án `inDialog` dùng popover ở mọi
  breakpoint khi nằm trong dialog.

## Finding 8: Ràng buộc "test chỉ đổi locator và cách chọn" đã bị vi phạm ngay trong Phase 4

- **Severity:** Medium
- **Location:** `plan.md` Constraints (5), đối chiếu Phase 4
- **Flaw:** Có ít nhất hai assertion trạng thái phải viết lại, không phải đổi locator.
- **Failure scenario:**
  - `expect(roleSelect).toHaveValue("")` — jest-dom `toHaveValue` chỉ chấp nhận form
    element và ném lỗi trên `<button role="combobox">`.
  - `within(select).queryByRole("option", …)` dựa vào việc option là con của `<select>`.
    Với popover portal, `within(select)` luôn rỗng, nên assertion "không có Cô Lan trong
    danh sách" trở thành **luôn đúng một cách giả tạo** nếu chỉ đổi locator mà không mở
    dropdown trước. Đây đúng nghĩa là một phantom test.
- **Evidence:**
  - `apps/web/src/features/center/__tests__/center-permissions.test.tsx:295`
    `expect(roleSelect).toHaveValue("");`
  - `apps/web/src/features/roster/__tests__/class-settings-handoff.test.tsx:72-73`
    `expect(within(select).queryByRole("option", { name: /Cô Lan/ })).not.toBeInTheDocument();`
    và `expect(within(select).getByRole("option", { name: /Thầy Nam/ })).toBeInTheDocument();`
  - `plan.md` Constraint (5): "Test hiện có chỉ được sửa ở **locator/role** và cách chọn
    (`selectOptions` → click); assertion nghiệp vụ giữ nguyên."
- **Suggested fix:** Nới constraint (5) thành "assertion nghiệp vụ giữ nguyên ý nghĩa,
  được phép viết lại cách quan sát", và ghi rõ test handoff phải mở dropdown trước khi
  assert vắng mặt.

## Finding 9: Bề mặt prop thiếu tên cho listbox; nhãn hardcode "Chọn lớp" sẽ rò sang mọi consumer

- **Severity:** Medium
- **Location:** Phase 1, "Architecture" (`HvSelectProps`) và bước 3 (port `ClassOptionList`)
- **Flaw:** `ClassOptionList` hardcode `aria-label="Chọn lớp"` cho `div[role=listbox]`.
  `HvSelectProps` không có prop nào ánh xạ tên này (`sheetTitle` chỉ là tiêu đề dialog
  dưới `sm`). Bước port không nói xử lý ra sao.
- **Failure scenario:** Mọi dropdown sau khi migrate đều xướng "Chọn lớp" — kể cả Vai
  trò, Hình thức, Nhóm hành động. Case port từ `records-class-select.test.tsx` vẫn assert
  đúng tên đó nên test hv kit xanh, trong khi các màn khác sai tên; lỗi chỉ lộ ra ở bước
  QA thủ công của Phase 5.
- **Evidence:**
  - `apps/web/src/features/teaching/components/records-class-select.tsx:118-121`
    `role="listbox" aria-label="Chọn lớp" id={listboxId}`.
  - `apps/web/src/features/teaching/__tests__/records-class-select.test.tsx:63` và `:171`
    `getByRole("listbox", { name: "Chọn lớp" })`.
  - `HvSelectProps` trong `phase-01-hv-select-primitive.md` không có trường nào cho tên
    listbox.
- **Suggested fix:** Ghi rõ listbox lấy tên từ `sheetTitle` (hoặc thêm `listboxLabel`),
  và điều chỉnh case port tương ứng.

## Finding 10: Đổi option rỗng thành placeholder làm mất đường reset của màn bàn giao

- **Severity:** Medium
- **Location:** Phase 4, "Requirements → Bàn giao" (và Nhân sự lớp)
- **Flaw:** Plan thay `<option value="">— Chọn giáo viên —</option>` bằng `placeholder`.
  Placeholder không phải một option chọn được, nên trạng thái "chưa chọn ai" trở thành
  một chiều.
- **Failure scenario:** Người dùng chọn nhầm giáo viên và muốn quay về trạng thái trung
  tính để nút "Bàn giao lớp" tắt lại. Hôm nay họ chọn lại dòng "— Chọn giáo viên —";
  sau migrate thì không còn cách nào ngoài rời trang. Phase 4 chỉ phân tích mất mát này
  cho select Vai trò (nơi một chiều là chủ ý), không cho bàn giao (nơi nó không phải).
- **Evidence:**
  - `apps/web/src/features/roster/pages/class-settings-page.tsx:356`
    `<option value="">— Chọn giáo viên —</option>`
  - `:390` `disabled={!targetId || reassign.isPending}` — giá trị rỗng là một trạng thái
    có nghĩa, không phải chỗ trống trang trí.
  - Cùng vấn đề ở `apps/web/src/features/roster/components/class-staff-section.tsx:249`
    `<option value="">— Chọn thành viên —</option>`.
- **Suggested fix:** Cho `HvSelect` một prop `clearable` (hoặc cho phép truyền option
  `value=""`) và dùng nó ở bàn giao và nhân sự lớp.

## Claim đã kiểm chứng là ĐÚNG

Ghi lại để tránh raise lại ở vòng review sau:

- Hotkey `/` **đúng** như plan mô tả: `apps/web/src/features/teaching/pages/records-page.tsx:35`
  `element.closest('[role="listbox"],[role="dialog"]') !== null`.
- `audit-page.test.tsx` **đã** dùng `screen` + `combobox`/`option`: `:92-93`.
- `enroll-student-dialog.test.tsx` **đã** dùng `screen` + `combobox`/`option`: `:42-43`.
- `data-placeholder:` **có** tiền lệ chạy được: Tailwind v4 trong `apps/web/package.json`,
  và class `data-placeholder:text-ink-400` đang dùng ở `apps/web/src/components/ui/select.tsx:38`.
- Ngưỡng ô lọc `> 5` **đúng**: `apps/web/src/features/roster/hooks/use-class-search.ts:20`.
- `useClassSearch` / `ClassSearchInput` chỉ còn `attendance/pages/sessions-page.tsx` và
  `roster/pages/students-page.tsx` dùng — cả hai đều là non-goal, nên **không** consumer
  nào bị ảnh hưởng.
- Số dòng consumer native **đúng hết**: `member-permissions-dialog.tsx:152` và `:209`,
  `class-staff-section.tsx:241`, `class-settings-page.tsx:344`.
- `aria-label` audit ("Giáo viên", "Nhóm hành động") và `w-[180px]` **đúng**:
  `audit-filters.tsx:74`, `:95`. Option `custom` disabled **đúng**: `:101-103`.
- `id="payment-method"` + `FieldLabel htmlFor` + `aria-invalid` **đúng**:
  `record-payment-dialog.tsx:228`, `:236`, `:238`.
- `roster.spec.ts` **đã** dùng `combobox "Lớp"` + `option`: `:81-82`, `:100-101`; và
  spec này chạy ở viewport mặc định 1280 (`playwright.config.ts` không set viewport),
  nên nhánh popover — giả định "verify unchanged" của Phase 3 hợp lý.
- `records-search.spec.ts` chạy ở 375px qua `test.use({ viewport: … })` dòng 9, nên
  assertion sheet của spec đó vẫn đúng sau migrate.

## Unresolved questions

1. `updatePositionStrategy` trên `Popover.Content` (mitigation của Phase 4) **không kiểm
   chứng được**: hook `scout-block` chặn đọc `node_modules`, và prop này không xuất hiện
   ở đâu trong `apps/web/src`. Cần xác nhận trước khi coi nó là phương án dự phòng.
2. Tên truy cập của `<button role="combobox">` lấy từ `<label htmlFor>` (D3) chạy được
   trong jsdom (dom-accessibility-api coi `button` là labelable), nhưng chưa được xác
   minh trên Chromium của Playwright cho `roster.spec.ts:81` (`combobox` tên "Lớp") và
   `class-staff-write.spec.ts:155` (`getByLabel("Bàn giao cho")`). Nên kiểm tra sớm ở
   Phase 3/4 thay vì để đến Phase 5.
