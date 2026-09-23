# Review — Phase 5: Danh mục khóa học & liên kết lớp ↔ khóa

Ngày: 2026-09-24 · Branch `feat/giang-day-menu` · Phạm vi: diff chưa commit và các file untracked của phase (bỏ qua `docs/diagrams/`, `.pi/` và hunk `docs/architecture.md` theo yêu cầu).

## Verdict: **SHIP WITH FIXES**

Không có lỗi Critical hay High. Tenancy đạt: mọi truy vấn mới lọc `center_id`, và các FK composite chặn nối chéo trung tâm ở cả `courses → program_template_versions`, `course_tuition_packs → courses` và `classes → courses`. Route manifest, snapshot (+7) và audit (+5) khớp nhau. Backfill `courses.read` theo đúng khuôn 000027, có sổ `rbac_backfill_rows` để down gỡ đúng dòng. Các caller hiện có của `useApiFormErrors` không bị hồi quy.

Có ba lỗi Medium:
- **Không gỡ được khóa học khỏi lớp qua HTTP.** `course_id: ""` bị binding từ chối với 422, trái với hợp đồng "chuỗi rỗng là gỡ". Test service xanh nhưng không đi qua binding.
- **Sửa khóa học bị chặn vĩnh viễn** khi phiên bản chương trình mẫu mặc định đã bị lưu trữ hoặc template đã bị xoá. Mọi PUT đều gửi lại id cũ và nhận 422, kể cả khi chỉ đổi tên.
- **Race xoá khóa ↔ gắn lớp.** Xoá khóa không khoá gì, nên một lớp có thể được gắn vào khóa đúng lúc khóa bị xoá mềm. Chip khóa của lớp sau đó trỏ tới trang 404.

Phần còn lại là Low/Nit.

## Verification đã chạy

| Lệnh | Kết quả |
|---|---|
| `go build ./...` | OK |
| `go vet` courses, classes (+ `-tags integration` cho courses, classes, migrations) | OK |
| `gofmt -l` courses, classes, shared, server | sạch |
| `go test` courses, classes, server, audit, authctx, routespec | tất cả `ok` |
| `golangci-lint run` courses, classes | 0 issues |
| `npx vitest run src/features/courses src/features/roster src/lib/forms` | 27 files, 215 passed, 3 skipped |
| `npx tsc -b --noEmit` (web) | OK |
| `npx eslint` courses, roster, lib/forms, `e2e/courses.spec.ts` | 0 error, 5 warning (`react-hooks/incompatible-library` do `form.watch`, cùng kiểu với code sẵn có; một cái mới ở `course-dialog.tsx:94`) |
| Chương trình Go nhỏ trong scratchpad, dùng đúng `go.mod` của API, gọi `binding.Validator.ValidateStruct` trên `*string` với tag `omitempty,uuid` | Con trỏ tới `""` **fail** tag `uuid`. `nil` thì pass. Đây là bằng chứng cho M1. |

Đọc, không chạy: `courses/integration_test.go`, `TestCourseLinkStaysInsideCenterAndCopiesPrice` trong `classes/integration_test.go`, `TestCoursesBackfillGrantsCoursesRead` trong `migrations_test.go`, và `e2e/courses.spec.ts`. Theo yêu cầu, không chạy integration hay docker.

## Findings

### Critical
Không có.

### High
Không có.

### Medium

**M1 — `course_id: ""` bị binding trả 422, nên hợp đồng "chuỗi rỗng là gỡ khóa" của `PUT /classes/:id` không dùng được qua HTTP. `POST /classes` cũng không nhận "blank" như comment mô tả.** *(CONFIRMED bằng thực nghiệm)*
`apps/api/internal/features/classes/dto.go:40` và `:65` (`CourseID *string binding:"omitempty,uuid"`); `service.go:394` (`resolveCourse`); `service_test.go` (`TestUpdateAttachesAndDetachesCourse`)
- Với field con trỏ, validator v10.30 coi con trỏ khác nil là "có giá trị", kể cả khi trỏ tới `""`. Vì vậy `omitempty` không bỏ qua, và tag `uuid` chạy trên chuỗi rỗng rồi fail. Chương trình kiểm tra cho ra `Field validation for 'CourseID' failed on the 'uuid' tag` với `""`.
- Kịch bản: một client, ví dụ Phase 7 hoặc app, gửi `PUT /classes/:id {"name": …, "course_id": ""}` để gỡ khóa theo đúng swagger và comment DTO. Kết quả là 422 `course_id`, và lớp không bao giờ gỡ được khỏi khóa.
- Hệ quả dây chuyền: lớp không gỡ được thì `DELETE /courses/:id` trả 409 `COURSE_IN_USE` mãi, trừ khi xoá lớp.
- Nhánh `""` của `resolveCourse` là code chết qua HTTP. `TestUpdateAttachesAndDetachesCourse` gọi service trực tiếp nên bỏ qua binding, và đây là một test ảo cho hành vi này. Web hiện chưa có đường gỡ, vì `ClassDialog` chỉ tạo lớp và `toClassCreateInput` bỏ `course_id` khi rỗng. Nhờ vậy UI hôm nay chưa lộ lỗi.
- Fix:
  - Đổi tag thành `binding:"omitempty"` và để `resolveCourse` tự parse. Hàm này đã trả 422 `course_id` khi uuid sai. Cách khác là dùng validator tự viết kiểu `uuid_or_empty`.
  - Thêm case HTTP vào `TestCourseAttachmentOverHTTP`: `PUT` với `"course_id": ""` trả 200 và `course: null`, còn `"course_id": "x"` trả 422.

**M2 — `PUT /courses/:id` kiểm lại `default_template_version_id` ngay cả khi không đổi. Khi phiên bản đó bị lưu trữ hoặc template bị xoá mềm, mọi lần sửa khóa đều trả 422.** *(CONFIRMED qua đọc code)*
`apps/api/internal/features/courses/service.go:185-203` (`fromRequest`), `:98-111` (`Update`); `repository.go:330-344` (`summary` join không lọc trạng thái hay `t.deleted_at`), `:423-435` (`FindTemplateVersion` join `t.deleted_at IS NULL`); `apps/web/src/features/courses/components/course-dialog.tsx` (`toCourseInput(values, props.course.default_template_version_id)`); `pages/course-detail-page.tsx:322` (`courseToInput(course)`) và `:350` (`status: "published"` gán cứng)
- Web luôn gửi lại id phiên bản đang lưu, cả ở dialog "Sửa" lẫn tab chương trình mẫu. Service coi mọi id khác nil là giá trị mới, rồi đòi `status = published` và template còn sống.
- Kịch bản:
  1. Owner gắn khóa TOAN-6 với "Toán 6 · v1" đã phát hành.
  2. Người có `library.edit` lưu trữ v1 (`POST /library/versions/:vid/archive`), hoặc xoá chương trình mẫu.
  3. Owner mở "Sửa" để đổi tên hay đổi đơn giá. Kết quả là 422, và lỗi hiện ở footer dạng thô "default_template_version_id: phải là phiên bản đã phát hành", vì field này không nằm trong form.
  4. Nút "Ngừng tuyển" (archive) vẫn chạy vì đi đường riêng. Riêng việc chuyển trạng thái draft ↔ active qua dialog thì bị chặn.
- Hiển thị cũng sai theo. `summary` vẫn trả `default_template` cho phiên bản đã lưu trữ hoặc template đã xoá. Tab chương trình mẫu gán nhãn cứng "đã phát hành", và link `/library/templates/:id` dẫn tới 404 khi template đã xoá.
- Fix:
  - Trong `Update`, đọc dòng hiện tại. Chỉ kiểm "published + template còn sống" khi `default_template_version_id` **khác** giá trị đang lưu. Giữ nguyên id cũ là hợp lệ, vì đó là trạng thái mà hệ thống đã tự để trôi tới.
  - Trả thêm `status` của phiên bản trong `DefaultTemplateResponse`, hoặc đặt `default_template = null` khi template đã xoá. Web hiện nhãn theo status thật, kèm gợi ý "chọn phiên bản khác".
  - Thêm integration test: gắn v1, lưu trữ v1, rồi `Update` đổi tên phải thành công.

**M3 — Xoá khóa học không tuần tự với việc gắn lớp vào khóa, nên một lớp có thể bị gắn vào khóa đã xoá mềm. Bất biến "khóa có lớp thì không xoá được" bị phá.** *(CONFIRMED qua đọc code, chưa tái hiện)*
`apps/api/internal/features/courses/service.go:134-149` (`Delete`: `Get` → `LiveClassCount` → `SoftDelete`, không có transaction và không khoá); `apps/api/internal/features/classes/service.go:75` và `:373-380` (`resolveCourse` chạy ngoài `WithinTx` của create); `repository.go:355` (`FindCourse` không khoá), `:245/:259/:273/:344` (`Preload("Course")`, và `CourseRef` không có `deleted_at`)
- FK `(course_id, center_id)` chỉ lấy `FOR KEY SHARE` trên dòng khóa. Soft delete là `UPDATE deleted_at`, không đổi cột khoá, nên hai khoá này tương thích và DB không chặn gì. Đây là cùng lớp lỗi với M1 của phase 4, lỗi đã được sửa bằng `FOR UPDATE`/`FOR SHARE`.
- Kịch bản (READ COMMITTED):
  1. Giáo viên A tạo lớp chọn khóa TOAN-6, và `FindCourse` thấy khóa còn sống.
  2. Owner B bấm "Xoá khóa học". `LiveClassCount` trả 0 vì insert của A chưa commit, rồi B soft delete và commit.
  3. A insert lớp và commit.
- Hệ quả:
  - Lớp mang `course_id` của khóa đã xoá. `Preload("Course")` không lọc `deleted_at`, nên bảng lớp và header vẫn hiện chip "Khóa: TOAN-6", còn link `/courses/:id` trả 404 "Không tìm thấy khóa học".
  - Nếu sau đó owner tạo khóa mới cùng mã TOAN-6 (mã được dùng lại sau khi xoá mềm), hai chip giống hệt nhau trỏ tới hai khóa khác nhau.
  - Lớp không gỡ được qua UI, và qua API cũng không được vì M1.
- Fix:
  - `Delete` chạy trong `WithinTx`. Trong đó khoá dòng khóa bằng `FOR UPDATE`, rồi đếm lớp, rồi soft delete.
  - `CreateAnchored`/`Update` của classes gọi `FindCourse` **bên trong** transaction ghi, với `clause.Locking{Strength: "SHARE"}`. Nếu xoá đến trước, lần gắn chờ, đánh giá lại `deleted_at IS NULL` và trả 422. Nếu gắn đến trước, xoá chờ, `COUNT` thấy lớp và trả 409.
  - Hàng rào phụ: `Preload("Course", "deleted_at IS NULL")` để chip không trỏ tới khóa đã xoá.
  - Thêm integration test chạy song song xoá khóa và tạo lớp có `course_id`, rồi khẳng định không lớp sống nào trỏ tới khóa có `deleted_at`.

### Low

**L1 — Dialog tạo lớp chỉ điền sẵn đơn giá khi giá đang bằng 0. Đổi từ khóa A sang khóa B giữ im lặng giá của A.** *(CONFIRMED)*
`apps/web/src/features/roster/components/class-dialog.tsx:79-90` (`handleCoursePick`)
- Kịch bản: chọn khóa A (180.000đ), giá được điền 180.000. Người dùng nhận ra chọn nhầm và đổi sang khóa B (250.000đ). Giá vẫn là 180.000 vì không còn bằng 0, và lớp được tạo với giá sai. Chọn "Không gắn khóa học" cũng giữ giá của A.
- Comment nói giữ "giá giáo viên cố ý gõ", nhưng code không phân biệt giá do prefill với giá do người gõ. Test "keeps a price the teacher already typed" chỉ phủ trường hợp gõ tay.
- Fix: nhớ giá đã prefill lần trước. Nếu giá hiện tại vẫn bằng giá prefill trước đó (hoặc bằng 0) thì điền lại theo khóa mới. Thêm test vitest cho A → B.

**L2 — Hai lần lưu gói học phí đồng thời làm một bên nhận 500.** *(PLAUSIBLE)*
`apps/api/internal/features/courses/service.go:152-180` (`SetTuitionPacks`); `repository.go:455-469` (`ReplacePacks`)
- Không có khoá dòng khóa học. Hai transaction cùng `DELETE` rồi `INSERT` vị trí 1..n. Unique `(course_id, position)` là DEFERRABLE, nên xung đột nổ lúc COMMIT của bên sau, và lỗi thành 500 `INTERNAL`. Trường hợp khóa chưa có gói nào thì `DELETE` không chặn gì cả.
- Phase 3 và 4 đã giải bài này bằng `LockVersion`.
- Fix: trong `WithinTx`, đọc khóa bằng `FOR UPDATE` (thay cho `Get` hiện nằm ngoài tx), rồi mới `ReplacePacks`. Việc này cũng đóng khe hở ghi gói vào khóa vừa bị xoá.

**L3 — Biên API chấp nhận vài đầu vào mà web chặn.** *(CONFIRMED)*
`apps/api/internal/features/courses/dto.go:15-24`, `:28-32`; `service.go:185-213`
- `name: "   "` qua được `min=1`, rồi bị trim thành `""` và lưu vào DB. Tên gói học phí cũng vậy. Cột là `NOT NULL` nhưng không có CHECK độ dài.
- `PUT /courses/:id` bỏ trống `status` sẽ ghi `draft`. Theo nghĩa "full replace" thì đúng, nhưng một client chỉ muốn đổi tên sẽ vô tình hạ khóa đang active về draft.
- Fix: kiểm tra `name` sau khi trim trong service (422 `name`, `tuition_packs[i].name`). Với PUT, bắt buộc có `status` (`binding:"required,oneof=…"`) hoặc giữ nguyên status đang lưu khi trống.

**L4 — Số lớp đang mở của khóa tính trên toàn trung tâm, còn tab "Vận hành" chỉ liệt kê lớp người xem đọc được.** *(CONFIRMED — cần chốt ý đồ sản phẩm)*
`apps/api/internal/features/courses/repository.go:330-344` (subquery không theo `readScoped`); `apps/web/src/features/courses/pages/course-detail-page.tsx:424` (`useClassesList` theo read scope của classes)
- Một thành viên không có `classes.view_all` thấy "3 đang học" nhưng tab chỉ có 1 dòng. Lời từ chối xoá "đang có lớp gắn vào" cũng không chỉ ra được lớp nào.
- Số đếm chỉ lộ số lượng, không lộ tên lớp, nên rủi ro dữ liệu thấp.
- Fix (cần user chọn): giữ số toàn trung tâm và ghi chú "trên toàn trung tâm" ở UI, hoặc đếm theo cùng read scope với danh sách lớp.

**L5 — Invalidation cache chưa phủ hết.** *(CONFIRMED)*
`apps/web/src/features/courses/hooks/use-courses.ts:49-101`
- `useUpdateCourse` invalidate `classesKeys.lists()` nhưng không invalidate `classesKeys.details()`. Header chi tiết lớp giữ mã và tên khóa cũ cho tới khi hết staleTime.
- `useCreateCourse`/`useUpdateCourse`/`useDeleteCourse` chỉ invalidate trong `onSuccess`. Phase 4 đã chuyển sang `onSettled` (fix L6 của phase 4) để một 404/409 do người khác sửa trước cũng làm bảng tải lại.
- `useSetTuitionPacks` đúng (chỉ patch detail và danh sách).
- Fix: thêm `classesKeys.details()` vào update. Chuyển phần invalidate danh sách sang `onSettled`.

**L6 — Gate quyền trên web chưa khớp khoá API ở hai chỗ.** *(CONFIRMED)*
`apps/web/src/features/courses/pages/course-detail-page.tsx:302-303`; `apps/web/src/features/roster/components/class-detail-header.tsx` (chip `Link` tới `/courses/:id`)
- Tab chương trình mẫu gọi `useTemplatesList`/`useVersions`, tức là cần `library.read`, nhưng chỉ gate theo `courses.edit`. Người có `courses.edit` mà bị deny `library.read` thấy một picker trống, không có lời giải thích.
- Chip khóa ở header lớp luôn là link. Người bị deny `courses.read` bấm vào sẽ gặp "Không tải được khóa học".
- Fix: gate picker thêm `has("library.read")` và hiện dòng giải thích khi thiếu quyền. Render chip dạng text khi `!has("courses.read")`.

### Nit

- **Danh sách không phân trang.** `courses-page.tsx:72`, `listCourseOptions` và tab "Vận hành" đều cố định `per_page: 100` và không có phân trang. Trung tâm có hơn 100 khóa hoặc hơn 100 lớp mỗi khóa sẽ bị cắt im lặng. Đây là cùng kiểu với `library-page.tsx`, không phải hồi quy.
- **Giá không có trần.** `default_unit_price` và `price` không có trần ở API lẫn web. Giá trị lớn hơn 2^53 mất chính xác khi đi qua JSON/JS. Có thể đặt trần hợp lý, ví dụ 1e12 đồng.
- **Regex mã khóa lệch nhẹ.** Regex của web `^[A-Z0-9][A-Z0-9-]*$` chặt hơn `classcode.Valid` của server (`^[A-Z0-9-]{2,20}$`), vì web cấm dấu `-` ở đầu. Không sai, chỉ lệch nhẹ.
- **Cleanup e2e dừng giữa chừng.** `e2e/courses.spec.ts` (`afterEach`): nếu `expect` của bước xoá lớp fail thì bước xoá khóa bị bỏ qua. Nên bọc từng bước bằng `try` riêng, giống fix L7 của phase 4.
- **Khóa đã lưu trữ vẫn gắn được.** API cho gắn lớp **mới** vào khóa đã lưu trữ (`FindCourse` nhận mọi status), trong khi UI chỉ liệt kê khóa active. Nếu "archived = ngừng tuyển" thì create nên từ chối khóa archived, còn update vẫn giữ được khóa đang gắn.
- **Không có UI gắn lớp cũ vào khóa.** Chưa có đường UI để gắn hoặc đổi khóa cho lớp đã tồn tại, vì `ClassDialog` chỉ tạo lớp. Spec phase 5 chỉ yêu cầu dialog tạo, nên đây không phải lỗi phạm vi. Tuy vậy, lớp có từ trước migration sẽ không bao giờ có khóa nếu không có Phase 7 hay trang cài đặt lớp.

## Kiểm tra theo focus points

1. **Tenancy: đạt.**
   - `live`, `packs`, `LiveClassCount`, `FindTemplateVersion` và `classes.FindCourse` đều lọc `center_id`.
   - `Create`/`ReplacePacks` gán `CenterID` từ scope, không lấy từ body.
   - Các JOIN trong `summary` (`v`, `t`) và subquery đếm lớp không lọc lại `center_id`. Chúng vẫn an toàn nhờ FK composite `(default_template_version_id, center_id)` và `(course_id, center_id)`.
   - `resolveCourse` khi update dùng `class.CenterID`, không dùng trung tâm của người gọi.
   - Khóa khác trung tâm trả 422 ở create và update (unit test và integration `TestCourseLinkStaysInsideCenterAndCopiesPrice`). `GET` chéo trung tâm trả 404.
2. **Transaction: một phần.** Chép giá từ khóa đọc ngoài transaction. Điều này vô hại, vì giá chỉ là snapshot lúc tạo. Các khe hở thật là xoá khóa ↔ gắn lớp (M3) và lưu gói đồng thời (L2).
3. **N+1 và truy vấn không giới hạn: đạt.**
   - `List` tốn một `COUNT`, một `SELECT` với hai subquery tương quan cho mỗi dòng (tối đa 100 dòng, dùng `idx_classes_course` partial `deleted_at IS NULL`, khớp predicate) và một truy vấn gói theo `IN`.
   - `Get` tốn hai truy vấn.
   - `Preload("Course")` thêm một truy vấn cho mỗi trang lớp.
   - Subquery dùng chung `classes.PhasePredicate`, nên số đếm không lệch khỏi bộ lọc phase. Integration kiểm running=1, upcoming=1, và lớp ended không được đếm.
4. **Validation so với DB CHECK: đạt, trừ L3.**
   - `code` 2–20 khớp `VARCHAR(20)` qua `classcode.Valid`. `name` 200, `subject`/`level` 100, `status` oneof khớp CHECK.
   - `total_sessions` và `duration_min` > 0 khớp CHECK. Giá `>= 0` khớp CHECK.
   - Gói học phí: tên 100, số buổi 1–1000, giá ≥ 0, trần 20 gói trả 422 `tuition_packs`. `validation.Elements` trả key theo chỉ số.
5. **Route policy và audit: đạt.** Có 7 route: 2 GET `perm(courses.read, none())` và 5 route mutating `perm(courses.edit, req(action, "course", id))` với param đúng (create dùng `""`). Snapshot +7 và `actionSnapshot` +5 khớp `routespec.go`. `impliedKeys[courses.edit] = {courses.read}` đúng khuôn library. Không bump `CatalogVersion`, đúng như đã chốt. Test baseline đổi 59 → 60 vì `courses.read` là `def()`.
6. **Gate quyền trên web: phần lớn đạt.** Nav `perm: "courses.read"`. Nút tạo, sửa, ngừng tuyển, xoá và editor gói đều gate theo `has("courses.edit")`. `RESOURCE_LABELS.courses`, `OVERFLOW_LABELS` và `OVERFLOW_PATH_PREFIXES` đã thêm. Hai chỗ lệch xem ở L6.
7. **Cache React Query: phần lớn đạt.** Create, update, delete và archive invalidate danh sách khóa cùng `classesKeys.courseOptions()`. Update invalidate thêm danh sách lớp. `useCourseOptions` đặt `staleTime: 0` và chỉ bật khi dialog mở. Hai thiếu sót xem ở L5.
8. **Form/zod so với DTO: đạt, trừ M2.**
   - `courseFormSchema` mirror đủ giới hạn. Ô số trống chuyển thành `null`, và `blankToNull` áp cho text.
   - `courseSchema` khớp `CourseResponse`, gồm `default_template` nullable và `tuition_packs` luôn là mảng.
   - `classSchema.course` là nullable với default `null`, và `courseOptionSchema` bỏ các trường thừa.
   - `toClassCreateInput` bỏ `course_id` rỗng. Nhờ vậy create từ web không dính M1.
9. **Accessibility: đạt.** Các nhãn test dùng đều có thật: combobox "Khóa học", "Chương trình mẫu", "Phiên bản"; "Tìm khóa học"; tabs "Lọc theo trạng thái" và "Các mục của khóa học" với `role="tabpanel"` gắn đúng `idBase`; và các nhãn "Tên gói n", "Số buổi n", "Giá n", "Chuyển lên/xuống gói n", "Xoá gói n".
10. **`useApiFormErrors`: không hồi quy.** Hàm nhận `Pick<…, "getValues" | "setError">`, nên mọi caller cũ vẫn type-check (`tsc` OK). Nhánh 409 đổi từ `code === "CONFLICT"` sang `status === 409`. Đã soát từng caller có `conflictField`:
    - contact-dialog (`phone`): mọi 409 của contacts vốn đã là `CONFLICT`, nên hành vi không đổi.
    - score-set-editor-modal (`name`): 409 của grading đều là `apperror.Conflict`, nên hành vi không đổi.
    - template-dialog (`code`): create/update template chỉ có 409 `CODE_TAKEN`. Lỗi này trước rơi xuống footer, giờ hiện ở ô mã, tức là tốt hơn.
    - course-dialog (`code`): create/update chỉ có 409 `CODE_TAKEN`, còn `COURSE_ARCHIVED`/`COURSE_IN_USE` đi đường archive/delete riêng với toast.
    - 14 caller còn lại không truyền `conflictField`, nên không bị ảnh hưởng.
11. **Import vòng: đạt.** `roster` không import `@/features/courses`. `courses` import `classesKeys`, `useClassesList`, `phaseLabel` và `phaseVariant` qua barrel của roster.

## Test gaps

Nên thêm cùng các fix:
- Handler (classes): `PUT /classes/:id` với `"course_id": ""` trả 200 và `course: null`. `POST` với `"course_id": ""` tạo lớp không gắn khóa. (M1)
- Integration (courses): gắn v1, lưu trữ v1 (hoặc xoá template), rồi `Update` đổi tên phải thành công. (M2)
- Integration: xoá khóa chạy song song với tạo lớp có `course_id` không để lại lớp sống nào trỏ tới khóa đã xoá. (M3)
- Vitest: chọn khóa A rồi đổi sang khóa B cập nhật đơn giá. (L1)

Nice-to-have:
- Integration: hai `SetTuitionPacks` đồng thời đều trả 2xx và kết quả là một trong hai danh sách. (L2)

## Quyết định giữ nguyên (đã xác minh, không mở lại)

- FK `classes.course_id` và `courses.default_template_version_id` dùng `ON DELETE SET NULL (col)` thay vì CASCADE như chữ trong plan, để xoá khóa hay phiên bản không bao giờ kéo theo lớp hay khóa. Cú pháp danh sách cột đã được dùng ở 000005, 000007, 000022 và 000027.
- Xoá khóa còn lớp gắn trả 409 `COURSE_IN_USE`, và lưu trữ khóa không chạm lớp. Integration `TestArchiveKeepsClassesAndDeleteIsGuarded` phủ việc này.
- `UpdateClassRequest.course_id` là con trỏ: nil giữ nguyên, `""` gỡ, uuid gắn. Ngữ nghĩa này giữ nguyên. M1 chỉ sửa tầng binding để `""` tới được service.
- `CreateClassRequest.default_unit_price` chỉ được bỏ trống khi có `course_id`, lúc đó giá chép từ khóa. Thiếu cả hai trả 422 `default_unit_price`.
- `roster` không import `courses`. Lookup khóa cho dialog lớp nằm ở `classes-api.ts` của roster.
- Gói học phí là dữ liệu thuần, không nối vào billing (non-goal của plan).
- `courses.read` là `def()` có backfill, `courses.edit` là `optIn()`, và không bump `CatalogVersion`.

## Unresolved questions

- L4: "Lớp đang mở" của khóa nên đếm toàn trung tâm hay theo phạm vi đọc lớp của người xem?
- Nit "khóa đã lưu trữ vẫn gắn được": có cho tạo lớp **mới** trên khóa đã lưu trữ qua API không?
- M2: khi template mặc định đã bị xoá mềm, khóa nên tự hiện "chưa gắn" hay giữ tham chiếu kèm cảnh báo?

## Điểm tốt

- Subquery đếm lớp dùng lại `classes.PhasePredicate`/`Today`, nên hai bề mặt không thể lệch phase. Đây là cách làm tốt hơn là chép lại predicate.
- Migration có test backfill đầy đủ các nhánh: vai trò hệ thống, thành viên không vai trò, thành viên đã rời, dòng deny được giữ, trung tâm đã xoá, và down chỉ gỡ đúng dòng trong sổ.
- `Omit(clause.Associations)` ở create/update lớp ngăn GORM upsert ngược vào bảng `courses` qua `CourseRef`.

## Disposition

Đã xử lý ngày 2026-09-24. Mọi mục "sửa" đều có test đỏ trước khi đổi code.

| Mục | Xử lý | Ghi chú |
|---|---|---|
| M1 | Sửa | Bỏ tag `uuid` trên hai `CourseID`; `resolveCourse` tự parse, sai định dạng → 422 `course_id`. `TestCourseAttachmentOverHTTP` phủ `""` (gỡ), `"x"` (422) và `""` khi tạo. |
| M2 | Sửa | `Update` đọc dòng hiện tại, chỉ kiểm tra "đã phát hành trong trung tâm" khi id khác id đang lưu; status trống giữ status cũ. `DefaultTemplateResponse.status` trả trạng thái thật; `summary` LEFT JOIN `program_templates … AND deleted_at IS NULL` nên template đã xoá → `default_template: null`, id vẫn giữ. Web hiện nhãn theo status thật, gợi ý chọn lại khi đã lưu trữ / đã xoá. Unit `TestUpdateKeepsUnchangedTemplateAndStatus`, integration `TestRetiredDefaultTemplateDoesNotBlockEdits`. Chọn phương án "giữ tham chiếu kèm cảnh báo" cho câu hỏi mở: API không tự ghi đè dữ liệu người dùng. |
| M3 | Sửa | `courses.Delete` chạy trong `WithinTx`: `Lock` (FOR UPDATE) → `LiveClassCount` → `SoftDelete`. `classes.FindCourse` thêm FOR SHARE và được gọi trong tx ghi của `CreateAnchored`/`Update`; `Preload("Course", "deleted_at IS NULL")` ở 4 chỗ. Integration `TestDeleteSerialisesWithClassAttach` chạy 8 vòng race, khẳng định không lớp sống nào trỏ vào khóa đã xoá. |
| L1 | Sửa | Dialog lớp nhớ giá vừa prefill; đổi khóa ghi đè khi giá hiện tại là 0 hoặc đúng bằng giá prefill trước, giữ nguyên khi người dùng đã gõ tay. Vitest A→B→gõ tay→A. |
| L2 | Sửa | `SetTuitionPacks` khoá dòng khóa (FOR UPDATE) trong tx trước `ReplacePacks`, hai lần ghi đồng thời xếp hàng thay vì 500 ở COMMIT. |
| L3 | Sửa | `name` trắng → 422 `name`; tên gói trắng → 422 `"<i>.name"`; PUT thiếu status giữ status đang lưu. Unit `TestBlankNamesAreRejected`. |
| L4 | Giữ quyết định | Bộ đếm lớp là sự thật danh mục, đếm toàn trung tâm; tab Lớp học vẫn theo phạm vi đọc của người xem. UI ghi rõ "Lớp đang học (toàn trung tâm)" / "Lớp sắp mở (toàn trung tâm)" để hai con số không bị hiểu là cùng một tập. |
| L5 | Sửa | `useUpdateCourse` invalidate thêm `classesKeys.details()`; create/update/delete chuyển invalidate danh sách sang `onSettled`. |
| L6 | Sửa | Tab Chương trình mẫu chỉ hiện picker khi có `library.read`, thiếu thì hiện dòng giải thích và vẫn cho "Bỏ chương trình mẫu"; chip khóa trên header lớp thành text thường khi thiếu `courses.read`. Vitest cho cả hai. |
| Nit e2e | Sửa | `afterEach` gom lỗi từng bước rồi mới assert, bước sau vẫn chạy khi bước trước lỗi. |
| Nit khóa lưu trữ | Sửa | Tạo lớp hoặc đổi sang khóa `archived` → 422 `course_id` "Khóa học đã ngừng tuyển…"; gửi lại đúng id đang gắn vẫn lưu được. `CourseRef.Status` + hằng `courseStatusArchived` trong classes (classes không import courses). Unit `TestArchivedCourseTakesNoNewClasses`. |
| Nit phân trang / trần giá / regex mã / không có UI gỡ khóa | Bỏ qua | Ngoài phạm vi phase: `per_page` 100 đã đủ cho danh mục một trung tâm; trần giá theo CHECK ≥ 0 giống lớp; regex mã giữ đồng bộ với mã lớp; gỡ khóa khỏi lớp làm qua trang cài đặt lớp (`course_id: ""`) như plan. |

Kiểm chứng sau khi sửa: `make test-api-unit`, `make scopelint`, `make api-docs`, `make lint`, `make test-web`
(1006 pass) xanh; integration `courses`/`classes`/`migrations` với `-p 1` xanh; e2e `courses.spec.ts` 1/1 trên
stack cô lập.
