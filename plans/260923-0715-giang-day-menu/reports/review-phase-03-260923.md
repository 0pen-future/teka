# Review — Phase 3: Kho học liệu (chương trình mẫu, phiên bản, buổi học mẫu)

Ngày: 2026-09-23 · Branch `feat/giang-day-menu` · Phạm vi: diff chưa commit (bỏ qua `docs/diagrams/`, `.pi/` và hunk `docs/architecture.md` theo yêu cầu).

## Verdict: **SHIP WITH FIXES**

Không có lỗi Critical. Tenancy, route manifest, catalog quyền, migration và backfill đều đúng spec và các quyết định đã chốt. Có một lỗi High đã xác nhận bằng đọc code: kiểm tra "bản nháp" nằm ngoài transaction và không khoá dòng phiên bản, nên một lần ghi buổi học chạy song song với Publish có thể sửa nội dung của phiên bản đã phát hành. Đây chính là bất biến mà Phase 7 dựa vào. Một lỗi Medium cùng gốc: vị trí buổi học tính bằng `len+1` không khoá, nên race tạo/xoá để lại khoảng trống và mọi lần thêm buổi sau đó trả 500. Cả hai sửa được bằng một khoá `FOR UPDATE` trên dòng phiên bản. Phần còn lại là Low/Nit.

## Verification đã chạy

| Lệnh | Kết quả |
|---|---|
| `go build ./...` | OK |
| `go vet` library, server, authctx, routespec, migrations (+ `-tags integration` cho library) | OK |
| `gofmt -l` các package đụng tới | sạch |
| `go test ./internal/features/library/ ./internal/server/ ./internal/shared/authctx/ ./internal/shared/routespec/ ./internal/features/audit/ ./migrations/` | tất cả `ok` |
| `golangci-lint run` library, authctx, routespec | 0 issues |
| `go run ./tools/scopelint ./internal/...` | exit 0 |
| `go tool swag init -g cmd/api/main.go --parseInternal` ra thư mục tạm rồi diff với `apps/api/docs` | `swagger.json`, `swagger.yaml` giống hệt; `docs.go` chỉ khác tên package do thư mục output → được sinh lại, không sửa tay; diff chỉ có dòng thêm |
| `npx vitest run src/features/library src/features/center` | 9 files, 89 passed |
| `npx tsc -b --noEmit` (web) | OK |
| `npx eslint src/features/library src/layouts/dashboard-layout.tsx e2e/library.spec.ts` | 0 error, 0 warning |

Đọc, không chạy: `integration_test.go` của library, `TestProgramTemplatesBackfillGrantsLibraryRead` trong `migrations_test.go`, `e2e/library.spec.ts`. Controller đã chạy xanh `go test -tags=integration -p 1 ./internal/features/library/... ./migrations/...` và e2e trên stack cô lập.

## Findings

### Critical
Không có.

### High

**H1 — Kiểm tra draft (`requireDraft`) chạy ngoài transaction và không khoá phiên bản → buổi học của phiên bản đã phát hành vẫn bị sửa được khi race với Publish.**
`apps/api/internal/features/library/service.go:272, :302, :322, :343` (so với `Publish` `:196-216`)
- `CreateLesson`, `UpdateLesson`, `DeleteLesson` và `ReorderLessons` gọi `requireDraft`, tức là một `SELECT` trạng thái không khoá, *trước* khi mở tx. `UpdateLesson` còn không dùng tx. Sau đó chúng ghi `template_lessons`.
- `Publish` chỉ làm `UPDATE program_template_versions SET status='published' WHERE status='draft'`. Câu này khoá dòng phiên bản, không đụng dòng buổi học, nên không xung đột với lần ghi buổi học.
- Kịch bản (READ COMMITTED): người soạn A bấm "Lưu" buổi 3 và `requireDraft` thấy `draft`. Người phát hành B bấm "Phát hành", status thành `published` và commit. A tiếp tục `UPDATE template_lessons ...` → 200. Phiên bản v1 đã phát hành giờ có nội dung khác lúc phát hành. Tạo, xoá và reorder cũng đi đúng đường này.
- DB không có hàng rào nào: không trigger, không cột trạng thái trên `template_lessons`. Spec gọi đây là bất biến chính ("Phiên bản published là bất biến để lớp áp dụng ổn định (Phase 7)"). Cửa sổ race hẹp, nhưng hai vai `library.edit` và `library.publish` được thiết kế cho hai người khác nhau cùng làm trên một bản nháp.
- Fix: thêm method repo `LockVersion(ctx, sc, id)` dùng `SELECT ... FOR UPDATE` (`clause.Locking{Strength: "UPDATE"}`) trên `program_template_versions`. Thay `requireDraft` bằng một bước chạy *bên trong* `WithinTx`: khoá, kiểm `Locked()`, rồi mới ghi. Bọc cả `UpdateLesson` trong tx. `Publish` và `Archive` giữ `UPDATE ... WHERE status = from` vì câu đó tự chờ khoá dòng. Thêm integration test: mở tx giữ khoá phiên bản, chạy Publish ở goroutine khác, và khẳng định lần ghi buổi học sau publish trả 409 `VERSION_LOCKED`.

### Medium

**M1 — Vị trí buổi mới là `len(existing)+1`, không khoá → race tạo/tạo trả 500, còn race tạo/xoá để lại khoảng trống làm mọi lần thêm buổi sau đó trả 500.**
`apps/api/internal/features/library/service.go:276-285` (tạo), `:325-334` (xoá + đánh số lại)
- Tạo/tạo: hai tx cùng đọc `n` buổi và cùng chèn `position = n+1`. Unique `(version_id, position)` là DEFERRED nên lỗi nổ lúc COMMIT. GORM dịch lỗi thành `gorm.ErrDuplicatedKey` (`AddError` dịch cả lỗi commit, `gorm@v1.31.2/gorm.go:405-411`). `CreateLesson` không map lỗi này, và `apperror.From` biến nó thành 500 `INTERNAL`.
- Tạo/xoá: tx xoá không thấy dòng chưa commit của tx tạo, nên chỉ đánh số lại `1..n-1`. Tx tạo chèn `n+1`. Cả hai commit được, và phiên bản còn `n` buổi ở vị trí `{1..n-1, n+1}`. Lần "Thêm buổi học" tiếp theo tính `len+1 = n+1`, trùng vị trí có sẵn, và trả 500. Lỗi này lặp lại mãi cho tới khi có người reorder hoặc xoá một buổi.
- Fix: khoá phiên bản như H1 để tuần tự hoá mọi lần ghi buổi của một phiên bản. Thêm vào đó, tính vị trí bằng `COALESCE(max(position), 0) + 1` và map `ErrDuplicatedKey` từ `WithinTx` thành 409 thay vì 500.

### Low

**L1 — Template đã xoá mềm vẫn sửa, phát hành và lưu trữ được qua `:vid`/`:lid`.**
`apps/api/internal/features/library/repository.go:168-172` (`versions`), `:242-246` (`lessons`)
- `GetVersion` và `GetLesson` chỉ lọc `center_id`, không join `program_templates.deleted_at`. Ví dụ: người soạn đang mở trang buổi học, người khác xoá chương trình, người soạn bấm "Lưu" → 200. `POST /library/versions/:vid/publish` trên bản nháp của template đã xoá cũng trả 200.
- Việc *đọc* phiên bản của template đã xoá là có chủ đích, để lớp đã gắn vẫn đọc được (comment `service.go:110-111`). Nhưng ghi thì không nên.
- Fix: trong bước khoá của H1, join template và trả 404 `program template` khi `deleted_at IS NOT NULL`. Chỉ áp dụng cho các đường ghi.

**L2 — Tìm kiếm `q` không escape `%`/`_`, trong khi comment phía web khẳng định API có escape.**
`apps/api/internal/features/library/repository.go:119-121`; `apps/web/src/features/library/api/library-api.ts:16`
- `classes/repository.go:284-286` đã có `likeEscaper` cùng `ESCAPE '\'`, và plan (Red Team F14) ghi rõ quy ước này. Ở đây `q = "CT_1"` khớp cả `CTX1`, và `q = "%"` trả mọi dòng. Truy vấn vẫn scope theo trung tâm nên không rò dữ liệu. Nhưng kết quả sai, và comment web là một khẳng định sai.
- Fix: dùng lại `likeEscaper` (hoặc chuyển nó sang `shared/`) với `ILIKE ? ESCAPE '\'`. Thêm một case unit/integration cho `_`.

**L3 — Swagger khai báo `page_size` nhưng API đọc `per_page`.**
`apps/api/internal/features/library/handler.go:65`
- `pagination.Parse` đọc `per_page` (`shared/pagination`), và web gửi `per_page`. Client sinh từ OpenAPI sẽ gửi `page_size` và bị bỏ qua im lặng. Library là handler duy nhất dùng tên này.
- Fix: đổi annotation thành `per_page`, rồi chạy `make api-docs`.

**L4 — Tạo bản nháp chỉ sao chép từ bản `published`. Khi mọi bản đã lưu trữ, bản nháp mới rỗng.** *(Plausible — khoảng trống sản phẩm, không phải lỗi so với spec)*
`apps/api/internal/features/library/service.go:149-152`
- Spec viết "copy lessons của bản published mới nhất", và code đúng chữ spec. Nhưng bản archived vẫn giữ nội dung. Owner lưu trữ v2 (bản published duy nhất) rồi bấm "Tạo bản nháp mới" sẽ nhận v3 trống, dù modal hứa "sao chép các buổi của phiên bản đã phát hành gần nhất".
- Fix đề xuất: dùng phiên bản không phải draft có `version_no` cao nhất làm nguồn (`status IN ('published','archived')`). Nếu giữ nguyên thì sửa câu mô tả trong modal.

**L5 — Publish chấp nhận bản nháp 0 buổi.** *(Plausible — liên quan Phase 7)*
`apps/api/internal/features/library/service.go:204-208`
- Một phiên bản rỗng được phát hành sẽ thành đích hợp lệ cho `courses.default_template_version_id` và `class_programs` ở Phase 5/7. Áp dụng nó sẽ ghi `class_curricula.lessons` rỗng. Spec không cấm điều này.
- Fix đề xuất: trả 422 hoặc 409 khi `LessonCount == 0`. Cách khác là để Phase 7 chặn lúc áp dụng. Việc này cần user quyết.

**L6 — Lưu buổi học bị 409 `VERSION_LOCKED` không làm mới trạng thái phiên bản, nên form vẫn mở để sửa.**
`apps/web/src/features/library/hooks/use-library.ts:163-172`
- `useUpdateLesson` chỉ invalidate trong `onSuccess`. Khi phiên bản vừa được phát hành bởi người khác, người soạn thấy lỗi nhưng `LessonEditor` vẫn hiện và nút "Lưu" vẫn bấm được. Các mutation khác của feature đã dùng `onSettled` với đúng lý do này (`use-library.ts:88-97`).
- Fix: chuyển sang `onSettled`, invalidate `versionsKeys.list(templateId)` và `lessonsKeys.detail(id)`. Muốn vậy hook cần nhận `templateId`.

**L7 — Danh sách và dropdown chương trình cắt ở 100 dòng mà không báo.**
`apps/web/src/features/library/pages/library-page.tsx:106, :233`
- `per_page: 100` là trần server (`maxPerPage = 100`). Trang không có phân trang hay chỉ báo "còn nữa". Tab "Buổi học mẫu" không có tìm kiếm phía server, nên chương trình thứ 101 trở đi không chọn được. Quy mô hiện tại khó chạm tới mức này.
- Fix: hiện `meta.total` kèm phân trang, hoặc dùng `q` phía server cho `HvSelect` của tab Buổi học mẫu.

### Nit

- `template-detail-page.tsx:289`: dialog xoá ghi "Mọi phiên bản và buổi học mẫu của chương trình sẽ bị xoá", nhưng API chỉ xoá mềm template và giữ nguyên phiên bản và buổi học. Nên viết "Chương trình sẽ bị gỡ khỏi kho; lớp đã dùng vẫn đọc được."
- `library-page.tsx:46`: `canEdit` không chờ `isResolved`, nên nút "Tạo chương trình mẫu" hiện muộn. Đây là đúng hiện tượng mà hai trang chi tiết đã tránh.
- `library-schemas.ts:72`: comment nói "Mirrors `classcode.Valid`", nhưng web cấm dấu `-` ở đầu còn server (`^[A-Z0-9-]{2,20}$`) cho phép. Web chặt hơn nên vô hại. Nên sửa comment hoặc thống nhất regex.
- `use-library.ts` `useLessonListWrite`: reorder hoặc xoá đổi `position` của các buổi khác, nhưng `lessonsKeys.detail` không bị invalidate. Tiêu đề "Buổi N" trên trang buổi học có thể cũ cho tới khi hết staleTime.
- `repository.go:128`: sắp xếp theo `name` không có khoá phụ `id`, nên phân trang không ổn định khi trùng tên.
- `000027_program_templates.down.sql`: chỉ gỡ dòng có trong sổ `rbac_backfill_rows`, đúng spec. Tuy vậy các dòng `library.edit`/`library.publish` owner gán tay sẽ còn lại sau down, khác với 000022 vốn xoá mọi key. Key lạ khi đọc lại không khớp capability nào (`authctx/permissions.go:5`), nên vô hại. Chỉ ghi nhận.
- `TemplateRequest.Subject/Level/Description` không trim, nên chuỗi rỗng từ client khác được lưu thành `""` thay vì NULL. Web đã chuyển rỗng thành `null`.

## Kiểm tra theo focus points

1. **Tenancy: đạt.** Mọi truy vấn repo đi qua `liveTemplates`, `versions` hoặc `lessons`, và cả ba đều có `center_id = sc.CenterID`. `SetPositions` lọc `center_id` và `version_id`. Subquery tóm tắt dùng `template_id` và dựa vào FK composite để giữ cùng trung tâm. Scopelint xanh. Id của trung tâm khác trả 404 (integration `TestCrossCenterRowsAreNotFound`), và id sai định dạng cũng trả 404. Kho là tài sản toàn trung tâm theo spec, nên không thu hẹp theo giáo viên.
2. **Authz: đạt.** Có 15 route: 5 GET dùng `perm(library.read, none())`. 10 route mutating có `req(action, entity, id)` và entity `program_template`/`template_version`/`template_lesson`. Publish dùng `library.publish`, archive dùng `library.edit`. Service lặp lại `authctx.Require` cho mọi method. Catalog: `library.read` là `def()`, edit/publish là `optIn()`, implied edit⇒read và publish⇒read, `CatalogVersion` không đổi. MSW mirror đủ ba key và `RESOURCE_LABELS` có nhóm `library`.
3. **State machine: đúng về logic, sai về đồng thời (H1, M1).** Chỉ một bản nháp nhờ unique một phần, và race tạo bản nháp được map thành 409 `DRAFT_EXISTS`. Publish/Archive dùng `UPDATE ... WHERE status = from`, nên publish đồng thời trả 409. Bản nháp mới sao chép buổi học trong cùng tx. Unique `(version_id, position)` DEFERRABLE cho phép hoán đổi trong một tx (integration `TestReorderAndDeleteKeepPositionsContiguous`).
4. **Migration: đạt.** Ba bảng đều có `center_id NOT NULL`, `UNIQUE (id, center_id)` và FK composite `ON DELETE CASCADE`. `created_by` dùng `SET NULL (created_by)` về `center_members`, theo khuôn 000022. Backfill hai bảng có step label riêng, dùng `ON CONFLICT DO NOTHING` để giữ deny, bỏ qua trung tâm đã xoá, owner và thành viên đã rời. Chạy lại up không nhân đôi dòng. Down gỡ theo sổ rồi drop theo thứ tự ngược. `MigrateDown(23)` và danh sách `domainTables` đã được cập nhật. Test backfill 000022 được ghim ở `m.Migrate(22)` để 000027 không làm lệch số đếm.
5. **Web: phần lớn đạt.** Nav có `perm: "library.read"`, và mục có mặt trong cả `OVERFLOW_LABELS` và `OVERFLOW_PATH_PREFIXES`. Nút Phát hành gate theo `library.publish`, còn soạn thảo gate theo `library.edit` cùng trạng thái draft. Mutation phiên bản và danh sách buổi học invalidate trong `onSettled`. Thiếu sót nằm ở L6, L7 và các Nit.
6. **Tương thích Phase 7:** `UNIQUE (id, center_id)` trên `program_template_versions` sẵn sàng cho FK composite của `class_programs`. Archive giữ nguyên buổi học. Để bất biến "published" thật sự đáng tin khi lớp gắn vào, cần sửa H1 trước Phase 7. L5 là quyết định nên chốt trước Phase 5/7.

## Test gaps

Nên thêm cùng các fix:
- Integration: race giữa ghi buổi học và Publish trả 409 `VERSION_LOCKED` (H1).
- Integration: tạo/xoá buổi song song vẫn giữ vị trí `1..n` liên tục, và không có 500 (M1).
- Service: ghi vào phiên bản của template đã xoá mềm trả 404 (L1). Tìm `q` chứa `_`/`%` (L2).
- Quyền: chưa có test cho dòng "member bị deny `library.read`" trong bảng 8 dòng của `docs/adding-permissions.md`. Test backfill chỉ chứng minh deny được giữ trong DB, chưa chứng minh bị 403 qua service hay HTTP.

Nice-to-have:
- Vitest: lưu buổi học bị 409 thì chuyển trang sang chế độ khoá (L6).

## Quyết định giữ nguyên

- Tất cả quyết định controller liệt kê đều được giữ: `created_by` nullable với `SET NULL`; `library.read` def kèm backfill; edit/publish opt-in; `CatalogVersion` = 4; archive gate `library.edit`; sort whitelist `name|code|created_at`; không có mục `backfill_parity_test` cho 000027; tab Buổi học mẫu chỉ đọc; trang chi tiết gate theo `isResolved`; test thứ tự tab Center được cập nhật.
- **Implied key thắng deny** *(quyết định có chủ đích, ghi nhận hệ quả)*: implied key được thêm *sau* bước deny (`authctx/permissions.go:68-92`). Vì vậy một member bị deny `library.read` nhưng được cấp `library.edit` vẫn đọc được kho. Đây là cùng ngữ nghĩa với `reports.send`, và việc soạn thảo mà không đọc được thì không có nghĩa. Không đề xuất đảo. Chỉ nên để ma trận quyền hiện rõ điều này cho owner.
- Nhiều phiên bản `published` cùng tồn tại, và `published_version_no` là số lớn nhất. Spec không yêu cầu tự lưu trữ bản cũ khi phát hành bản mới, nên review không raise.

## Unresolved questions

- L5: có chặn phát hành bản nháp rỗng không? Nếu có, chặn ở Phase 3 (publish) hay Phase 7 (áp dụng)?
- L4: bản nháp mới có nên sao chép từ bản archived khi không còn bản published không?

## Điểm tốt

- Migration theo đúng khuôn 000022, kể cả sổ backfill, giữ deny và bỏ qua trung tâm đã xoá. Test backfill phủ đủ các nhánh: role hệ thống, thành viên không vai, thành viên đã rời, dòng deny sẵn có và trung tâm đã xoá.
- Route manifest, audit action và snapshot đều khớp nhau. Path param `:id`/`:vid`/`:lid` tách riêng nên mỗi dòng audit trỏ đúng thực thể.
- Swagger được sinh lại thật, và lint, typecheck, eslint đều sạch hoàn toàn.
- Test web khẳng định hành vi thật theo từng vai: chỉ đọc, chỉ soạn, chỉ phát hành. Chúng không chỉ render cho có.

Status: DONE_WITH_CONCERNS
Summary: SHIP WITH FIXES — 0 Critical, 1 High (race requireDraft/Publish phá bất biến published), 1 Medium (race vị trí buổi → 500 kéo dài), 7 Low, 7 Nit; mọi kiểm tra unit/lint/typecheck/vitest/swagger đều xanh.
Concerns/Blockers: Không chạy integration hay Playwright theo ràng buộc; H1/M1 xác nhận bằng đọc code (không có test tái hiện race).

## Disposition (2026-09-23, sau khi sửa)

Kết quả sau sửa: unit library + `make test-api-unit` + `make scopelint` xanh; integration `go test -tags=integration -p 1 ./internal/features/library/...` xanh (thêm 2 test race); web `vitest` 954 pass, `lint` 0 lỗi (7 warning có sẵn ở file không chạm), `typecheck` xanh; Playwright `e2e/library.spec.ts` pass trên stack cô lập.

| Finding | Xử lý | Cách sửa / lý do giữ |
|---|---|---|
| H1 race requireDraft ↔ Publish | **Đã sửa** | Mọi ghi buổi học (`CreateLesson`/`UpdateLesson`/`DeleteLesson`/`ReorderLessons`) và mọi chuyển trạng thái (`Publish`/`Archive`) đi qua `WithinTx` và khoá hàng phiên bản bằng `LockVersion` (`SELECT … FOR UPDATE OF program_template_versions`) trước khi kiểm tra trạng thái. Integration test `TestLessonWritesSerialiseWithPublish` chứng minh ghi buổi bị chặn tới khi publish xong rồi nhận 409 `VERSION_LOCKED`. |
| M1 position = len+1 ngoài khoá | **Đã sửa** | `NextPosition` = `COALESCE(max(position),0)+1` chạy trong cùng transaction đã khoá phiên bản; `gorm.ErrDuplicatedKey` được map thành 409 kèm thông điệp tải lại. `TestConcurrentLessonWritesKeepPositionsContiguous` chạy 6 create + 1 delete đồng thời và kiểm tra vị trí 1..6 liên tục. |
| L1 ghi lên phiên bản của template đã xoá mềm | **Đã sửa** | `LockVersion` JOIN `program_templates … deleted_at IS NULL`, nên mọi ghi và chuyển trạng thái trả 404. Đọc vẫn hoạt động có chủ đích (lớp đã dùng vẫn xem được). |
| L2 ILIKE không escape | **Đã sửa** | Thêm package dùng chung `internal/shared/likeq` (`Contains`/`Prefix`), thay `likeEscaper` cục bộ ở `classes` và `audit`; library dùng `ILIKE ? ESCAPE '\'`. Integration test tìm `8_9` chỉ khớp tên có dấu gạch dưới, `%` trả 0. |
| L3 swagger `page_size` vs `per_page` | **Đã sửa** | Annotation đổi thành `per_page`, `make api-docs` tái sinh docs. |
| L4 bản nháp mới rỗng khi mọi phiên bản đã lưu trữ | **Đã sửa, có đổi so với câu chữ spec** | Spec ghi "copy lessons của bản published mới nhất". Quyết định: bản nháp sao chép từ phiên bản **không phải nháp** có `version_no` cao nhất (published, hoặc archived khi không còn published). Lý do: mọi phiên bản released đều bất biến nên nội dung sao chép là an toàn; trung tâm lưu trữ v1 rồi muốn sửa lại sẽ không phải nhập lại từ đầu. Phương án thay thế nếu muốn giữ đúng câu chữ spec: chỉ copy từ published, nháp rỗng khi không có published — đổi lại bằng cách trả `LatestReleasedVersion` về lọc `status = 'published'`. MSW và mô tả modal web đã cập nhật theo quyết định này. |
| L5 phát hành bản nháp 0 buổi | **Giữ nguyên, câu hỏi mở** | Spec không cấm; chặn ở đây là thêm quy tắc sản phẩm. Ghi nhận để Phase 7 quyết định chặn tại thời điểm áp chương trình vào lớp (nơi số buổi mới có ý nghĩa). |
| L6 `useUpdateLesson` chỉ invalidate khi thành công | **Đã sửa** | Invalidate danh sách buổi, chi tiết buổi và danh sách phiên bản trong `onSettled`; 409 `VERSION_LOCKED` làm trang tự chuyển sang chế độ chỉ đọc. Vitest mới: PUT trả 409 và store đổi trạng thái → hiện thông báo khoá, form biến mất. |
| L7 danh sách cắt ở 100 không có chỉ báo | **Giữ nguyên** | Giới hạn 100 là trần server dùng chung của `pagination`; ở quy mô hiện tại một trung tâm không tới 100 chương trình/phiên bản. Sẽ thêm phân trang khi có nhu cầu thật. |
| Nit: dialog xoá nói "xoá mọi phiên bản" | **Đã sửa** | Mô tả mới: "Chương trình sẽ bị gỡ khỏi kho. Lớp đã dùng phiên bản của nó vẫn đọc được." |
| Nit: `library-page.tsx` `canEdit` chưa chờ `isResolved` | **Đã sửa** | Trang hiện `HvStateBlock` loading tới khi center context resolve. |
| Nit: comment `library-schemas.ts` sai với regex | **Đã sửa** | Comment nêu rõ web chặt hơn server (không cho `-` đứng đầu). |
| Nit: `useLessonListWrite` không invalidate detail | **Đã sửa** | Thêm khoá `lessonsKeys.details()` và invalidate trong `onSettled`. |
| Nit: sort theo name thiếu tie-breaker | **Đã sửa** | `ListTemplates` thêm `ORDER BY … , program_templates.id ASC`. |
| Nit: down-migration để lại quyền `library.edit/publish` cấp tay | **Ghi nhận** | Không sửa: down-migration chỉ chạy ở dev, và xoá hàng `center_member_permissions` theo key nằm ngoài phạm vi; ghi vào phase file. |
| Nit: Subject/Level/Description không trim | **Đã sửa** | `optionalText` trim và chuyển chuỗi rỗng thành NULL ở cả create/update; có unit test. |
| Test gaps (race, template đã xoá → 404, `_`/`%`, deny `library.read` → 403) | **Đã bổ sung** | Unit `TestWritesToDeletedTemplateAreNotFound`, `TestLessonWritesAndTransitionsTakeTheVersionLock`; integration race tests, search escape, deny row → 403. |
