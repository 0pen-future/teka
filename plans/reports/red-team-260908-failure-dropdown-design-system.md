---
title: "Red team — Failure Mode Analyst: dropdown-design-system"
role: FLOW TRACER
plan: plans/260908-0832-dropdown-design-system/
date: 2026-09-08
---

# Red team (failure modes) — dropdown design system

9 findings. Mọi finding đều kèm trích dẫn file:line từ repo.

## Finding 1: Phase 3 xoá `ui/select.tsx` trong khi Phase 2 (được tuyên bố chạy song song) vẫn import nó

- **Severity:** Critical
- **Location:** plan.md, bảng *Phases* + phase-03, "Implementation Steps" bước 4 và front-matter `dependencies: [1]`
- **Flaw:** Đồ thị phụ thuộc sai. `ui/select.tsx` có 4 consumer, trong đó **một** thuộc Phase 2 chứ không phải Phase 3.
- **Failure scenario:** Hai worktree song song. Worktree Phase 3 chạy `grep -rn "components/ui/select" apps/web/src` — vẫn thấy `class-select.tsx` (chưa migrate ở nhánh này) nên gate bước 4 chặn, Phase 3 không hoàn thành được nếu không rebase lên Phase 2. Nếu người thực hiện bỏ qua grep và `git rm` luôn, merge Phase 2 + Phase 3 tạo bản dựng đỏ mà **không phase nào bắt được**, vì mỗi phase chỉ chạy `vitest` trên thư mục của mình (phase-02 bước 5 chạy `src/features/teaching`, phase-03 bước 5 chạy `audit/collections/roster`). Rollback cũng thủng: plan.md mục *Rollback* nói "Phase 3/4 độc lập nên revert từng commit" — revert riêng Phase 2 sau khi Phase 3 đã landed sẽ khôi phục `class-select.tsx` import một file đã bị xoá.
- **Evidence:**
  - `apps/web/src/features/teaching/components/class-select.tsx:1-7` — `import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";`
  - 4 consumer thực tế: `record-payment-dialog.tsx:14`, `audit-filters.tsx:11`, `enroll-student-dialog.tsx:12`, `class-select.tsx:7`
  - plan.md: "Phase 2, 3, 4 độc lập về file sau Phase 1 … → có thể chạy song song"
  - phase-03: `dependencies: [1]`; bước 4: "`grep -rn "components/ui/select" apps/web/src` = 0 → `git rm apps/web/src/components/ui/select.tsx`"
- **Suggested fix:** Đặt `dependencies: [1, 2]` cho Phase 3, hoặc tách việc xoá `ui/select.tsx` thành bước đầu của Phase 5 (sau khi 2/3/4 đã gộp). Ghi rõ trong *Rollback* rằng revert Phase 2 buộc phải revert cả commit xoá file.

## Finding 2: `classbook-page.test.tsx` sẽ đỏ ở dòng 444, trái với tuyên bố "không sửa"

- **Severity:** Critical
- **Location:** phase-02, "Requirements → Non-functional" và "Success Criteria" (`classbook-page.test.tsx` không sửa); phase-02 "Risk Assessment" chỉ phân tích dòng 451
- **Flaw:** D4 tách nhãn thành `label` + `meta`, nhưng markup option của chuẩn **không** chèn ` · ` giữa hai span — dấu `·` chỉ tồn tại trên trigger. Test hiện có assert đúng chuỗi có dấu `·` **trong option**.
- **Failure scenario:** Sau Phase 2, option "Toán 6B" có `textContent` là `"Toán 6BTối Thứ Ba"`. `toHaveTextContent("Toán 6B · Tối Thứ Ba")` (chỉ chuẩn hoá khoảng trắng, không chèn ký tự) fail. Phase 2 bước 5 chạy `npx vitest run src/features/teaching` sẽ đỏ, nhưng plan đã ghi trước là "xanh không sửa" → người thực hiện sẽ đi tìm lỗi ở primitive thay vì hiểu đây là hệ quả của thay đổi hợp đồng dữ liệu đã chốt ở D4.
- **Evidence:**
  - `apps/web/src/features/teaching/__tests__/classbook-page.test.tsx:444-446`
    ```js
    expect(within(picker).getByRole("option", { name: /Toán 6B/ })).toHaveTextContent(
      "Toán 6B · Tối Thứ Ba",
    );
    ```
  - `apps/web/src/features/teaching/components/records-class-select.tsx:143-147` — option render `<span className="flex-1">{klass.name}</span>` rồi `<span …>{klass.student_count} HS</span>`, không có `·`
  - So sánh: trigger tại `records-class-select.tsx:215-217` mới có `· {selected.student_count} HS`
  - phase-02: "`classbook-page.test.tsx` xanh **không sửa** (đã dùng combobox/listbox/option)"
- **Suggested fix:** Hoặc liệt kê `classbook-page.test.tsx:444` vào danh sách file sửa (đổi assertion sang `/Toán 6B/` + `/Tối Thứ Ba/`), hoặc chốt lại D4 để option cũng render `·` giữa label và meta. Chọn một, đừng để hai chỗ mâu thuẫn.

## Finding 3: `min-w-[230px]` trong class trigger vô hiệu hoá mọi override chiều rộng của consumer

- **Severity:** High
- **Location:** phase-01, "Architecture" mục 1 ("sao chép nguyên chuỗi class") + "Success Criteria" (so string equality className); phase-03 (`w-[180px]`), phase-04 (`w-[150px] shrink-0`); plan.md D10
- **Flaw:** Chuỗi class chuẩn chứa `min-w-[230px]` **và** `max-sm:w-full`. `cn` là `twMerge`, mà `min-width` và `width` là hai nhóm khác nhau → twMerge giữ cả hai, và theo CSS `min-width` thắng `width`. D10 giả định "chiều rộng qua `className`" hoạt động — không đúng.
- **Failure scenario:** Audit `w-[180px]` render ra 230px, đẩy hàng filter (`Giáo viên`, `Nhóm hành động`, `Hành động`, 2 ô ngày) xuống dòng. Nghiêm trọng hơn ở hộp thoại phân quyền tại 375px: hàng quyền là `flex items-center gap-2`, `HvSelect` mang `w-[150px] shrink-0` nhưng thực tế `min-w-[230px]` + `max-sm:w-full` → không co được, tràn ngang. `HvModal` content chỉ có `overflow-y-auto`, không có `overflow-x` → nội dung bị cắt hoặc sinh scroll ngang. Không gate nào bắt: lint/typecheck/vitest đều mù với layout, và **Success Criteria của Phase 1 chủ động khoá lỗi này lại** bằng test so sánh bằng chuỗi className.
- **Evidence:**
  - `apps/web/src/features/teaching/components/records-class-select.tsx:18-24` — `"inline-flex min-h-11 min-w-[230px] items-center … max-sm:w-full"`
  - `apps/web/src/lib/utils/cn.ts:4-6` — `twMerge(clsx(inputs))`
  - `apps/web/src/components/hv/hv-modal.tsx:66-72` — Content: `"fixed inset-x-0 bottom-0 … max-h-[85vh] w-full … overflow-y-auto … p-6"` (không có `overflow-x`)
  - `apps/web/src/features/center/components/member-permissions-dialog.tsx:186` — hàng quyền `<div className="flex items-center gap-2 py-2">`
  - `apps/web/src/features/audit/components/audit-filters.tsx:74,95` — `className="w-[180px]"`
  - phase-01: "**sao chép nguyên chuỗi class** từ `records-class-select.tsx`"; Success Criteria: "snapshot class trigger/option/content **bằng** chuỗi trong `records-class-select.tsx`"
- **Suggested fix:** Tách `min-w-[230px] max-sm:w-full` ra khỏi `triggerClassName` cơ sở, để consumer tự truyền (records/staff truyền, audit và hàng quyền không). Sửa Success Criteria Phase 1 để so chuỗi *sau khi trừ* nhóm width, nếu không chính test sẽ bảo vệ bug này.

## Finding 4: `secretary-send.spec.ts:38` gọi `inputValue()` — Playwright ném lỗi trên phần tử không phải input

- **Severity:** High
- **Location:** phase-04, "Related Code Files" — `e2e/secretary-send.spec.ts` (dòng 34–41 `selectOption(target)` → click + option theo nhãn)
- **Flaw:** Plan chỉ mô tả việc thay `selectOption`. Dòng 38 là một **phép đọc giá trị**, không phải phép chọn, và không có tương đương click. `locator.inputValue()` reject với "Node is not an `<input>`, `<textarea>` or `<select>` element" khi target là `button[role=combobox]`.
- **Failure scenario:** Helper `setSendReportsGrant` là cơ chế assert-then-set để spec chạy được trên database e2e dùng lại giữa các lần chạy (comment ngay trên helper nói rõ điều đó). Sau Phase 4, dòng 38 ném lỗi → spec đỏ ngay lần chạy đầu, và `revokeGrant` trong `afterEach` cũng đi qua cùng helper → **grant `reports.send` bị bỏ lại ở trạng thái bật**, làm hỏng các spec sau và cả lần chạy kế tiếp trên cùng database. Đây là lỗi có tác dụng phụ lên state dùng chung, không chỉ một test đỏ.
- **Evidence:**
  - `apps/web/e2e/secretary-send.spec.ts:34` `const reportsSend = dialog.getByRole("combobox", { name: "Quyền Gửi báo cáo học phí" });`
  - `apps/web/e2e/secretary-send.spec.ts:38` `if ((await reportsSend.inputValue()) === target) {`
  - `apps/web/e2e/secretary-send.spec.ts:41` `await reportsSend.selectOption(target);`
  - Comment trên helper: "The e2e database is reused between runs, so the grant must be assert-then-set"
- **Suggested fix:** Ghi rõ trong Phase 4 rằng dòng 38 phải đổi sang đọc **nhãn** trên trigger (`textContent()` so với `"Cấp riêng"` / `"Theo vai trò"`), kèm bảng ánh xạ `grant → "Cấp riêng"`, `inherit → "Theo vai trò"`, `deny → "Chặn riêng"`. Ánh xạ này không suy ra được từ mô tả hiện tại của plan.

## Finding 5: `class-staff-write.spec.ts:195` truyền nhãn `"Cô Lan (chủ trung tâm)"` mà Phase 4 cố ý phá bỏ

- **Severity:** High
- **Location:** phase-04, "Requirements → Bàn giao" (`meta: member.is_owner ? "chủ trung tâm" : undefined` — thay hậu tố trong ngoặc) và "Related Code Files" (chỉ nêu dòng 155)
- **Flaw:** Plan chỉ đăng ký dòng 155 (chỗ gọi `selectOption`). Nhưng nhãn option được truyền vào từ **dòng 195**, và chính Phase 4 đổi định dạng nhãn đó. Ngoài ra plan không nói gì về việc option bị portal ra khỏi `#teacher-handoff`.
- **Failure scenario:** Sau Phase 4, tên truy cập của option là `"Cô Lan chủ trung tâm"` (label span + meta span, không dấu ngoặc). Lời gọi `ensureClassTeacher(owner, classId, OWNER.name, "Cô Lan (chủ trung tâm)")` không khớp option nào → timeout 30s. Thêm một tầng nữa: helper scope vào `const card = page.locator("#teacher-handoff")`; ở nhánh popover (Playwright mặc định 1280px vì `playwright.config.ts` không khai báo `projects`/`viewport`), `Popover.Portal` gắn content vào `document.body` nên `card.getByRole("option", …)` **không bao giờ** thấy option. Phase 4 bước 5 xử lý đúng chuyện portal cho `secretary-send` ("option nằm trong portal ngoài dialog") nhưng bỏ sót ở đây. Vì `ensureClassTeacher` cũng chạy trong `test.afterEach` để trả lớp về Thầy Minh, hỏng ở đây làm **lệch dữ liệu seed cho các lần chạy sau**.
- **Evidence:**
  - `apps/web/e2e/class-staff-write.spec.ts:195` `await ensureClassTeacher(owner, classId, OWNER.name, \`${OWNER.name} (chủ trung tâm)\`);`
  - `apps/web/e2e/class-staff-write.spec.ts:150,155` — `const card = page.locator("#teacher-handoff");` / `await card.getByLabel("Bàn giao cho").selectOption({ label: targetOptionLabel });`
  - `apps/web/e2e/class-staff-write.spec.ts:166-176` — `test.afterEach` gọi `ensureClassTeacher`
  - `apps/web/src/features/roster/pages/class-settings-page.tsx:357-359` — `{member.full_name}{member.is_owner ? " (chủ trung tâm)" : ""}`
  - `apps/web/playwright.config.ts` — không có `projects`, không set `viewport`, nên mặc định 1280×720 và luôn vào nhánh popover
- **Suggested fix:** Liệt kê dòng 195 vào danh sách sửa với nhãn mới, và ghi rõ option phải lấy ở scope `page`, không phải `card`. Cân nhắc giữ `(chủ trung tâm)` trong `label` thay vì `meta` để không đụng e2e.

## Finding 6: `center-permissions.test.tsx:295` dùng `toHaveValue("")` — vi phạm chính ràng buộc (5) của plan

- **Severity:** High
- **Location:** plan.md "Constraints" mục (5) ("Test hiện có chỉ được sửa ở **locator/role** và cách chọn … assertion nghiệp vụ giữ nguyên"); phase-04, "Related Code Files" (`center-permissions.test.tsx` dòng 294–296)
- **Flaw:** Trong khoảng 294–296 có một assertion, không chỉ locator và cách chọn. `toHaveValue` của jest-dom chỉ hỗ trợ `input`, `select`, `textarea`; gọi trên `button[role=combobox]` là ném lỗi, không phải fail mềm. Ràng buộc (5) khiến người thực hiện tưởng chỉ cần đổi locator.
- **Failure scenario:** Sửa 294 và 296 theo plan, giữ 295 → test ném "only inputs, selects and textareas". Người thực hiện đọc ràng buộc (5) sẽ do dự vì việc đổi assertion đó bị plan cấm ngầm. Ngữ nghĩa cũng đổi thật: hiện tại `value=""` ứng với một `<option value="" disabled>` hiện hữu, sau migration là placeholder — assertion mới phải là "trigger hiện `Giáo viên (mặc định)`", tức một khẳng định khác về hành vi, không phải đổi locator.
- **Evidence:**
  - `apps/web/src/features/center/__tests__/center-permissions.test.tsx:294-296`
    ```js
    const roleSelect = await within(dialog).findByRole("combobox", { name: "Vai trò" });
    expect(roleSelect).toHaveValue("");
    await user.selectOptions(roleSelect, HOC_VU.id);
    ```
  - `apps/web/src/features/center/components/member-permissions-dialog.tsx:157-162` — `<option value="" disabled>Giáo viên (mặc định)</option>`
  - phase-04 Risk có nhắc rủi ro option disabled → placeholder, nhưng không nhắc `toHaveValue`
- **Suggested fix:** Nới ràng buộc (5) thành "assertion về **kết quả nghiệp vụ** giữ nguyên; assertion về cơ chế điều khiển (`toHaveValue`, `inputValue`, `selectOptions`) được phép đổi", và liệt kê rõ dòng 295. Cùng lúc kiểm tra `screen.findByRole("dialog")` ở 292 và 333: nếu quên `mockViewport(1024)` theo D9, sheet của HvSelect là dialog thứ hai và `findByRole` ném "multiple elements", một thông báo lỗi không dẫn về nguyên nhân thật.

## Finding 7: Phương án dự phòng cho Esc trong sheet lồng nhau vừa không thực thi được vừa sai kỹ thuật

- **Severity:** High
- **Location:** plan.md "Risks" mục *Dialog lồng dialog*; phase-03 "Risk Assessment" mục *Sheet trong dialog*
- **Flaw:** Hai chỗ đều kê đơn `onEscapeKeyDown={(e) => e.stopPropagation()}` "ở sheet". Nhưng (a) `HvModalProps` **không có** prop `onEscapeKeyDown` để truyền xuống, và (b) plan.md mục *Files → Không đụng* liệt kê `HvModal`. Khi rủi ro này xảy ra, phản ứng đã định không thực hiện được nếu không phá một ràng buộc do chính plan đặt ra.
- **Failure scenario:** Ở 375px, mở "Ghi nhận thanh toán" (HvModal) rồi mở sheet "Hình thức" (HvModal thứ hai). Nếu Esc đóng cả hai, người thực hiện phải mở `hv-modal.tsx` để thêm prop — đây là primitive dùng chung của toàn bộ hv kit, sửa nó nằm ngoài phạm vi đã duyệt và không có gate nào của Phase 3 phủ các consumer HvModal khác. Về kỹ thuật, `stopPropagation` cũng sai: Radix `DismissableLayer` đăng ký Esc bằng listener capture trên `document`, nên `stopPropagation` không chặn được listener khác trên **cùng một node** (cần `stopImmediatePropagation`), và Radix vốn đã gate việc dismiss theo "layer cao nhất" nên fix này vừa thừa vừa không hiệu lực.
- **Evidence:**
  - `apps/web/src/components/hv/hv-modal.tsx:118-145` — `HvModalProps` chỉ có `open`, `onOpenChange`, `title`, `description`, `children`, `footer`, `size`, `className`, `onOpenAutoFocus`, `onCloseAutoFocus`
  - `apps/web/src/components/hv/hv-modal.tsx:147-160` — `HvModal` không nhận và không forward `onEscapeKeyDown`
  - plan.md "Files → **Không đụng**: … `HvModal`, `useMediaQuery`, backend"
  - plan.md Risks: "Tín hiệu vỡ: Esc đóng cả form → dùng `onEscapeKeyDown` chặn lan ở sheet"
- **Suggested fix:** Kiểm chứng hành vi Esc lồng nhau **trong Phase 1** bằng một test hv-select (sheet mở từ trong một HvModal), rồi ghi kết quả vào plan. Nếu cần fix thật, đưa việc thêm `onEscapeKeyDown` vào `HvModal` thành một bước tường minh của Phase 1 và gỡ `HvModal` khỏi danh sách "Không đụng".

## Finding 8: Blast radius locator của Phase 2 bị đếm thiếu 5 lần

- **Severity:** Medium
- **Location:** phase-02, "Related Code Files" (`records-toolbar.test.tsx` dòng 43; `records-pages.test.tsx` dòng 329) và "Requirements → Non-functional" ("test `records-pages.test.tsx` chỉ sửa role ở 1 dòng")
- **Flaw:** Số liệu sai. `records-pages.test.tsx` có 5 locator `button` cho picker, `records-toolbar.test.tsx` có 2.
- **Failure scenario:** Người thực hiện sửa đúng 2 dòng plan liệt kê, chạy `npx vitest run src/features/teaching`, thấy 5 test đỏ với thông báo `Unable to find an accessible element with the role "button"`. Vì plan khẳng định chỉ 1 dòng cần đổi, nghi ngờ đầu tiên hướng vào `HvSelect` (role không được set? trigger không render?) chứ không vào locator — mất thời gian debug sai hướng, đúng lúc Phase 2/3/4 chạy song song nên khó phân biệt nguồn lỗi.
- **Evidence:**
  - `apps/web/src/features/teaching/__tests__/records-pages.test.tsx:302, 321, 329, 340, 365` — `getByRole("button", { name: /^Lớp/ })`
  - `apps/web/src/features/teaching/__tests__/records-toolbar.test.tsx:43, 96`
  - phase-02: "`records-toolbar.test.tsx` (dòng 43: `button` → `combobox`), `records-pages.test.tsx` (dòng 329)"
- **Suggested fix:** Thay danh sách dòng cứng bằng một lệnh grep kiểm chứng được trên `apps/web/src/features/teaching/__tests__`, yêu cầu đổi hết. Lưu ý dòng 365 nằm trong test `mockViewport(375)` → nhánh sheet, phải giữ nguyên assertion `getByRole("dialog", { name: "Chọn lớp" })`.

## Finding 9: hv kit sẽ phải import ngược vào feature (`ClassSearchEmptyNote`), hoặc lặng lẽ đánh mất parity

- **Severity:** Medium
- **Location:** phase-01, "Implementation Steps" bước 2 (chỉ port `useClassSearch` → `useOptionSearch`) và "Architecture" mục 2
- **Flaw:** Chuẩn hiện tại render note "không khớp" bằng `ClassSearchEmptyNote` **nhập từ `@/features/roster`**. Phase 1 nói port cái hook, không nói gì về component note. Plan cũng liệt kê `roster/components/class-search.tsx` vào mục "Không đụng".
- **Failure scenario:** Hai ngã, cả hai đều xấu. (a) `components/hv/hv-select.tsx` `import { ClassSearchEmptyNote } from "@/features/roster"` — primitive dùng chung phụ thuộc một feature, đảo ngược đúng quy tắc layering mà plan viện dẫn từ `docs/frontend-guidelines.md` để biện minh cho D1; kéo theo cả barrel `features/roster/index.ts` vào mọi trang có dropdown, kể cả trang audit không liên quan roster. (b) Viết lại markup note trong hv-select — khi đó Success Criteria Phase 1 ("class **bằng** chuỗi trong `records-class-select.tsx`") không kiểm được khối này, và note ở `/records` có thể lệch so với note ở Điểm danh / Lớp & học sinh vẫn dùng component cũ. Plan không chọn ngã nào, nên quyết định rơi vào lúc code, khi đã muộn.
- **Evidence:**
  - `apps/web/src/features/teaching/components/records-class-select.tsx:6` — `import { ClassSearchEmptyNote, useClassSearch, type Class } from "@/features/roster";`
  - `apps/web/src/features/teaching/components/records-class-select.tsx:159-163` — `<ClassSearchEmptyNote note={emptyNote} />`
  - `apps/web/src/features/roster/index.ts:6` — `export { ClassSearchEmptyNote, ClassSearchInput } from "./components/class-search";`
  - Consumer còn lại vẫn dùng bản cũ: `apps/web/src/features/attendance/pages/sessions-page.tsx:166`, `apps/web/src/features/roster/pages/students-page.tsx:322`
  - plan.md "Không đụng: … `roster/components/class-search.tsx`"
- **Suggested fix:** Chốt trong Phase 1: hoặc chuyển `ClassSearchEmptyNote` thành primitive hv (và cập nhật 2 consumer roster/attendance — khi đó nó không còn nằm ở "Không đụng"), hoặc nội tuyến markup note trong `hv-select.tsx` và ghi rõ đây là bản sao có chủ ý, kèm test so chuỗi class riêng cho khối note.

## Ghi chú trace không thành finding

- **Popover portal trong Dialog modal:** Radix `DismissableLayer` xếp lớp theo thứ tự mount và bắt `pointerdown` qua capture trên React tree (portal vẫn là con React của dialog), nên click option không bị hiểu là "outside". `aria-hidden` của Dialog chỉ đánh dấu các node **đã tồn tại** lúc dialog mở, còn popover mount sau nên không bị ẩn. Bằng chứng thực nghiệm: `record-payment-dialog.tsx:229-249` và `enroll-student-dialog.tsx` đang chạy Radix Select (cùng cơ chế portal) trong `HvModal` và test đang xanh. Giả định của plan ở đây hợp lý.
- **Hotkey `/` ở `/records`:** `isTypingTarget` bail theo `element.closest('[role="listbox"],[role="dialog"]')`, mà markup mới vẫn giữ `div[role=listbox]` và sheet vẫn là `role=dialog`, nên hành vi không đổi. Xem `apps/web/src/features/teaching/pages/records-page.tsx:28-36`.
- **Rò rỉ state của `mockViewport`:** `viewportWidth` là biến module-level không được reset giữa các test trong cùng file (`apps/web/src/test/viewport.ts:22`). Các file hiện tại đều đặt `mockViewport` trong `beforeEach` (`records-pages.test.tsx:59`, `records-toolbar.test.tsx:39`) nên an toàn; D9 nên nói rõ **phải** dùng `beforeEach` chứ không gọi rải rác, nếu không một test gọi `mockViewport(375)` giữa chừng sẽ đổi nhánh cho mọi test sau nó.
