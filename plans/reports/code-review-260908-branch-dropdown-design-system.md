# Code review — toàn nhánh `feat/dropdown-design-system`

- Plan: `plans/260908-0832-dropdown-design-system/` (Phase 5, bước 7)
- Nhánh: `feat/dropdown-design-system` (5 commit `ba5c9df` → `063c449`) + sửa chưa commit ở `docs/frontend-guidelines.md`
- Diff: 30 file, +1318 / −945
- Reviewer: `reviewer-branch` (code-reviewer), 2026-09-08

## Verdict

**DONE_WITH_CONCERNS.** Cuộc di trú đúng như plan: 10/10 vị trí chạy `HvSelect`,
`components/ui/select.tsx` đã xoá, không còn `<select>` native, không có import
`@/features` trong `components/hv`, 4 guard D7 đều có thật, 5 gate của `apps/web`
xanh khi tôi chạy lại. Không có finding **High** hay **Critical**: nhánh không
chạm trust boundary, không đổi API, không đổi schema, không thêm dependency.

Một finding **Medium** duy nhất là chất lượng test: test D7 cho picker override
quyền là *phantom* — assertion của nó đúng kể cả khi gỡ guard.

## Gate tôi chạy lại (tại HEAD + docs chưa commit)

| Gate | Lệnh | Kết quả |
|---|---|---|
| Typecheck | `npm run typecheck` | ✅ sạch |
| Lint | `npm run lint` | ✅ 0 error, 5 warning `react-hooks/incompatible-library` có sẵn ở `student-dialog.tsx` / `class-settings-page.tsx`, không liên quan |
| Unit | `npx vitest run` | ✅ 85 file, 660 passed, 3 skipped |
| Format | `npm run format:check` | ✅ sạch. Prettier chạy từ `apps/web` nên `docs/frontend-guidelines.md` nằm ngoài scope — đúng như lead ghi |
| Build | `npm run build` | ✅ built in 882ms |

E2e không chạy lại trong session này; lấy theo bằng chứng của
`plans/reports/qa-260908-1027-dropdown-design-system.md`: 8/8 + rerun 4/4 trên
stack cách ly `teka-e2e`.

## Findings

| # | Sev | Vị trí | Vấn đề | Đề xuất |
|---|-----|--------|--------|---------|
| M1 | Medium | `apps/web/src/features/center/__tests__/center-permissions.test.tsx:341` | Test *"keeps the form clean when re-picking a permission's current override mode (D7)"* không phân biệt được có guard hay không. `dirty` ở `member-permissions-dialog.tsx:82-91` là `modes !== null && data.catalog.some(...)`; gỡ guard ở `:222-224` thì `setModes({ ...draft })` chỉ đổi `modes` từ `null` sang một object **có giá trị y hệt server**, `.some()` vẫn `false`, nút "Lưu" vẫn `disabled`. Assertion đúng trong cả hai trường hợp. | Guard đó **có** tác dụng thật nhưng khác: giữ `modes === null` để một lần refetch nền của `useCenterPermissions` còn chảy được vào dialog (`draft = modes ?? initialModes(member)`). Hoặc (a) đổi assertion sang thứ thật sự khác nhau — đẩy một bản read model mới qua msw sau khi re-pick và assert dialog phản ánh nó; hoặc (b) bỏ cả guard lẫn test, vì plan tự nói consumer chỉ set state thuần không cần guard, rồi ghi lý do "giữ draft chưa khởi tạo" vào comment nếu vẫn muốn giữ guard. |
| L1 | Low | `apps/web/src/components/hv/hv-select.tsx:151-152` | `enabledOptions().indexOf(event.target)` trả `-1` khi focus đang nằm trên chính `div[role=listbox]` (đường đi có thật: `focusInitial` ở `:297` rơi về listbox khi danh sách rỗng hoặc bị disabled hết). Lúc đó `ArrowUp` gọi `focusOption(-2)` → nhảy vào option **áp chót** thay vì option cuối. | Chuẩn hoá vị trí xuất phát: `const current = enabledOptions().indexOf(event.target as HTMLButtonElement);` rồi trong nhánh `ArrowUp` dùng `focusOption(current < 0 ? -1 : current - 1)`. |
| L2 | Low | `member-permissions-dialog.tsx:169-179` (Vai trò), `:214-229` (từng quyền), `record-payment-dialog.tsx:224-234` (Hình thức) | Ba picker không truyền `searchNoun`, nên nếu danh sách vượt 5 mục thì ô lọc rơi về mặc định "mục" (`Tìm mục…`, `Không có mục nào khớp "q"`) — đúng thứ tiêu chí Success Criteria của plan cấm. Hôm nay không với tới được: hệ chỉ seed 3 vai trò (`apps/api/migrations/000013_center_rbac.up.sql:53-57`) và không có endpoint tạo vai trò; 3 chế độ override; 3 hình thức thanh toán. | Thêm `searchNoun="vai trò"` / `"chế độ"` / `"hình thức"`, hoặc `searchThreshold={Infinity}` cho ba chỗ này để nói rõ "không bao giờ lọc". Rẻ và đóng hẳn đường lệch DS. |
| L3 | Low | `hv-select.tsx:399-406` + QA report mục 5 | `PopoverPrimitive.Content` mang `role="dialog"` mặc định của Radix (QA đã đo: *"Trường `inSheet` … luôn `true` vì Radix Popover content cũng có `role=dialog`"*). Hệ quả: trigger hứa `aria-haspopup="listbox"` nhưng popup thực tế được đọc là dialog; và mọi `getByRole("dialog")` trần đều nhập nhằng **kể cả ở nhánh desktop**, không chỉ nhánh sheet mà D9 lo. Đây là tính chất có sẵn của chuẩn `/records`, giờ nhân lên 10 chỗ — không phải hồi quy. | Nếu muốn khớp lời hứa ARIA: thử `<PopoverPrimitive.Content role="presentation" …>` (Content spread props nên override được) và chạy lại `hv-select.test.tsx` + e2e. Nếu không đổi, thêm một câu vào `docs/frontend-guidelines.md` mục Testing: query dialog của form phải có `name`, vì popover cũng là `role=dialog`. |
| L4 | Low | `docs/frontend-guidelines.md:55` | "the only dropdown" trong khi `components/ui/dropdown-menu.tsx` vẫn còn (menu hành động của `mode-toggle.tsx`, hiện không mount). Câu này sẽ sai ngay khi ai đó mount lại ModeToggle. | Đổi thành "the only single-value picker" hoặc "mọi dropdown chọn-một-giá-trị", để `DropdownMenu` (menu hành động) không bị gộp vào. |
| L5 | Low | `member-permissions-dialog.tsx:227` | `sheetTitle={permission.label}` → listbox và bottom sheet được đặt tên theo **tên quyền** ("Tạo lớp học") chứ không phải thứ đang chọn (chế độ override). Đúng D12 nhưng đọc lên hơi lạ. | Nếu muốn rõ hơn: `sheetTitle={`Quyền ${permission.label}`}`. Chấp nhận giữ nguyên cũng được — chỉ ghi nhận. |
| L6 | Low | `audit-filters.tsx:55-64`, `enroll-student-dialog.tsx:82-86` | Mảng `options` được dựng lại mỗi lần render, nên `useMemo` lọc ở `hv-select.tsx:89-93` không bao giờ hit. Vô hại ở cỡ danh sách hiện tại (≤20 mục, lọc O(n)). | Không cần sửa. Nếu một picker nào đó lên hàng trăm mục thì bọc `useMemo` ở consumer. |
| L7 | Low | `hv-select.tsx:86-99` vs `features/roster/hooks/use-class-search.ts:18-33` | `useOptionSearch` là bản sao gần như nguyên văn của `useClassSearch` (cùng ngưỡng 5, cùng mẫu note). D5 cố ý chọn nhân đôi để hv kit không phụ thuộc feature, nhưng không có gì ghim hai bên khớp nhau — sửa chữ ở một chỗ là DS lệch âm thầm. | Thêm một dòng comment chéo ở cả hai hàm nêu rõ "đổi mẫu note thì phải đổi cả hai", hoặc ghi mẫu note vào một hằng số dùng chung ở `lib/`. |
| L8 | Low | `hv-select.test.tsx` | Thiếu case biên `options.length === searchThreshold` (đúng 5 → **không** hiện ô lọc) và thiếu case ô lọc ở nhánh sheet. | Thêm 2 test ngắn; cả hai đều dùng lại `renderSelect` sẵn có. |
| I1 | Info | `.env.production.example`, `docker-compose.prod.yml` | Hai file này bẩn trong working tree từ trước session, không liên quan tới plan dropdown. | Đừng gộp vào commit/PR của nhánh này. |

### Findings cũ không nêu lại

`L1` của review Phase 1 (`aria-controls` phát cả khi đóng → IDREF treo) vẫn **mở**
theo đúng quyết định D2 của người dùng. `L3` (value trùng làm vỡ `key`), `L6`
(đổi breakpoint khi đang mở thì remount surface), `L4` (không có
`aria-describedby`) và `L5`/`L7` của review Phase 2–4 đã được ghi là chấp nhận
hoặc từ chối có căn cứ; tôi không có bằng chứng mới nên giữ nguyên.

## Đối chiếu Success Criteria của `plan.md`

| # | Tiêu chí | Kết quả |
|---|---|---|
| 1 | `grep -rn "components/ui/select\|<select" apps/web/src` = 0; đã xoá `ui/select.tsx` | ✅ 0 hit; `ls components/ui/` không còn `select.tsx` |
| 2 | 10 vị trí render `HvSelect`; combobox → listbox ≥ sm, dialog `sheetTitle` < sm; ảnh khớp chuẩn | ✅ `grep -c "<HvSelect"` trong `features` = 10, `sheetTitle=` = 10; ảnh + token đo được ở QA report 10/10 |
| 3 | Ô lọc chỉ khi > 5 mục, note đúng noun riêng, không rơi về "mục" | ✅ ở 10 vị trí hôm nay (QA: vị trí 4 có 19 mục → `Tìm nhóm hành động…`; các chỗ khác ≤5 nên không hiện). ⚠️ 3 picker chưa khai `searchNoun` — xem L2 |
| 4 | `grep "@/features" components/hv` = 0; listbox tên = `sheetTitle` | ✅ 0 hit; `listboxLabel={sheetTitle}` tại `hv-select.tsx:342` |
| 5 | ↑↓ Home End Enter Space Esc, trả focus, gõ-để-lọc, bỏ qua option disabled | ✅ `hv-select.test.tsx:150-258` phủ đủ; QA bàn phím 3/3 PASS. ⚠️ mép L1 (ArrowUp từ listbox rỗng) |
| 6 | Placeholder `ink-400`; disabled `cream-200`/`ink-300`; `aria-invalid` viền `coral-400`; danh sách dài cuộn theo viewport | ✅ token ở `hv-select.tsx:52-53, 66`; QA đo `max-height` 355–646px thật sự có tác dụng; `aria-invalid`/`aria-disabled` phủ bằng unit test vì seed không dựng được trạng thái đó |
| 7 | Hành vi nghiệp vụ không đổi + 4 guard D7 | ✅ guard tại `class-select.tsx:38`, `member-permissions-dialog.tsx:94` và `:222`, `class-settings-page.tsx:311`. Test thật cho 3/4; guard override chỉ có test phantom — M1 |
| 8 | Trigger bàn giao vẫn `Tên (chủ trung tâm)`; `class-staff-write` + `secretary-send` idempotent | ✅ label tại `class-settings-page.tsx:356`; `ensureClassTeacher` nhận `"${OWNER.name} (chủ trung tâm)"` (`class-staff-write.spec.ts:197`); QA ghi rerun 4/4 |
| 9 | lint / typecheck / test / build xanh; e2e xanh trên stack cách ly | ✅ 4 gate + `format:check` tôi chạy lại đều xanh; e2e 8/8 theo QA report (không tự chạy lại) |
| 10 | `docs/frontend-guidelines.md` liệt kê `HvSelect`, không nhắc `components/ui/select` | ✅ `:55-59` và `:141-143`; `grep -rn "ui/select" docs/` = 0. ⚠️ chữ "the only dropdown" — L4 |

Phase 5 Success Criteria: xoá file ✅, 4 gate ✅, e2e + rerun ✅ (theo QA), báo cáo
QA 10 × 3 ảnh ✅, docs ✅, "không còn finding High/Medium mở" ❌ — còn M1 ở trên.

## Không tự kiểm chứng được trong session này

- E2e trên stack cách ly (không dựng `teka-e2e` trong session review); lấy theo QA report.
- Ảnh QA và computed style (không mở trình duyệt).
- `role="dialog"` của Radix Popover Content: hook chặn đọc `node_modules`, tôi
  dựa vào phép đo trong QA report mục 5.

## Trust boundary

Không có gì để báo: nhánh chỉ đổi tầng trình bày. Không thêm request, không đổi
đường auth/authz, không đổi payload gửi lên API. `data-value` trên trigger chỉ
mang id/enum đã có sẵn trong DOM, không có PII. Không `dangerouslySetInnerHTML`.
Query lọc chỉ chạy client-side trên `label`.
