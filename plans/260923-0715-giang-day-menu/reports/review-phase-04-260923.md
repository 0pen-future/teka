# Review — Phase 4: Kho học liệu (Học liệu, Bài tập, Trường nhật ký, Bộ điểm)

Ngày: 2026-09-24 · Branch `feat/giang-day-menu` · Phạm vi: diff chưa commit và các file untracked của phase (bỏ qua `docs/diagrams/`, `.pi/` và hunk `docs/architecture.md` theo yêu cầu).

## Verdict: **SHIP WITH FIXES**

Không có lỗi Critical hay High. Tenancy đạt: mọi truy vấn mới đều lọc `center_id`, FK composite chặn nối chéo trung tâm, và `GET /library/versions/:vid` trả 404 với trung tâm khác. Mọi ghi nội dung phiên bản (gắn học liệu/bài tập, trường nhật ký, bộ điểm) đều đi qua `lessonWrite`, tức là khoá dòng phiên bản bằng `LockVersion` rồi mới ghi. Vì vậy chúng tuần tự với Publish đúng như quyết định của phase 3. Route manifest, audit và snapshot khớp nhau. Swagger được sinh lại thật.

Có ba lỗi Medium:
- **Race xoá mục ↔ gắn mục.** Kiểm tra "đang được gắn" không khoá gì, nên một học liệu có thể bị xoá mềm đúng lúc được gắn. Buổi học sau đó kẹt với lỗi 422 mà UI không gỡ được.
- **Học liệu/bài tập bị khoá vĩnh viễn.** Chúng không xoá được khi link nằm trong template đã xoá mềm. Thông báo lỗi lại bảo người dùng làm việc không thể làm.
- **`url` không kiểm tra scheme** ở biên API, trong khi dữ liệu này sẽ tới học viên.

Phần còn lại là Low/Nit.

## Verification đã chạy

| Lệnh | Kết quả |
|---|---|
| `go build ./...` | OK |
| `go vet` library, validation, routespec, server, migrations (+ `-tags integration` cho library, migrations) | OK |
| `gofmt -l` library, validation, routespec, server, audit, migrations | sạch |
| `go test ./internal/features/library/ ./internal/shared/validation/ ./internal/shared/routespec/ ./internal/server/ ./internal/features/audit/` | tất cả `ok` |
| `golangci-lint run` library, validation, routespec | 0 issues |
| `go tool swag init -g cmd/api/main.go --parseInternal` ra thư mục tạm rồi diff với `apps/api/docs` | `swagger.json`, `swagger.yaml` giống hệt. `docs.go` chỉ khác tên package. So sánh JSON với HEAD: 121→130 path, 166→180 definition, không path/definition cũ nào bị xoá hay đổi. Diff lớn trong git chỉ do sắp xếp lại. |
| `npx vitest run src/features/library` | 3 files, 52 passed |
| `npx tsc -b --noEmit` (web) | OK |
| `npx eslint src/features/library e2e/library.spec.ts` | 0 error, 0 warning |

Đọc, không chạy: `integration_test.go`, `items_test.go`, `TestLibraryItemsSchemaInvariants` trong `migrations_test.go`, và `e2e/library.spec.ts`. Controller đã chạy xanh bộ integration và Playwright trên stack cô lập.

Ghi chú số đếm: tóm tắt của controller nói có 21 route mới. Thực tế là **15** route: 5 GET `library.read` cùng 10 route mutating `library.edit`. `routespec.go`, snapshot (+15) và audit (+10) đều đúng với con số 15. Spec ghi "+12 snapshot" là ước lượng thiếu, không phải lỗi.

## Findings

### Critical
Không có.

### High
Không có.

### Medium

**M1 — Xoá học liệu/bài tập không tuần tự với lần gắn chúng vào buổi, nên một mục đã xoá mềm vẫn có thể còn được gắn. Khối chọn của buổi đó sau đó kẹt 422 vĩnh viễn.** *(CONFIRMED qua đọc code, chưa tái hiện)*
`apps/api/internal/features/library/service.go:654-670` (`DeleteMaterial`), `:729-745` (`DeleteExercise`), `:751-790` và `:792-831` (`SetLessonMaterials`/`SetLessonExercises`); `repository.go:457` (`FindMaterials`), `:483` (`MaterialLinkCount`)
- `DeleteMaterial` chạy `GetMaterial` không khoá, rồi `COUNT` link, rồi `UPDATE deleted_at`. `SetLessonMaterials` khoá dòng *phiên bản*, không khoá dòng *học liệu*. Nó gọi `FindMaterials` không khoá rồi `INSERT` link.
- FK `(material_id, center_id)` chỉ lấy `FOR KEY SHARE` trên dòng học liệu. Lệnh soft delete không đổi cột khoá nên chỉ lấy `FOR NO KEY UPDATE`. Hai khoá này tương thích, vì vậy DB không chặn gì.
- Kịch bản (READ COMMITTED):
  1. Người soạn A lưu học liệu của buổi 3, và `FindMaterials` thấy "Slide" còn sống.
  2. Người B bấm "Xoá học liệu Slide". `COUNT` trả 0 vì insert của A chưa commit. `deleted_at` được ghi và B commit.
  3. A insert link rồi commit.
  4. Kết quả là "Slide" đã xoá mềm nhưng vẫn gắn vào buổi 3.
- Hệ quả thấy được:
  - `ListLessonMaterials` đọc "as-is" nên buổi vẫn hiện Slide. Comment ở `repository.go:574` nói "a linked material can never be deleted", và comment này giờ sai.
  - `MaterialsPicker` (`apps/web/src/features/library/components/lesson-attachments.tsx:145-152, :176-210`) đưa Slide vào `selected`, nhưng danh mục chỉ liệt kê mục còn sống. Vì vậy không có checkbox nào để bỏ chọn nó.
  - Mọi lần "Lưu học liệu" sau đó gửi lại id của Slide và nhận 422 "Có học liệu không tồn tại". Khối này kẹt cho tới khi có người gọi API trực tiếp.
  - `copyVersionContent` còn chép link hỏng này sang bản nháp mới.
- Fix:
  - Khoá dòng học liệu ở cả hai phía. `DeleteMaterial` đọc bằng `FOR UPDATE` (`clause.Locking{Strength: "UPDATE"}`). `FindMaterials` trong `SetLessonMaterials` dùng `FOR SHARE`. Làm tương tự cho exercise.
  - Nếu delete đến trước, `FOR SHARE` của lần gắn sẽ chờ. Sau khi delete commit, PostgreSQL đánh giá lại `deleted_at IS NULL` trên bản dòng mới, nên lần gắn trả 422.
  - Nếu lần gắn đến trước, delete chờ. `COUNT` chạy sau đó là một câu lệnh mới, thấy link đã commit, và trả 409.
  - Có thể thêm hàng rào phía web: picker hiện cả các mục đang gắn nhưng không có trong danh mục, để người soạn bỏ chọn được. Hàng rào này cũng giúp khi mục nằm ngoài 100 dòng đầu.
  - Thêm integration test chạy song song delete và gắn, rồi khẳng định không còn link nào trỏ tới dòng có `deleted_at`.

**M2 — Link trong template đã xoá mềm (và trong phiên bản đã phát hành/lưu trữ) làm học liệu/bài tập không bao giờ xoá được. Thông báo 409 lại bảo người dùng "gỡ khỏi các buổi", điều mà họ không làm được.** *(CONFIRMED)*
`apps/api/internal/features/library/repository.go:483` (`MaterialLinkCount` đếm mọi link), `:265` (`LockVersion` trả 404 khi template đã xoá); `errors.go:63, :68`; `apps/web/src/features/library/components/items-tabs.tsx:268`; `apps/web/e2e/library.spec.ts` (cleanup `afterEach`)
- Theo thiết kế, xoá template là xoá mềm và giữ link. Nhưng mọi đường ghi vào phiên bản của template đã xoá đều trả 404, vì `LockVersion` join `t.deleted_at IS NULL` theo quyết định L1 của phase 3. Vì vậy link trong template đã xoá không gỡ được bằng bất kỳ API nào, và `MaterialLinkCount` vẫn đếm chúng. Kết quả là học liệu bị khoá vĩnh viễn khỏi thao tác xoá.
- Link trong phiên bản published/archived cũng không gỡ được, vì các phiên bản đó bất biến. Chặn xoá trong trường hợp này là hợp lý, vì lớp đang đọc chúng. Nhưng thông báo "hãy gỡ khỏi các buổi trước khi xoá" gây hiểu nhầm, và UI không cho biết mục đang được dùng ở đâu.
- Kịch bản:
  1. Owner gắn "Slide" vào v1 và phát hành.
  2. Sau đó owner xoá chương trình.
  3. Xoá "Slide" trả 409 mãi mãi, và không có màn hình nào dẫn tới chỗ đang dùng nó.
- Chính e2e cũng dính lỗi này. Test ghi "Detach so the afterEach can clear the catalog; the template's soft delete keeps links". Nếu test fail trước bước gỡ, `afterEach` xoá template trước rồi mới xoá mục. Hai dòng `Slide E2E-…` và `Bài tập E2E-…` khi đó kẹt lại vĩnh viễn trong DB e2e.
- Fix (cần user chọn):
  - (a) `MaterialLinkCount`/`ExerciseLinkCount` chỉ đếm link thuộc template còn sống, bằng cách join `template_lessons → program_template_versions → program_templates` với `deleted_at IS NULL`. Đọc vẫn an toàn vì `ListLessonMaterials` đọc dòng as-is. Lớp gắn phiên bản của template đã xoá vẫn thấy học liệu (đã xoá mềm), nhưng học liệu đó biến mất khỏi danh mục.
  - (b) Giữ nguyên chặn, nhưng trả kèm danh sách template/phiên bản đang dùng mục, và đổi thông báo theo trạng thái. Ví dụ: "đang dùng trong phiên bản đã phát hành, không thể xoá".
  - Tối thiểu nên sửa câu thông báo, và đảo thứ tự cleanup e2e: gỡ link trước khi xoá template, hoặc bỏ qua bước xoá template khi chưa gỡ link.

**M3 — `url` của học liệu không được kiểm tra scheme ở biên API. Bất kỳ chuỗi nào dài tới 2000 ký tự cũng được lưu, rồi render thành `href` và sẽ được chia sẻ tới học viên.** *(CONFIRMED phần lưu; PLAUSIBLE phần khai thác)*
`apps/api/internal/features/library/dto.go:127` (`URL *string binding:"omitempty,max=2000"`); `apps/web/src/features/library/schemas/library-schemas.ts` (`materialFormSchema.url: optionalText(2000)`); `apps/web/src/features/library/components/items-tabs.tsx:216-224`, `lesson-attachments.tsx:319-327`
- Server nhận `javascript:alert(document.cookie)`, `data:text/html,…` và `file:///…`. Web đặt thẳng giá trị vào `<a href>` ở bảng danh mục cho mọi người có `library.read`, và ở chế độ chỉ đọc của buổi học.
- React 19.2 chặn riêng `javascript:` trong `href`, nên client web hiện tại không bị XSS trực tiếp. Tuy vậy, cờ `shared_with_students` tồn tại chính là để URL này đi tới học viên ở các phase sau, qua các bề mặt khác như tin Zalo, cổng học viên hay app. Một người có `library.edit` (quyền optIn, không nhất thiết là owner) có thể gieo link độc cho toàn trung tâm.
- Fix: validate ở API, vì đây là biên hệ thống. Có thể dùng `binding:"omitempty,max=2000,url"` kèm kiểm tra scheme `http`/`https` trong service (validator `url` chấp nhận mọi scheme). Hoặc thêm validator `httpurl` dùng chung trong `shared/validation`. Phía web dùng `z.url({ protocol: /^https?$/ })` cho trường `url`. Thêm unit test cho `javascript:` → 422.

### Low

**L1 — Buổi học bị xoá giữa lúc đọc buổi và lúc khoá phiên bản khiến lần gắn học liệu trả 500 thay vì 404.** *(CONFIRMED)*
`apps/api/internal/features/library/service.go:833-841` (`lessonLinkWrite`); `repository.go:584` (`ReplaceLessonMaterials`)
- `lessonLinkWrite` đọc buổi học *trước* `LockVersion`. Nếu `DeleteLesson` (đang giữ khoá phiên bản) xoá buổi và commit, lần gắn lấy được khoá, rồi `INSERT` link vi phạm FK `(lesson_id, center_id)`. `TranslateError` biến lỗi thành `gorm.ErrForeignKeyViolated`, và `apperror.From` trả 500 `INTERNAL`. Với body `[]`, API lại trả 200 cho một buổi không còn tồn tại.
- Fix: sau `LockVersion`, đọc lại buổi học (`GetLesson`) ngay trong closure của `lessonWrite` và trả 404 nếu không còn. Cách khác là map `ErrForeignKeyViolated` thành 404 `template lesson`.

**L2 — Swagger của `GET`/`PUT /library/lessons/{lid}` vẫn khai báo `LessonResponse`, trong khi API giờ trả `LessonDetailResponse` có `materials`/`exercises`.** *(CONFIRMED)*
`apps/api/internal/features/library/handler.go:455, :487`
- Docs được sinh lại đúng, nhưng annotation nguồn đã cũ. Client sinh từ OpenAPI sẽ thiếu hai mảng này. Đây là hợp đồng mà Phase 7 dự định đọc.
- Fix: đổi hai dòng `@Success` thành `data=LessonDetailResponse`, rồi chạy `make api-docs`.

**L3 — Body gắn học liệu/bài tập không có giới hạn số phần tử.** *(PLAUSIBLE)*
`apps/api/internal/features/library/service.go:751-760`, `:792-801`
- Trường nhật ký có trần 30 và bộ điểm có trần 20. Gắn mục thì không có trần. Một body vài chục nghìn id làm `IN ?` và batch insert vượt giới hạn 65535 tham số của PostgreSQL, và lỗi trả về là 500. Chỉ người có `library.edit` làm được việc này.
- Fix: đặt trần, ví dụ 100 mục mỗi buổi, rồi trả 422 với key `materials`/`exercises`, cùng khuôn với `maxLogFields`.

**L4 — `copyVersionContent` ghi link theo từng buổi (N+1) trong lúc giữ khoá phiên bản nguồn/đích.** *(CONFIRMED)*
`apps/api/internal/features/library/service.go:173-230`
- Mỗi buổi tốn một `DELETE` và một `INSERT` cho học liệu, rồi thêm một cặp nữa cho bài tập. Thêm vào đó là vòng lặp O(buổi × link) trong bộ nhớ. Một chương trình 40 buổi tốn khoảng 160 câu lệnh trong một transaction. Tính đúng không sai, nhưng transaction bị kéo dài không cần thiết.
- Fix: gom toàn bộ link của mọi buổi thành một slice (ánh xạ `sourceLessonID → copies[i].ID`), rồi `Create` một lần cho mỗi bảng. Bỏ bước `DELETE`, vì bản nháp mới chưa có link nào.

**L5 — Sửa học liệu/bài tập làm đổi nội dung mà các phiên bản đã phát hành trả về.** *(CONFIRMED — quyết định thiết kế cần chốt)*
`apps/api/internal/features/library/service.go:639` (comment "Lessons linking it see the change at once: the link is by reference")
- Phase 3 coi phiên bản published là bất biến để lớp áp dụng ổn định (Phase 7). Ở đây link trỏ theo tham chiếu. Vì vậy đổi `url`/`title` của một học liệu sẽ đổi luôn những gì mọi lớp gắn phiên bản đã phát hành nhìn thấy, và việc này không cần `library.publish`.
- Trade-off: tham chiếu giúp sửa link hỏng một lần cho mọi lớp, và đơn giản. Chép theo phiên bản (snapshot title/url vào bảng nối) giữ đúng bất biến, nhưng tốn dữ liệu, và link hỏng phải sửa lại trong bản nháp mới.
- Đề xuất: giữ tham chiếu và ghi quyết định vào spec/Phase 7. Hoặc chặn sửa `url` của mục đang gắn vào phiên bản không phải nháp. Việc này cần user quyết.

**L6 — Mutation danh mục chỉ invalidate trong `onSuccess`, nên khi lỗi (404 vì người khác đã xoá, 409) bảng vẫn giữ dòng cũ.** *(CONFIRMED)*
`apps/web/src/features/library/hooks/use-library.ts` (`useCatalogWrite`)
- Người dùng bấm "Xoá" một học liệu mà người khác vừa xoá sẽ nhận toast lỗi, nhưng dòng vẫn còn trong bảng cho tới khi hết staleTime. Các mutation khác trong feature đã chuyển sang `onSettled` theo đúng lý do này (fix L6 của phase 3).
- Fix: chuyển `useCatalogWrite` sang `onSettled`.

**L7 — Cleanup e2e dừng hẳn nếu bước xoá template lỗi, và xoá template trước khi gỡ link.** *(CONFIRMED)*
`apps/web/e2e/library.spec.ts` (`test.afterEach`)
- `deleteRunTemplate` và hai lần `deleteRunItem` chạy tuần tự trong một `try`. Một ngoại lệ ở bước đầu bỏ qua hai bước sau. Thứ tự này cũng gây ra rò rỉ vĩnh viễn đã mô tả ở M2.
- Fix: xoá mục trước, hoặc bọc từng bước bằng `try` riêng. Hoặc gọi API gỡ link (`PUT …/materials []`) trong cleanup trước khi xoá template.

### Nit

- `apps/web/src/features/library/lib/row-errors.ts:27`: lỗi không theo chỉ số được hiện kèm key thô, ví dụ "log_fields: tối đa 30 trường" hoặc "material_id: bị trùng". Nên chỉ hiện `message`, hoặc map key sang nhãn tiếng Việt.
- `validation.Elements` (`apps/api/internal/shared/validation/validation.go:92, :110`): lỗi trong phần tử con qua `dive` (ví dụ tuỳ chọn thứ 4 dài hơn 100 ký tự) có key dạng `"0.options[3]"`, vì `fe.Field()` giữ chỉ số con *(PLAUSIBLE)*. Regex web `^(\d+)\.(\w+)$` không khớp key này, nên lỗi rơi vào phần chung. Web đã tự chặn trước nên chỉ client khác gặp. Có thể cắt `[n]` trong `fieldName`, hoặc để regex web nhận `\w+(\[\d+\])?`.
- `LogFieldsEditor`/`ScoreSetEditor`: nút lưu không gate theo `dirty`, khác với khối gắn học liệu. Tuỳ chọn của trường "select" tách bằng dấu phẩy, nên không nhập được tuỳ chọn chứa dấu phẩy. UI cũng không chặn trước trần 30/20 dòng.
- `template-detail-page.tsx` (`GradingPanel`): editor được key theo nội dung server. Một lần refetch nền, ví dụ khi focus lại cửa sổ, sẽ remount editor và xoá bản sửa chưa lưu nếu người khác vừa lưu. Đây là hành vi có chủ đích ("or a concurrent change"), nhưng nên có một toast báo.
- `Service.GetVersion` (`service.go:569`) đọc phiên bản, trường nhật ký, buổi và link bằng các câu lệnh riêng, không cùng snapshot. Với bản nháp đang bị sửa, payload có thể lệch nhau trong chốc lát. Với bản published thì vô hại.
- `MaterialRequest` cho phép `kind = link` mà không có `url`. Nếu "link" nghĩa là phải có đường dẫn thì nên bắt buộc có `url` khi `kind = link`.

## Kiểm tra theo focus points

1. **Tenancy: đạt.** Các scope `materials`, `exercises`, `lessonMaterials`, `lessonExercises` và `logFields` đều lọc `center_id = sc.CenterID`. `SetScoreSet` đi qua `versions`. Các JOIN `library_materials m`/`library_exercises e` không lọc lại `center_id`, nhưng FK composite `(material_id, center_id)` đảm bảo cùng trung tâm. Service kiểm material/exercise cùng trung tâm bằng `FindMaterials`/`FindExercises` (mục khác trung tâm trả 422), và FK là hàng rào thứ hai (`TestLibraryItemsSchemaInvariants`). Integration `TestAttachmentsRespectCenterAndVersionLock` chứng minh `GetVersion` với trung tâm khác trả 404, và người ngoài ghi vào buổi của trung tâm khác cũng nhận 404.
2. **Đồng thời/bất biến: đạt với phiên bản, thiếu với mục (M1).** Cả bốn đường ghi mới đều đi qua `lessonWrite`. `WithinTx` lồng nhau dùng lại transaction bao ngoài (`apps/api/internal/database/tx.go:23-26`), nên `lessonLinkWrite` không mở tx thứ hai. Ghi đè toàn bộ là idempotent (integration kiểm body lặp lại cho kết quả bằng nhau), vị trí đánh lại `1..n`, và id trùng trong body trả 422 trước khi mở tx. Khoảng hở còn lại là L1.
3. **Ngữ nghĩa xoá: một phần (M2).** Mục đã xoá mềm biến mất khỏi danh sách, khỏi picker và khỏi `GetMaterial`, nhưng vẫn đọc được qua link. Việc template xoá mềm giữ link *nhất quán* với 409 in-use theo nghĩa hẹp, nhưng làm mục không bao giờ xoá được.
4. **Validation: đạt, trừ M3.** Các giới hạn DTO khớp DB: title 200, label 100, difficulty 1..5, tags 20×50, options 20×100. `cleanStrings` trim, bỏ trống và bỏ trùng. Tuỳ chọn cho kind khác select được lưu `[]`. Khoá điểm đi qua regex, được kiểm trùng, và lỗi dùng key chỉ số. `ScoreSet.Value` ghi `[]` thay vì NULL. `validation.Elements` trả nil cho mảng rỗng/nil (có unit test), và body `null` tương đương `[]`, tức là xoá hết. Việc này đúng với mô tả "[] clears".
5. **Routespec/audit/snapshot: đạt.** Có 15 route: 5 GET `perm(library.read, none())` và 10 route mutating có `req(action, entity, id)` đúng param (`:vid`, `:lid`, `:id`, còn create dùng `""`). Snapshot và `actionSnapshot` khớp. Swagger được sinh lại, không sửa tay. Riêng annotation lesson còn cũ (L2).
6. **Web: phần lớn đạt.**
   - Invalidation: ghi link invalidate chi tiết buổi, chi tiết phiên bản và danh sách phiên bản trong `onSettled`. Ghi nhật ký/điểm invalidate chi tiết và danh sách phiên bản. Ghi danh mục invalidate mọi `details()`, nhưng chỉ khi thành công (L6).
   - Mỗi khối được key theo dữ liệu server nên lưu khối này không reset khối kia.
   - Tên a11y đầy đủ cho checkbox, nút di chuyển và nút xoá, và panel có `role="tabpanel"` gắn đúng `idBase`.
   - Chế độ chỉ đọc gate theo `library.edit` cùng trạng thái draft. 409 `VERSION_LOCKED` refetch danh sách phiên bản và tự chuyển sang chế độ chỉ đọc.
   - Các trường hợp biên của `rowErrorsFromApi` xem ở phần Nit. Độ bền của cleanup e2e xem ở L7.

## Test gaps

Nên thêm cùng các fix:
- Integration: `DeleteMaterial` chạy song song với `SetLessonMaterials` không để lại link nào tới dòng đã xoá mềm (M1).
- Service/integration: xoá mục chỉ còn được gắn trong template đã xoá mềm, theo phương án user chọn (M2).
- Unit handler: `url = "javascript:…"` trả 422 (M3).
- Integration: gắn học liệu vào buổi vừa bị xoá trả 404, không phải 500 (L1).

Nice-to-have:
- Vitest: picker hiện và cho bỏ chọn mục đang gắn nhưng không có trong danh mục (M1, phía web).

## Quyết định giữ nguyên

- Tất cả quyết định trong spec được giữ:
  - Học liệu là link, không upload.
  - Link được ghi đè toàn bộ bằng `PUT`.
  - `log_fields`/`score_set` gắn ở phiên bản và chịu cùng khoá publish.
  - Không thêm key quyền mới.
  - Bộ điểm lưu JSONB.
- Các quyết định phase 3 cũng được giữ: `LockVersion` trong `WithinTx`, 404 khi ghi vào template đã xoá, `likeq` có escape (`ListMaterials`/`ListExercises` dùng `ILIKE ? ESCAPE '\'`), và ánh xạ `ErrDuplicatedKey` thành 409.
- Id trường nhật ký đổi sau mỗi lần lưu, vì ghi đè toàn bộ tạo dòng mới. Việc này chỉ xảy ra trên bản nháp nên không ảnh hưởng lớp đã gắn phiên bản published. Phase 7 nên tham chiếu trường nhật ký theo `(version_id, id)` của bản published, không theo id của bản nháp.

## Unresolved questions

- M2: khi template đã xoá mềm, link của nó có còn tính là "đang dùng" không? Chọn (a) bỏ qua link của template đã xoá, hay (b) giữ chặn và chỉ ra chỗ đang dùng?
- L5: sửa học liệu có được phép đổi nội dung mà phiên bản đã phát hành nhìn thấy không?

## Điểm tốt

- Mọi đường ghi mới đều tái sử dụng `lessonWrite` thay vì tự khoá lại, nên bất biến publish của phase 3 phủ luôn nội dung mới mà không có code trùng lặp.
- `validation.Elements` là helper dùng chung nhỏ, có test, và giải đúng khoảng hở thật của gin (`SliceValidationError` làm mất chỉ số).
- `TestLibraryItemsSchemaInvariants` kiểm cả FK chéo trung tâm theo hai chiều, unique DEFERRABLE khi hoán đổi, và cascade khi xoá cứng template.

## Disposition (2026-09-24, sau khi sửa)

| Finding | Xử lý | Bằng chứng |
|---|---|---|
| M1 | **Sửa.** Xoá học liệu/bài tập giờ chạy trong `WithinTx`: `LockMaterial`/`LockExercise` (`FOR UPDATE`) → đếm usage → xoá mềm. `FindMaterials`/`FindExercises` (dùng khi gắn vào buổi) khoá `FOR SHARE`, nên gắn và xoá tuần tự trên dòng mục: gắn giữ SHARE → xoá chờ rồi 409; xoá giữ UPDATE → gắn chờ rồi 422. Web: `pickerRows` giữ dòng đã gắn nhưng không còn trong trang danh mục để người dùng bỏ tick được. | `TestDeleteAndAttachSerialiseOnTheItemRow` (integration); vitest "keeps an attached material the catalog page no longer lists so it can be unticked" |
| M2 | **Sửa theo phương án (a), có điều chỉnh.** `MaterialUsage`/`ExerciseUsage` chỉ đếm link của template **còn sống** (`program_templates.deleted_at IS NULL`), tách `Draft`/`Released`. Link nháp → 409 "gỡ khỏi các buổi" (người dùng làm được); link trong phiên bản đã phát hành/lưu trữ → 409 với thông báo riêng "đang dùng trong phiên bản đã phát hành hoặc lưu trữ" (không hứa gỡ). Đây là thay đổi so với ghi chú ban đầu "xoá mềm template không gỡ liên kết nên học liệu vẫn 409": link vẫn được giữ trong DB, nhưng không còn tính là đang dùng. **Phương án thay thế (b)** — giữ chặn kể cả template đã xoá và trả về danh sách chỗ đang dùng — bị bỏ vì người dùng không mở được template đã xoá để gỡ, nên mục sẽ kẹt vĩnh viễn. | `TestLinksOfDeletedTemplatesDoNotBlockItemDeletes`; `TestLinkedItemsCannotBeDeleted` (unit, fake repo) |
| M3 | **Sửa.** API: `materialURL` parse bằng `net/url`, chỉ nhận `http`/`https` có host, ngược lại 422 key `url`. Web: `optionalHttpUrl` trong zod chặn trước khi gọi API. | `TestMaterialURLMustBeHTTP`; handler case "material script url"; vitest "refuses a material link that is not http(s) before calling the API" |
| L1 | **Sửa.** `lessonLinkWrite` đọc lại buổi bên trong closure đã khoá phiên bản → 404 thay vì 500. | `TestAttachToLessonDeletedMeanwhileIsNotFound` |
| L2 | **Sửa.** `@Success` của `GET`/`PUT /library/lessons/{lid}` → `LessonDetailResponse`; `make api-docs` sinh lại. | diff `apps/api/docs/*` |
| L3 | **Sửa.** Trần `maxLessonItems = 100` cho body gắn học liệu/bài tập, 422 key `materials`/`exercises`. | cap case trong `TestLessonMaterialsValidation`/`TestLessonExercisesValidation` |
| L4 | **Sửa.** `copyVersionContent` gom link của mọi buổi rồi ghi một lần qua `CreateLessonMaterials`/`CreateLessonExercises`; không còn DELETE + N+1 trong lúc giữ khoá. | unit tests copy version vẫn xanh; integration library xanh |
| L5 | **Giữ tham chiếu (không sửa).** Học liệu/bài tập là danh mục dùng chung; phiên bản published tham chiếu theo id, nên sửa nội dung danh mục hiện ra ở mọi phiên bản. Đây là hành vi chủ đích của spec (học liệu là link, sửa một chỗ áp dụng mọi nơi). Nếu sau này cần đóng băng nội dung theo phiên bản thì snapshot khi publish, tách phase riêng. | quyết định ghi ở Completion notes phase 4 |
| L6 | **Sửa.** `useCatalogWrite` invalidate trong `onSettled` để lỗi 404/409 cũng làm bảng tải lại. | `use-library.ts` |
| L7 | **Sửa.** `afterEach` e2e chạy từng bước dọn trong `try` riêng, gom lỗi rồi ném lỗi đầu tiên; xoá template trước vì link của template đã xoá không còn giữ mục. | `e2e/library.spec.ts`, 2/2 trên stack cô lập |
| Nit `row-errors` | **Sửa.** Lỗi chung chỉ hiện `message`, không kèm key thô. | `row-errors.ts` |
| Nit `fieldName` | **Sửa.** `validation.fieldName` cắt hậu tố `[n]`, key thành `"<i>.<field>"`. | `elements_test.go` case `opts` |
| Nit `kind=link` không bắt buộc `url` | **Giữ.** Spec không ràng buộc; học liệu "link" chưa có đường dẫn vẫn hợp lệ như bản nháp nội dung. | — |
| Nit editor (dirty gate, dấu phẩy trong tuỳ chọn, trần dòng, toast remount, snapshot `GetVersion`) | **Giữ, ghi nhận.** Ngoài phạm vi phase; không phải lỗi dữ liệu. | — |

Gate sau khi sửa: `go test` library + validation, integration library (`-p 1`, 45.6s) và migrations, `make test-api-unit`, `make scopelint`, `make lint`, `make test-web` (111 files, 975 passed / 3 skipped), e2e `library.spec.ts` 2/2 trên stack cô lập.
