# Code review — Hồ sơ học sinh: dropdown lớp + tìm học sinh

- Ngày: 2026-09-07
- Nhánh: `feat/records-class-dropdown-student-search` (6 commit trên `master`), diff = `git diff master...HEAD`
- Plan: `plans/260907-0434-records-class-dropdown-student-search/plan.md`
- Phạm vi: 63 file, +2949 / −125. Read-only review, không sửa file nguồn.

## Verdict

**DONE_WITH_CONCERNS** — không có lỗi chặn merge. Đạt cả 8 Success Criteria, mọi gate xanh,
không breaking change ngoài trường JSON additive. Còn 1 vấn đề phạm vi đọc phía API (F1) và
2 lỗi bàn phím phía web (F2, F3) nên sửa trước khi ship.

## (a) Success Criteria

| # | Tiêu chí | Kết quả | Bằng chứng |
|---|---|---|---|
| 1 | Bỏ `role="tablist"`, toolbar trắng bo 20px, hai nhãn LỚP / TÌM HỌC SINH | PASS | grep không còn `role="tab"` trong `features/teaching`; `records-toolbar.tsx` dùng `rounded-[20px] bg-white shadow-soft-sm` |
| 2 | Trigger `Tên lớp · N HS`, popover ≥sm / sheet <sm, nhóm "Lớp đang dạy", ✓ mint, ô lọc >5, note không khớp, ↑↓ Enter Esc, `?class_id=` replace | PASS (lưu ý F2) | `records-class-select.test.tsx` phủ ArrowUp/Down/Home/End/Enter/Esc, ngưỡng lọc >5, `emptyNote` |
| 3 | `<mark>` sun-200, counter `n / N` aria-live, `?q=`, `/` focus, × xoá | PASS (lưu ý F3) | `records-pages.test.tsx`, `records-toolbar.test.tsx` |
| 4 | Card không khớp + "Xoá tìm kiếm"; lớp rỗng giữ khối cũ | PASS | Trang rẽ nhánh theo `rows` chưa lọc nên hai trạng thái không đè nhau |
| 5 | 5 hàng shimmer, reduced motion, status sr-only | PASS | `SKELETON_ROWS = 5`, `motion-reduce:animate-none`, `<p role="status" className="sr-only">` |
| 6 | <768px: toolbar dọc, hàng 2 dòng, nút Xem, counter + CSV dưới bảng | PASS | Test compact trong `records-pages.test.tsx`; spec e2e `records-search.spec.ts` (Playwright không chạy trong phiên này) |
| 7 | Tiêu đề, CSV, cột/màu desktop, trang chi tiết, sessions/students/classbook không đổi | PASS | `git diff --name-only` không chạm file cấm; `records-pages.test.tsx` có 0 dòng xoá |
| 8 | Gate web + api xanh | PASS | xem mục (e) |

## (b) Blast radius

**Go.** `fakeRepository` trong `service_test.go` là implementer duy nhất còn lại của
`classes.Repository`, đã cập nhật. `ClassIDs` chỉ thay đoạn inline cũ trong `ListReadable`,
hành vi y hệt. `StudentCounts` cố ý không nhét vào `ListReadable` / `GetReadableWithRoles`
nên probe ghi của sessions không tốn thêm query. `FromModel` (create/update/archive) vẫn trả
`student_count: 0`, giống cách `my_staff_roles` đã làm; web không `setQueryData` từ response
mutation nên không có cache bẩn.

**Web.** `HvModal` chỉ thêm prop optional `onCloseAutoFocus`, consumer khác không đổi.
`StudentRecordsTable` chỉ có `records-page` dùng. `useClassSearch` tái dùng nguyên trạng
(`sessions-page`, `students-page` không đổi). Nội dung CSV vẫn dựng từ `rows` chưa lọc.

## (c) Public contract

`student_count` là additive; zod có `.default(0)`; swagger regenerate đúng 1 property trong
`ClassResponse` (docs.go / swagger.json / swagger.yaml). Không đổi chữ ký export nào, không
env var mới, không dependency mới.

## (d) Convention

0 hex thô, 0 `any`, 0 `eslint-disable` / `@ts-expect-error`, 0 mã phase hay plan ID trong
comment. A11y có `role=listbox` / `role=option`, `aria-labelledby`, `aria-controls`,
`aria-haspopup=listbox`, và trả focus về trigger ở cả hai nhánh.

## (e) Gate

| Gate | Kết quả |
|---|---|
| `npm run lint` | 0 lỗi, 5 warning cũ (đều trong `roster/`, không bị đụng) |
| `npm run typecheck` | sạch |
| `npx vitest run` | 84 file, 626 pass / 3 skip |
| `npm run build` | ok |
| `go build ./...` | ok |
| `go test ./...` | ok |
| `go vet -tags integration ./internal/features/classes/...` | compile ok |
| `go run ./tools/scopelint ./internal/...` | exit 0 |

Chưa xác minh: `TestStudentCountsMatchActiveEnrollments` nằm sau build tag `integration`, cần
Docker nên **không chạy**. E2E Playwright cũng không chạy theo yêu cầu.

## Findings

### F1 — Medium-High. `student_count` bỏ qua bộ lọc đọc của enrollments

`apps/api/internal/features/classes/repository.go:314-334` đếm center-wide, trong khi
`enrollments.readScoped` (`apps/api/internal/features/enrollments/repository.go:139-147`) còn
thu hẹp thêm khi thiếu `enrollments.view_all`:

```go
Where("center_id = ? AND class_id IN ? AND ended_on IS NULL AND deleted_at IS NULL", sc.CenterID, classIDs)
```

Một member được cấp `classes.view_all` nhưng không có `enrollments.view_all` và không có stint
trên lớp sẽ thấy trigger ghi "N HS" còn bảng rỗng, đồng thời biết được sĩ số của lớp mà họ
không được xem roster. Hai key này độc lập trong `BuildPermSet` và `impliedKeys` không nối
chúng, nên cấu hình này có thật. Comment biện minh bằng tiền lệ `CountOpenEnrollments`, nhưng
cái đó là guard nội bộ cho lệnh xoá, không trả ra client.

**Sửa:** trong `CountActiveEnrollmentsByClass`, nếu `!sc.CenterWideFor(authctx.PermEnrollmentsViewAll)`
thì thêm `(enrollments.teacher_id = ? OR <classscope.ReadExists("enrollments.class_id")>)`
đúng như port enrollments.

### F2 — Medium. Space trên option không chọn lớp khi có >5 lớp

`apps/web/src/features/teaching/components/records-class-select.tsx:34, 89-94`. `isPrintableKey`
coi `" "` là ký tự in được, nên nhánh `default` gọi `preventDefault()` rồi đẩy dấu cách vào ô
lọc thay vì để button kích hoạt. Comment ngay trên đó hứa "Enter/Space activate natively" —
chỉ đúng khi ≤5 lớp, thành ra hành vi bàn phím khác nhau giữa hai ngưỡng.

**Sửa:** `return event.key.length === 1 && event.key !== " " && !event.ctrlKey && !event.metaKey && !event.altKey;`

### F3 — Medium. Phím `/` của trang tranh chấp với picker đang mở

`apps/web/src/features/teaching/pages/records-page.tsx:25-31, 86-89`. `isTypingTarget` chỉ nhận
input/textarea/select/contenteditable, nên khi focus đang ở một `role="option"`: với >5 lớp,
`/` vừa bị append vào ô "Tìm lớp" (qua nhánh default của F2) vừa kéo focus sang ô tìm học sinh
làm popover đóng; với bottom sheet thì còn đấu focus với trap của Radix Dialog.

**Sửa:** thêm một dòng bail đầu handler:
`if (document.activeElement?.closest('[role="listbox"],[role="dialog"]')) return;`

### F4 — Low. Bộ đếm dùng nhầm cờ pending

`apps/web/src/features/teaching/pages/records-page.tsx:81` lấy `total` theo `sessionsPending`,
không theo trạng thái của `useEnrollmentsList`. Nếu sessions về trước enrollments thì counter
nháy "0 học sinh" kèm khối "Lớp chưa có học sinh đang học.". Nhánh rẽ này có sẵn từ master nên
không phải hồi quy, nhưng giờ nó lái thêm một bề mặt mới. **Sửa:** OR thêm `isPending` của
query enrollments.

### F5 — Low. Trạng thái "chưa có lớp nào"

Khi danh sách lớp rỗng, trigger hiện "Chọn lớp" với listbox trống và trang vẫn nói "Lớp chưa có
học sinh đang học." (sai chủ thể) kèm counter "0 học sinh". Wording cũ từ master, nhưng counter
là mới.

### F6 — Low. `mockViewport` không có đường reset

`apps/web/src/test/viewport.ts` giữ `viewportWidth` ở module scope và thay `window.matchMedia`
vĩnh viễn cho cả file test, không có `resetViewport()`. Hiện an toàn vì mỗi describe đặt
`beforeEach(() => mockViewport(...))`, nhưng test nào quên sẽ thừa hưởng width của test trước.

### F7 — Info. Hai live region cùng lúc khi đang tải

Skeleton có `role="status"` và `ResultCount` cũng vậy, nên trong lúc loading một breakpoint có
2 status (counter rỗng). Không hỏng test hiện tại, nhưng `getByRole("status")` trong test tương
lai sẽ vỡ strict mode.

### F8 — Info. IME tiếng Việt

Ô tìm là controlled input đồng bộ qua URL mỗi keystroke. Nên thử telex/VNI trên Android trước
khi ship; không có bằng chứng lỗi, chỉ là rủi ro chưa được phủ.

## Non-issues (đã kiểm và loại)

- `findFoldedMatch` xử lý đúng NFD, `đ/Đ`, hoa/thường, surrogate pair, và kéo dấu phụ đuôi vào
  range — `"Trần"` với query `"an"` tô đúng `ần`.
- Truy vấn đếm có `center_id`, được `idx_enrollments_class` phục vụ, và chỉ 1 query grouped cho
  cả trang nên không N+1.
- `useSyncExternalStore` trả boolean nên snapshot ổn định, không loop.
- `<mark>` cắt từ chính `name` nên không có đường XSS; `q` chỉ được render như text.
- 5 deviation cố ý mà lead liệt kê đều hợp lý; riêng deviation 1 (re-pick giữ `q`) đã có test
  khoá hành vi rõ ràng.
- Fixture `Class` ở attendance/collections/roster handlers đã cập nhật `student_count`, MSW mô
  phỏng đúng predicate của API.

## Việc nên làm, theo thứ tự

1. Sửa F1 (quyết định sản phẩm: che số hay lọc theo quyền — đề xuất lọc theo port enrollments).
2. Sửa F2 và F3, cả hai là sửa một dòng, kèm test bàn phím.
3. Cân nhắc F4–F6 trong cùng đợt.

## Unresolved questions

- F1: sĩ số có được phép là thông tin center-wide không, hay phải bám đúng phạm vi đọc roster
  của người gọi? Cần chủ sản phẩm quyết.
- `TestStudentCountsMatchActiveEnrollments` và 3 spec e2e chưa được chạy lại độc lập trong
  review này (cần Docker / Playwright).
