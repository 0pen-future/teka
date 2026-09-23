# Review Phase 1 — Nav Giảng dạy + Danh sách lớp + Chi tiết lớp (2026-09-23)

## Scope
- API: migration 000025, `shared/classcode`, `shared/dbtypes`, `features/classes/*` (model, dto, phase, repository, service, handler, routes), routespec + snapshot, testutil fixtures, imports/centers tests, seeds, teaching alias.
- Web: `features/roster` (2 trang, 8 component, url-state hook, libs, schemas/api/hooks, MSW), `dashboard-layout.tsx`, `hv-segmented.tsx`.
- Ngoài phạm vi: `docs/`, `apps/api/docs/`, `plans/journals/`.
- ~1.6k dòng sửa + ~2.4k dòng file mới.

## Verification đã chạy
| Lệnh | Kết quả |
|---|---|
| `go vet` classes/shared/imports/teaching/seeds | sạch |
| `go test` (không tag integration) classes, classcode, dbtypes, server, teaching, imports | pass |
| `make scopelint` | pass |
| `npx tsc --noEmit` web | pass |
| `npx eslint` roster/layouts/hv/msw | 0 lỗi, 4 warning (1 mới: `class-ops-card.tsx:154` `form.watch`, cùng loại warning có sẵn) |
| `npx vitest run` toàn bộ web | 106 file, 903 pass |
| Integration Go | KHÔNG chạy (theo ràng buộc) |

## Tổng quan
Phần tenancy làm đúng. `CountReadableByPhase` và `ListReadable` đều đi qua `readScoped`. `q` được escape `\ % _` với `ESCAPE '\'`. `tag` được bind qua `json.Marshal` + `?::jsonb`. Pointer-merge ở `Update` đúng. `Save()` ghi đủ cột nên `recruiting=false` và xoá note vẫn được lưu. Gate `GetByID` (scoped) giữ nguyên nên 404-vs-403 không đổi. Route `/stats` đứng trước `/:id` ở cả Gin và hai file MSW. Không có blocker về bảo mật. Rủi ro chính nằm ở tab Buổi học (side-effect ghi DB và N+1) và việc cắt cứng 100 dòng.

## Critical
Không có.

## High

### H1. Tab Buổi học materialise toàn bộ vòng đời lớp chỉ bằng việc mở tab — Confirmed
- `apps/web/src/features/roster/components/class-sessions-tab.tsx:40-47`, `lib/class-sessions.ts:22-31`; API `sessions/service.go:195-225` (`ListRangeReadable` → `materialiseRange` cho owner, `giao_vien`, hoặc `sessions.view_all`).
- Tab gửi mọi cửa sổ 400 ngày từ `start_date` tới `end_date` (hoặc hôm nay+90) song song. Mỗi GET chèn hàng `planned` cho mọi ngày theo lịch.
- Kịch bản 1: lớp nhập từ Excel, khai giảng 2026-01-05, trước đó chỉ mở sổ đầu bài vài tuần gần đây. Owner mở tab → hàng chục buổi `planned` trong quá khứ xuất hiện. Các buổi này rơi vào predicate "quá khứ, chưa xác nhận" của `ListPending`/`ListUnconfirmedInWindow` mà chốt kỳ dùng. Kết quả: danh sách chờ xác nhận phình ra và có thể chặn chốt sổ.
- Kịch bản 2: lớp có `end_date` năm 2027 → tạo trước ~1 năm buổi `planned` tương lai. Không có cơ chế dọn buổi planned khi sửa lịch tuần (grep `classes/service.go` UpdateSchedule/DeleteSchedule không đụng sessions). Đổi lịch từ Thứ 2 sang Thứ 4 sau đó → buổi Thứ 2 cũ vẫn nằm trong sổ đầu bài.
- Đề xuất: tab chỉ dùng đường read-only. Thêm query `readonly=1` gọi `ListRangeReadOnly`, hoặc thêm endpoint list không generate. Nếu vẫn muốn sinh buổi, giới hạn cửa sổ quanh hôm nay (ví dụ tháng hiện tại, giống classbook) và phân trang theo tháng.

### H2. N+1 truy vấn trên tab Buổi học — Confirmed
- `sessions/service.go:548-554` `toDetail` gọi `enrollments.ActiveOn` cho từng buổi. `materialiseRange` gọi nó trong vòng lặp (`:170-178`).
- Kịch bản: lớp 3 buổi/tuần chạy 2 năm, khoảng 310 buổi → 2 request, mỗi request vài trăm query `ActiveOn`, cộng bulk insert. Mở tab trên lớp dài là spike DB rõ rệt. Trước đây classbook chỉ gọi theo tuần hoặc tháng nên chi phí này bị giới hạn.
- Đề xuất: dùng đường đếm theo lô, giống cách `ListRangeReadOnly` để caller tự batch. Kết hợp với cửa sổ hẹp ở H1.

### H3. Danh sách lớp cắt cứng 100 dòng và lọc "Cần tuyển sinh" chạy phía client — Confirmed
- `apps/web/src/features/roster/pages/class-list-page.tsx:29` (`per_page: 100`, không có UI phân trang), `:44-46` (lọc recruiting trên trang đã tải).
- Kịch bản: trung tâm có 130 lớp. Chip "Tất cả 130" chỉ hiện 100 dòng, không báo gì. Chip "Cần tuyển sinh 12" có thể chỉ hiện 7 vì 5 lớp còn lại nằm ngoài 100 dòng đầu (sort theo tên). Số trên chip không khớp bảng.
- Plan R6 dùng `useClassesList({..., page})`. Index `idx_classes_center_recruiting` được tạo "cho lọc" nhưng API không có filter `recruiting`.
- Đề xuất: thêm `recruiting=true` vào `ListFilter` và `parseListFilter` (bool, sai giá trị → 422). Thêm phân trang (hoặc "Tải thêm") dùng `meta.total`.

## Medium

### M1. Race khi check-then-insert mã lớp trả 500 thay vì 409 — Confirmed
- `classes/service.go` `resolveCode` → `repo.CodeExists` rồi mới insert hoặc save. `TranslateError: true` (`database/postgres.go:24`) biến 23505 thành `gorm.ErrDuplicatedKey`, nhưng cả `CreateWithSchedules` lẫn `Update` đều không map lỗi này. `apperror.From` rơi về `Internal` → 500.
- Kịch bản: hai người cùng tạo lớp với mã `TOAN9` cùng lúc (hoặc import đang chạy song song với tạo tay). Cả hai probe đều thấy trống. Người thứ hai nhận 500 và log lỗi.
- Đề xuất: trong service, `errors.Is(err, gorm.ErrDuplicatedKey)` → `codeTakenError(code)`. Làm cho cả create (có thể retry generate) lẫn update. Giữ probe để có thông báo sớm.

### M2. Ops card ghi đè name/ngày/đơn giá bằng bản đọc cũ, và hai mutation đồng thời mất cập nhật — Plausible
- `roster-schemas.ts` `toClassUpdateInput` luôn gửi `name/start_date/end_date/default_unit_price` từ `klass` đang cache, vì PUT bắt buộc các trường này và chúng là full-replace. Comment trong ops card nói "never clobbers a rename made elsewhere", điều này chỉ đúng với `code`.
- Kịch bản A: tab 1 mở chi tiết lớp. Tab 2 đổi tên ở `/settings`. Tab 1 bật "Cần tuyển sinh" trước khi refetch → tên cũ bị ghi lại.
- Kịch bản B: nút switch và form (hai instance `useUpdateClass` riêng, `isPending` riêng) gửi gần nhau. Server `GetByID` → `Save()` ghi mọi cột, nên request đến sau ghi lại `recruiting` hoặc `note` cũ mà nó đã đọc.
- Đề xuất: repo có đường patch riêng cho catalog (`Model(&Class{}).Where(scoped).Updates(map[...]...)` chỉ gồm các cột có pointer khác nil). Hoặc cho base fields thành pointer ở PUT. Tối thiểu là disable switch khi form đang lưu và sửa lại comment.

### M3. Nút "+ Lớp học" không gate `classes.create` — Confirmed
- `class-list-page.tsx:64-66` render nút cho mọi người có `classes.list`. Trước đây luồng tạo lớp ở `/students` chỉ owner thấy (`students-page.tsx` shell guard). `classes.create` là key cấp riêng trong catalog (`authctx/catalog.go:177`).
- Kịch bản: member có `classes.list` nhưng không có `classes.create` điền hết form, bấm lưu, nhận 403.
- Đề xuất: `has("classes.create")` trước khi render nút và dialog.

### M4. Ô tìm kiếm ghi lại `q` cũ khi URL bị reset từ bên ngoài — Confirmed
- `class-filter-bar.tsx:23-34`: `query` chỉ khởi tạo từ `q` một lần và không bao giờ đồng bộ lại. Effect chạy khi `q` đổi, thấy `query !== q` rồi ghi `query` cũ lên URL sau 300 ms. Comment nói ngược lại.
- Kịch bản: đang ở `/classes?q=toan`, bấm "Danh sách lớp học" ở sidebar (hoặc link "Về danh sách lớp học") → URL thành `/classes`. 300 ms sau URL bật lại `?q=toan`.
- Đề xuất: lưu `q` cuối cùng đã commit trong ref. Khi `q` đổi mà khác giá trị đã commit thì `setQuery(q)`. Hoặc key component theo `q` ngoài.

### M5. Tab Học viên cắt 100 dòng nhưng tiêu đề hiện tổng — Confirmed
- `class-students-tab.tsx:23-27,43`: `per_page: 100` cho cả enrollments và students. Tiêu đề dùng `meta.total`.
- Kịch bản: lớp 120 học viên → "Học viên (120)" nhưng bảng có 100 dòng. Cột liên hệ trống với những học viên không nằm trong 100 dòng đầu của `/students?class_id=`.
- Đề xuất: phân trang, hoặc ít nhất ghi "hiển thị 100/120" và dẫn sang `/students?class_id=`.

## Low
- **L1. Backfill hỏng khi một trung tâm có từ 10 000 lớp (Confirmed).** `000025...up.sql` `lpad(rn::text, 4, '0')` cắt chuỗi dài hơn 4 ký tự (`'10000'` thành `'1000'`), trùng với `L1000`. Khi đó `CREATE UNIQUE INDEX` fail và migration dirty. Rất khó xảy ra nhưng sửa rẻ: `lpad(rn::text, greatest(4, length(rn::text)), '0')`.
- **L2. Filter weekday/shift khớp cả lịch chưa hiệu lực (Confirmed).** `repository.go` EXISTS chỉ kiểm tra `effective_to`, không kiểm tra `effective_from <= today`. Swagger ghi "still in effect". Lớp đổi lịch từ tháng sau sẽ khớp cả thứ cũ lẫn thứ mới. Nên thêm `s.effective_from <= ?` hoặc sửa mô tả.
- **L3. `phase=archived` với `status` mặc định `active` luôn rỗng mà không báo gì (Confirmed).** Web luôn gửi `status=all` nên chưa lộ. Client khác sẽ bị bẫy. Có thể 422 khi tổ hợp mâu thuẫn, hoặc cho `phase` override `status`.
- **L4. `today` phía web là ngày UTC (Confirmed, theo pattern có sẵn trong repo).** `class-list-page.tsx:26` và `class-detail-page.tsx:39` dùng `toISOString()`. Từ 00:00 đến 07:00 giờ VN, `activeSchedules` và horizon buổi học lệch một ngày so với `phase` do server tính theo `Asia/Ho_Chi_Minh`.
- **L5. `addDays` trong `lib/class-sessions.ts` trùng `addDaysIso` của `attendance/lib/session-dates.ts`.** Đây là vi phạm DRY.
- **L6. A11y (Plausible).** Chip `role="radio"` trong radiogroup không có roving tabindex hay phím mũi tên. Tooltip `title` trên tab và nút disabled không tới được bằng bàn phím hay screen reader, và Firefox không hiện title trên button disabled. Nên thêm `aria-describedby` hoặc text phụ.
- **L7. API không chuẩn hoá mã (Confirmed).** API không `ToUpper`/trim tags. Web upper-case mã nhưng client API trực tiếp gửi `toan9c` nhận 422. Tags toàn khoảng trắng và tags trùng lặp vẫn được API nhận.
- **L8. Test gap.** Test toggle không assert việc invalidate `stats` (ma trận test dòng "Toggle ..."). Không có test HTTP hoặc integration cho race 409 (M1).

## Không phải vấn đề (đã xác minh)
- **`SUM(...)` trả NULL khi scope rỗng:** GORM v1.31.2 scan struct qua `**int64` (`schema/pool.go`), nên NULL thành 0.
- **`CodeExists` không đi qua `scoped`:** đây là chủ đích, vì index tính theo trung tâm. Có `center_id` nên scopelint pass. Rò rỉ duy nhất là member biết một mã đã tồn tại trong cùng trung tâm, chấp nhận được.
- **Lệch giữa `canWriteClass` (owner hoặc `giao_vien`) và gate `scoped` (owner hoặc `teacher_id`):** `giao_vien` chỉ được cấp qua handoff (`classstaff/service.go:92`), nên luôn trùng `teacher_id`. Web không kiểm `classes.edit`, giống `class-settings-page` hiện có.
- **Tương thích ngược:** trường mới trong `ClassResponse` chỉ thêm vào. PUT cũ không gửi trường mới thì giữ nguyên. Hai consumer `ListFilter` ở centers chỉ đặt `Status`.
- **Thứ tự route:** `/classes/stats` đứng trước `/:id` ở Gin (`routes.go`) và ở cả hai file MSW. `classes/:id/settings` vẫn match riêng trong react-router.

## Plan follow-up
- R1, R2, R4, R5, R7, R8, R9 đã hoàn thành và khớp code. Ngoại lệ là L1 cho R1 và M2 cho R9.
- R3 thiếu `effective_from` (L2).
- R6 thiếu phân trang (H3).
- R10 đầy đủ trừ L8. Integration Go chưa được reviewer chạy.
- Tab Buổi học làm đúng thiết kế trong plan (cửa sổ 400 ngày nối tiếp). Chính thiết kế đó gây ra H1 và H2, nên cần quyết định lại ở cấp plan.

## Recommended actions (theo thứ tự)
1. H1 + H2: đổi tab Buổi học sang đọc không generate, với cửa sổ hẹp có phân trang.
2. H3: thêm filter `recruiting` phía API và phân trang cho danh sách.
3. M1: map `ErrDuplicatedKey` sang 409 `CLASS_CODE_TAKEN`.
4. M2: tách patch catalog khỏi `Save()` toàn hàng.
5. M3, M4, M5, rồi các mục Low.

## Unresolved questions
- Việc sinh buổi `planned` cho quá khứ và tương lai xa khi mở tab có phải là hành vi sản phẩm mong muốn không? Nếu có, cần thêm cơ chế dọn buổi planned khi sửa lịch.
- Chip "Tất cả" có nên gồm lớp lưu trữ không? Hiện tại có gồm, còn mặc định cũ của `GET /classes` là `active`.

Status: DONE_WITH_CONCERNS

## Disposition (cook, 2026-09-23)

| Finding | Xử lý | Bằng chứng |
|---|---|---|
| H1 + H2 | **Fixed, đảo cơ chế D7.** `GET /classes/:id/sessions?readonly=true` gọi `ListRangeExisting`: gate `GetReadableWithRoles`, chỉ đọc `ListByClassAndRangeReadable`, không materialise, không cap 400 ngày (chỉ kiểm `to >= from`), `student_count` từ một truy vấn `enrollments.ActiveInRange` rồi đếm theo ngày trong bộ nhớ. `readonly` không phải bool → 422 trường `readonly`. Tab Buổi học gửi đúng một request `from=start_date&to=end_date ?? hôm nay+90d&readonly=true`; bỏ `sessionWindows`. Đường generate mặc định giữ nguyên cho classbook/lịch. | `sessions/service_test.go` `TestListRangeExistingNeverGeneratesAndCountsRosterPerDate`; `sessions/handler_test.go` `TestListRangeReadonlyServesExistingRowsOnly`; `enrollments/integration_test.go` `TestActiveInRangeKeepsAnyEnrollmentOverlappingTheWindow`; `class-detail-page.test.tsx` "Buổi học tab" (3 test). |
| H3 | **Giảm nhẹ, follow-up.** Danh sách hiện `HvNotice` "Đang hiển thị N/total lớp…" khi `items.length < meta.total`. Filter `recruiting` phía API và phân trang chuyển sang follow-up (chưa thuộc scope phase; chip "Cần tuyển sinh" vẫn lọc phía client trên trang đã tải). | `class-list-page.test.tsx` "tells the reader when the catalog was cut". |
| M1 | **Fixed.** `codeConflictOr` map `gorm.ErrDuplicatedKey` từ create (trong tx) và update sang 409 `CLASS_CODE_TAKEN`; lỗi có `fields.code` khi biết mã. | `classes/service_test.go` `TestDuplicateKeyFromIndexIsCodeTakenConflict`. |
| M2 | **Accepted limitation.** PUT là full-replace theo hợp đồng hiện có (R2/R9); tách patch catalog đổi hợp đồng công khai, để follow-up. Không sửa code. | — |
| M3 | **Fixed.** Nút "+ Lớp học" và `ClassDialog` chỉ render khi `has("classes.create")`. | `class-list-page.test.tsx` "hides + Lớp học from a member without classes.create". |
| M4 | **Fixed.** `ClassFilterBar` giữ ref `committed`; `q` đổi từ ngoài (khác giá trị đã commit) → `setQuery(q)`, không re-arm. | `class-list-page.test.tsx` "follows the URL when the search is cleared from outside". |
| M5 | **Giảm nhẹ.** Tab Học viên hiện `HvNotice` "Đang hiển thị N/total học viên" + link "Xem tất cả" → `/students?class_id=`. Phân trang để follow-up cùng H3. | `class-detail-page.test.tsx` "tells the reader when the roster was cut". |
| L1–L8 | Ghi nhận, chưa sửa trong phase này. L1 (lpad ≥10k lớp) và L2 (`effective_from` trong filter weekday/shift) đưa vào follow-up. | — |

Gate sau sửa: `make lint`, `make test-api-unit`, `make scopelint`, `npm run typecheck`, `vitest run` (106 file, 908 pass), `go test -tags=integration -p 1` cho enrollments/sessions/classes: pass.
