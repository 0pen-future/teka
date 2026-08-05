# Code review — section "CHỌN LỚP" (Điểm danh + Lớp & học sinh)

Ngày: 2026-08-05 · Baseline: `ff52003` · Nhánh: master (uncommitted)
Reviewer: code-reviewer subagent · Không sửa file nào.

## Phạm vi

| File | Trạng thái | LOC |
|---|---|---|
| `apps/web/src/features/roster/hooks/use-class-search.ts` | mới | 32 |
| `apps/web/src/features/roster/components/class-search.tsx` | mới | 26 |
| `apps/web/src/features/roster/index.ts` | +2 | — |
| `apps/web/src/features/roster/pages/students-page.tsx` | +9/-1 | — |
| `apps/web/src/features/attendance/pages/sessions-page.tsx` | +38/-22 | — |
| `apps/web/src/features/attendance/__tests__/sessions-page.test.tsx` | +63 | — |

Gates đã chạy lại (không tin kết quả báo trước):

- `npx tsc -b --noEmit` → exit 0, sạch.
- `npx eslint src/features/roster src/features/attendance` → 0 error, 3 warning `react-hooks/incompatible-library` đều ở `class-dialog` / `student-dialog` / `class-settings-page` (pre-existing, không thuộc diff).
- `npx vitest run` → 24 file / 107 test pass.

## Kết luận acceptance criteria

| # | Tiêu chí | Kết quả | Bằng chứng |
|---|---|---|---|
| 1 | ≤5 lớp ẩn input; >5 hiện, đứng trước dãy tab | PASS | `use-class-search.ts:19`; `sessions-page.tsx:129-131`, `students-page.tsx:120-122` (input là flex item đầu tiên, trước `<div role="tablist">`). Test `sessions-page.test.tsx:78-81, 87`. Màn học sinh chỉ verify bằng đọc code — không có test. |
| 2 | Lọc chỉ thu hẹp tab lớp thật; "Tất cả"/"Chưa ghi danh" luôn còn | PASS | `students-page.tsx:124-128` — hai tab cố định nằm ngoài spread `classSearch.filtered`. |
| 3 | Không khớp → note đúng chuỗi, inline | PASS | `use-class-search.ts:29` chuỗi khớp chính xác; `class-search.tsx:25` đúng token 13px / `font-bold` / `text-ink-400`; render sau tablist (`sessions-page.tsx:154`, `students-page.tsx:149`). Test `sessions-page.test.tsx:110`. |
| 4 | Xoá query khôi phục đủ tab; lọc không đổi lớp đang chọn | PASS | `sessions-page.tsx:74` `explicitClassId ?? classes[0]?.id` dùng `classes` đầy đủ, không dùng `filtered`; `students-page.tsx:40` selection nằm ở URL param. Test clear ở `sessions-page.test.tsx:99-100`. Xem M1 về hệ quả UX. |
| 5 | Không regression (tab, `class_id` URL, session list, ⚙, create-session) | PASS có lưu ý | Không có logic nghiệp vụ nào bị đụng: `selectClass`, debounce `q`, `useStudentsList`, `useSessionsList`, điều kiện ⚙ đều nguyên vẹn. Lưu ý M4 và H1. |

Contract công khai (c): barrel chỉ **thêm** export (`index.ts:6-7`), không đổi/xoá gì. `useClassSearch<T extends NamedClass>` trả về `T[]` — không widening, không `any`, không đổi schema/DB. Không có circular import (`class-search.tsx` và `use-class-search.ts` không import ngược barrel).

## Findings

### High

**H1 — Màn "Lớp & học sinh" chỉ tải 20 lớp đầu, nên ô tìm kiếm báo "không khớp" sai**
`apps/web/src/features/roster/pages/students-page.tsx:68`

```ts
const { data: classesPage } = useClassesList({ status: "active" });
```

Không truyền `per_page`. Backend mặc định 20 (`apps/api/internal/shared/pagination/pagination.go:17` `defaultPerPage = 20`). `sessions-page.tsx:69` thì truyền `per_page: 100` (đúng bằng `maxPerPage`).

Kịch bản hỏng: giáo viên có 25 lớp active. Ô "Tìm lớp…" hiện (vì `classes.length = 20 > 5`), người dùng gõ tên một lớp nằm ở trang 2 → `filtered.length === 0` → hiển thị `Không có lớp nào khớp "Hoá 12"` **dù lớp đó tồn tại**. Trước khi có tính năng này thì việc cắt 20 tab chỉ là thiếu pill; giờ nó biến thành một khẳng định sai với người dùng.

Sửa tối thiểu: `useClassesList({ status: "active", per_page: 100 })`. Nếu muốn chắc, dùng `classesPage?.meta.total` để phát hiện truncation và đổi note thành "chỉ tìm trong N lớp đã tải" hoặc chuyển search lên server (`?query=`) — nhưng theo YAGNI thì `per_page: 100` là đủ cho quy mô sản phẩm hiện tại, và nên làm cùng lúc ở cả hai màn để đồng nhất.

### Medium

**M1 — Lọc mất tab đang chọn → không tab nào `aria-selected="true"`, và hành động ghi dữ liệu vẫn nhắm vào lớp vô hình**
`apps/web/src/features/attendance/pages/sessions-page.tsx:132-153`, `create-session-dialog.tsx:31-77`

Kịch bản: 6 lớp, chọn "Văn 8C", gõ "anh" → chỉ còn pill "Anh 9A" (chưa chọn). Nhưng:

- danh sách buổi học bên dưới vẫn là của Văn 8C, không còn dấu hiệu nào cho biết đang ở lớp nào;
- nút "Thêm buổi học" (`sessions-page.tsx:110-116`) vẫn enable với `selectedClassId = Văn 8C`, và `CreateSessionDialog` chỉ có title "Thêm buổi học" — **không hiển thị tên lớp**. Người dùng có thể tạo buổi học vào một lớp mà màn hình không hề hiện tên.

Đây là hệ quả trực tiếp của AC4 (đúng spec), nhưng hệ quả ghi dữ liệu thì không nằm trong spec. Hai lựa chọn rẻ tiền: (a) luôn giữ pill của lớp đang chọn trong `filtered` dù không khớp query, hoặc (b) hiển thị tên lớp đang chọn trong `CreateSessionDialog` / cạnh dãy tab. (a) vẫn thoả AC4 ("lọc không đổi lớp đang chọn") và giải quyết cả M2. Đây là thay đổi hành vi so với spec prototype nên cần user quyết, không tự sửa.

**M2 — `role="tablist"` render 0 tab con khi không khớp**
`apps/web/src/features/attendance/pages/sessions-page.tsx:132-153`

Khi `filtered` rỗng, `<div role="tablist" aria-label="Lớp">` vẫn tồn tại nhưng không sở hữu `role="tab"` nào — vi phạm ARIA (tablist phải chứa ≥1 tab), và screen reader thông báo một tablist rỗng. Test hiện tại còn assert chính trạng thái này (`sessions-page.test.tsx:109`). Màn học sinh không dính vì luôn có 2 tab cố định. Sửa: bọc `{classSearch.filtered.length > 0 ? <div role="tablist">…</div> : null}` ở `sessions-page`.

**M3 — Không có test nào cho màn "Lớp & học sinh"**
`apps/web/src/features/roster/__tests__/` (không có `students-page.test.tsx`)

AC2 — điều kiện quan trọng nhất và duy nhất chỉ tồn tại ở màn này ("Tất cả"/"Chưa ghi danh" luôn còn) — chỉ được xác minh bằng đọc code. AC1/AC4 trên màn này cũng vậy. Cả 3 test mới đều nằm ở `sessions-page`. Đề xuất thêm 1 test roster: 6 lớp → gõ chuỗi không khớp → `getAllByRole("tab")` trả về đúng `["Tất cả", "Chưa ghi danh"]` và note hiện. Đây là ~15 dòng và khoá được đúng ràng buộc plan ghi trong "Constraints".

**M4 — Pill "⚙ Cài đặt lớp" vẫn hiện khi tab lớp tương ứng đã bị lọc mất**
`apps/web/src/features/roster/pages/students-page.tsx:153`

Điều kiện dùng `classes` (danh sách đầy đủ) trong khi pill dùng `classSearch.filtered`. Chọn "Văn 8C" → gõ "anh" → dãy tab không còn Văn 8C nhưng link ⚙ vẫn trỏ `/classes/<Văn 8C>/settings` và bảng học sinh vẫn của Văn 8C. Nhất quán với AC4, nhưng comment ngay phía trên (`:151-152`) nói "⚙ chỉ hiện khi một tab lớp thật đang active" — comment giờ không còn đúng với những gì render ra. Ít nhất phải sửa comment; lý tưởng thì gộp với hướng xử lý M1.

### Low

**L1 — Note nội suy `query` chưa trim, trong khi lọc thì trim**
`apps/web/src/features/roster/hooks/use-class-search.ts:21` vs `:29`. Gõ `"  hoá  "` → note ra `Không có lớp nào khớp "  hoá  "`. Dùng lại biến `q` gốc đã trim (chưa lowercase) sẽ nhất quán.

**L2 — Không fold dấu tiếng Việt, không normalize Unicode**
`use-class-search.ts:25`. `toLowerCase()` xử lý đúng chữ hoa có dấu ("Toán" → "toán"), nên yêu cầu case-insensitive trong spec là ĐẠT. Nhưng gõ "toan" không ra "Toán" — thói quen rất phổ biến của người dùng VN. Backend cũng chỉ dùng `ILIKE` (`apps/api/internal/features/students/repository.go:105`) nên đây là hành vi *nhất quán với dự án*, không phải lỗi. Rủi ro thật hơn: không có `.normalize("NFC")` — tên lớp lưu dạng NFD (gõ từ macOS/iOS) sẽ không khớp chuỗi NFC người dùng gõ, dù trông giống hệt nhau. Một dòng `.normalize("NFC")` cho cả hai vế loại bỏ hẳn lớp lỗi này.

**L3 — Chuỗi class trùng lặp nguyên si**
`class-search.tsx:19` trùng từng utility với input tìm học sinh ở `students-page.tsx:170` (chỉ khác `w-[150px]` vs `w-full max-w-[240px]`). Repo chưa có `HvInput` nên viết inline là đúng pattern hiện tại, nhưng hai chuỗi giống nhau cách nhau 50 dòng trong cùng feature nên tách một hằng `pillSearchInputClassName`.

**L4 — `ClassSearchEmptyNote` là wrapper 1 dòng**
`class-search.tsx:24-26`. Ranh giới trừu tượng khá mỏng (dấu hiệu điển hình của code sinh bởi AI), nhưng nó gom 3 token DS vào một chỗ và có 2 call site → giữ được, không cần đổi.

**L5 — `useMemo` mất tác dụng khi query classes chưa có data**
`students-page.tsx:69` / `sessions-page.tsx:70` `classesPage?.items ?? []` tạo array mới mỗi render → `filtered` recompute mỗi render lúc đang tải. Chi phí không đáng kể ở list ≤100 phần tử; ghi nhận cho đủ.

**L6 — Chiều cao input (~41px) thấp hơn touch target 44px của các pill**
`class-search.tsx:19` (`py-2` + 13.5px) so với `min-h-11` của tab. Giống hệt input tìm học sinh sẵn có nên không phải sai lệch mới, và đúng spec prototype (`px-4 py-2`). Chỉ cần kiểm tra mắt phần canh dòng khi tab wrap.

**L7 — Barrel roster giờ export cả component**
`index.ts:6`. Comment đầu file lập luận về ranh giới chunk (giữ `routes.tsx` riêng). Thêm component vào barrel kéo JSX vào mọi consumer của `@/features/roster`. Ảnh hưởng không đáng kể (26 dòng), nhưng `students-page` đã import theo đường dẫn tương đối rồi — chỉ `sessions-page` cần barrel.

## Các edge case được yêu cầu kiểm tra

**Query cũ khi danh sách tụt xuống ≤5 sau refetch — ĐÚNG như thiết kế.**
`use-class-search.ts:22` `if (!showSearch || !q) return classes` chặn hẳn việc lọc ngầm. Kiểm chứng thêm: state `query` vẫn được giữ khi input unmount, nên nếu số lớp vượt lại 5 (tạo lớp mới), input remount với query cũ và lọc ngay. Vì giá trị hiển thị ngay trong ô nên không phải lọc "vô hình" — chấp nhận được, không cần sửa.

**Layout flex-wrap.** `students-page.tsx:119` — input (150px) + tablist (tự wrap bên trong) + note + `ml-auto` cho ⚙. Tablist lồng bên trong có `min-width: auto` = độ rộng pill dài nhất nên vẫn co và tự wrap được; `items-center` canh input với hàng pill. Không thấy vấn đề tràn. Trường hợp `filtered` rỗng, tablist thành flex item rộng 0 — vô hại.

**E2E — an toàn, đã verify.** `apps/api/seeds/seed.go:111,119` chỉ seed 2 lớp active → ngưỡng >5 không bao giờ chạm trong e2e, input không xuất hiện. Selector `role="tab"` duy nhất trong e2e là `collections.spec.ts:79-80` (màn Thu tiền, không nằm trong diff, là non-goal của plan). `roster.spec.ts:75` dùng `getByPlaceholder("Tìm theo tên học sinh")` — khác hẳn `"Tìm lớp…"`, không mơ hồ. Đổi lại: tính năng mới **không có** e2e coverage nào.

**Bẫy focus-visible (d) — lập luận ĐÚNG, đã kiểm chứng.** `src/styles/globals.css:229-232` đặt `:focus-visible { outline: none; box-shadow: var(--ring) }` trong `@layer base`. Input mới không mang utility `shadow-*` nào nên box-shadow ring nền không bị ghi đè. `outline-none` trong Tailwind v4 chỉ phát ra `outline-style: none` (khác v3 vốn set `outline: 2px solid transparent`), không đụng tới `box-shadow` → ring vẫn hiện, cộng thêm `focus:border-mint-400`. Không cần `focus-visible:ring-4`. Các tab pill vẫn giữ nguyên `focus-visible:outline-none focus-visible:ring-4` vì có `shadow-press-mint`/`shadow-soft-sm`.

**Pattern file.** kebab-case đúng; tách hook sang `hooks/use-class-search.ts` thay vì gộp vào `class-search.tsx` như plan viết là **lệch plan có căn cứ** — `react-refresh/only-export-components` sẽ báo lỗi nếu file component export thêm hook. Đây là lệch đúng, nên cập nhật mục "Files" trong `plan.md`.

## Rủi ro AI-code đã soi (không phát hiện)

Không có helper generic vô chủ, không reimplement util sẵn có, không `any`, không catch-and-swallow, không suppress lint mới, không file lạc đề (3 file untracked `.agents/`, `engineer/` không thuộc diff này nhưng nên thêm vào `.gitignore` hoặc dọn trước khi commit). Test mới có assert hành vi thật (đếm tab, so text từng pill, clear khôi phục) chứ không phải test rỗng — riêng assert `getByRole("heading", { name: "Cần điểm danh" })` ở `sessions-page.test.tsx:97` là proxy hơi gián tiếp cho "selection không đổi"; assert thẳng vào tên lớp trong danh sách buổi học sẽ chặt hơn.

## Hành động đề xuất (theo thứ tự)

1. **H1** — thêm `per_page: 100` vào `useClassesList` ở `students-page.tsx:68`. Một dòng, chặn được lỗi báo sai "không có lớp nào khớp".
2. **M2** — chỉ render `<div role="tablist">` khi `filtered.length > 0` ở `sessions-page`.
3. **M3** — thêm test roster khoá AC2 (2 tab cố định sống sót khi query không khớp).
4. **M1 + M4** — chốt với user: giữ pill của lớp đang chọn luôn hiển thị, hay hiện tên lớp trong `CreateSessionDialog`? Đồng thời sửa comment `students-page.tsx:151-152` cho khớp hành vi thực tế.
5. **L1, L2** — trim `query` trong note; cân nhắc `.normalize("NFC")`.
6. Cập nhật mục "Files" của `plan.md` cho khớp việc tách `hooks/use-class-search.ts`; dọn `.agents/` và `engineer/` trước khi commit.

## Metrics

- Type: `tsc -b --noEmit` sạch; 0 `any` trong diff; hook generic có ràng buộc `T extends NamedClass`.
- Lint: 0 error, 3 warning pre-existing ngoài diff.
- Test: 107/107 pass; 3 test mới, đều ở `sessions-page`; `students-page` 0 test; e2e 0 coverage cho tính năng.

## Câu hỏi còn treo

1. Khi lớp đang chọn bị lọc khỏi dãy pill, sản phẩm muốn hành vi nào? (giữ pill đang chọn / hiện tên lớp ở dialog / để nguyên như prototype)
2. `per_page` cho danh sách lớp ở màn học sinh nên nâng lên 100 hay chuyển search sang server-side? (100 là trần cứng của API — trên 100 lớp active thì cả hai màn đều sai)
3. `roster.spec.ts:50,89` vẫn `page.goto("/classes")` trong khi commit `1161903` đã gỡ luồng quản lý lớp cũ — pre-existing, ngoài phạm vi diff này, nhưng cần xác nhận e2e đó còn chạy được.
