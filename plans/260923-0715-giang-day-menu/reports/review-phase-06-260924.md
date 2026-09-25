# Review — Phase 6: Lộ trình học

Ngày: 2026-09-24 · Branch `feat/giang-day-menu` · Phạm vi: hai commit `4ae6166` (API) và `2f6151f` (web), đọc qua `git diff 4ae6166^..2f6151f`.

## Verdict tóm tắt

Không có lỗi Critical, High hay Medium. Phần lõi đúng và khớp khuôn `courses`:
- **Tenancy.** Mọi truy vấn của `paths` đều gắn `center_id`, gồm cả `live`, `stages`, `links`, `LockCourses` và câu join phía khóa học `stageLinks`. Các FK composite `(path_id, center_id)`, `(stage_id, center_id)` và `(course_id, center_id)` chặn nối chéo trung tâm ngay ở DB. Id của trung tâm khác đọc thành 404.
- **Đồng thời.** Mọi thao tác ghi vào vị trí giai đoạn (thêm, xoá, sắp xếp, gán khóa) đều khoá `learning_paths` bằng `FOR UPDATE` trước. `SetStageCourses` khoá khóa học bằng `FOR SHARE`, còn `courses.Delete` khoá bằng `FOR UPDATE` trong cùng transaction với phần đếm. Vì vậy hai thứ tự đều cho ra kết quả nhất quán: gán trước thì xoá nhận 409, xoá trước thì gán nhận 422.
- **RBAC.** Route manifest (+11), snapshot (+11) và audit (+8) khớp nhau. `paths.edit` kéo theo `paths.read`. `GET /courses/:id/paths` đòi cả hai quyền đọc.
- **Swagger.** Chạy lại `swag init` ra scratchpad cho kết quả trùng khớp với `apps/api/docs/swagger.json` đã commit, nên không có sửa tay. Không có thay đổi nào trong `apps/web/src/components/ui`.

Các finding còn lại là Low. Chủ yếu gồm cache phía web không làm mới đủ, lost update khi hai người sửa cùng một giai đoạn, và vài chỗ biên API rộng hơn web.

## Verification đã chạy

| Lệnh | Kết quả |
|---|---|
| `go vet ./...` (api) | OK |
| `go vet -tags integration` paths, courses, migrations | OK (chỉ compile, không chạy) |
| `gofmt -l` paths, courses, shared | sạch |
| `go test` paths, courses, idset, authctx, routespec, server, audit | tất cả `ok` |
| `go tool swag init … -o <scratchpad>` rồi so với `apps/api/docs/swagger.json` của `2f6151f` | trùng khớp |
| `npx vitest run src/features/courses` | 4 files, 43 passed |
| `npx tsc -b --noEmit` (web) | OK |
| `npx eslint` courses, `dashboard-layout.tsx`, `center/schemas` | 0 error, 2 warning. Cả hai là `react-hooks/incompatible-library` do `form.watch`, cùng kiểu với `course-dialog.tsx`; cái mới ở `path-dialog.tsx:83`. |

Đọc, không chạy (cần Docker): `paths/integration_test.go`, `TestDeleteIsGuardedByLearningPaths` trong `courses/integration_test.go`, `TestLearningPathsBackfillGrantsPathsRead` và `TestLearningPathsSchemaInvariants` trong `migrations_test.go`.

## Đối chiếu yêu cầu

| Mục kiểm | Kết quả | Bằng chứng |
|---|---|---|
| Mọi truy vấn gắn `center_id` | Đạt | `paths/repository.go:88-106` (scope `live`/`stages`/`links`), `:251-262` (`LockCourses`); `courses/repository.go` `stageLinks` (join `ps.center_id = psc.center_id`, `lp.center_id = ps.center_id`, `psc.center_id = ?`) |
| Id chéo trung tâm trả 404 | Đạt | `ErrNotFound`/`ErrStageNotFound` → `notFound()`. Test: `TestStageOfAnotherPathIsNotFound`, `TestStageCoursesStayWithinTheCenter`, và `ListPaths` của outsider trả 404 |
| `LockCourses` chỉ nhận khóa sống cùng trung tâm | Đạt, nhưng **nhận mọi trạng thái** chứ không chỉ `active` (xem L4) | `repository.go:259`; `service.go:200` ("archived is fine") |
| Reorder dưới unique DEFERRABLE, có khoá cha | Đạt | `service.go:175-197` (`Lock` → `ListStages` → `idset.Same` → `SetStagePositions`); migration `uq_path_stages_position … DEFERRABLE INITIALLY DEFERRED` |
| Xoá giai đoạn đánh số lại liền mạch | Đạt | `service.go:150-171`; test tích hợp và unit đều khẳng định vị trí 1..n |
| Thay danh sách khóa atomically; cap 20; id trùng trả 422 | Đạt | `service.go:202-239` (cap và trùng kiểm trước tx; delete và insert trong `WithinTx` cùng khoá path); binding `max=20` ở `dto.go:33` |
| `paths.read` mặc định, `paths.edit` opt-in, edit ⇒ read | Đạt | `authctx/catalog.go` (`def`/`optIn`/`impliedKeys`); backfill 2 nhánh với step label riêng; down gỡ theo `rbac_backfill_rows` (khuôn 000029) |
| Mọi route ghi đòi `paths.edit`; `GET /courses/:id/paths` đòi cả `courses.read` và `paths.read` | Đạt | `routespec.go` (manifest ghi `paths.read`, service kiểm thêm `courses.read`); test `TestListPathsNeedsPathsReadAndALiveCourse` |
| `COURSE_IN_PATH` chỉ tính lộ trình còn sống | Đạt | `stageLinks` join `lp.deleted_at IS NULL`; `TestDeleteIsGuardedByLearningPaths` xoá mềm lộ trình rồi xoá khóa thành công |
| Web gate `has("paths.edit")`/`has("paths.read")` | Đạt | `learning-paths-page.tsx:45`, `path-detail-page.tsx:79`, `course-detail-page.tsx:295`, nav `perm: "paths.read"` |
| Web cập nhật cache sau thao tác giai đoạn | Đạt với thao tác giai đoạn. **Thiếu** ở sửa/xoá lộ trình và phía khóa học (L1) | `use-paths.ts:89-103` |
| Form khớp giới hạn API | Đạt. Chỉ có regex mã lệch nhẹ (L6) | `paths-schemas.ts:72-94` so với `dto.go`/`classcode.Valid` |
| A11y timeline | Đạt cơ bản (`<ol>`, `section aria-label`, nút icon có nhãn). Nhãn nút trùng giữa các giai đoạn (L5) | `path-detail-page.tsx:278-318` |
| Lỗi hiện qua toast `ApiError` | Đạt | `apiMessage()` + `hvToast(…, danger)` cho reorder, gán khóa, xoá giai đoạn, xoá lộ trình |

## Findings

### Critical
Không có.

### High
Không có.

### Medium
Không có.

### Low

**L1 — Cache phía web không làm mới khi dữ liệu liên quan đổi, nên chi tiết khóa học có thể hiện lộ trình đã xoá hoặc đã đổi tên, và timeline có thể hiện tên hoặc trạng thái khóa cũ.** *(CONFIRMED qua đọc code)*
`apps/web/src/features/courses/hooks/use-paths.ts:59-83` (`useUpdatePath`, `useDeletePath` chỉ invalidate `pathsKeys.lists()`); `apps/web/src/features/courses/hooks/use-courses.ts:39-108` (không mutation nào chạm `pathsKeys`); `staleTime: 30_000` ở `app/providers.tsx:12`.
- Kịch bản 1:
  1. Owner mở chi tiết khóa TOAN-6. Tab Thông tin hiện "LT-6 · Giai đoạn 1", tạo cache `["paths","by-course",id]`.
  2. Owner sang `/paths/LT-6` và xoá lộ trình.
  3. Owner quay lại chi tiết khóa trong vòng 30 giây. Section "Lộ trình học" vẫn hiện LT-6, và link `/paths/:id` dẫn tới "Không tìm thấy lộ trình".
  4. Đổi tên hay chuyển trạng thái lộ trình cũng để lại nhãn cũ theo cách tương tự.
- Kịch bản 2: owner ngừng tuyển (archive) hoặc đổi tên một khóa, rồi mở lại lộ trình chứa khóa đó trong vòng 30 giây. Chip vẫn hiện tên cũ và không có badge "Ngừng tuyển".
- Fix:
  - Trong `onSettled` của `useUpdatePath`/`useDeletePath`, thêm `invalidateQueries({ queryKey: [...pathsKeys.all, "by-course"] })`.
  - Các mutation sửa, lưu trữ và xoá khóa trong `use-courses.ts` invalidate thêm `pathsKeys.details()` (và `pathsKeys.lists()`, vì card đếm khóa theo khóa còn sống). Nên đặt ngay trong feature `courses` vì cả hai bộ key cùng một feature.

**L2 — Thay danh sách khóa theo kiểu ghi đè từ trạng thái client, nên hai người sửa cùng giai đoạn sẽ im lặng xoá thay đổi của nhau (lost update).** *(CONFIRMED qua đọc code)*
`apps/web/src/features/courses/pages/path-detail-page.tsx:349` và `:366` (`save([...stage.courses.map(c => c.id), courseId])`); `apps/api/internal/features/paths/service.go:202-239`.
- Kịch bản:
  1. A và B cùng mở `/paths/LT-6`, và giai đoạn 1 có [TOAN-6].
  2. A thêm VAN-6, server lưu [TOAN-6, VAN-6].
  3. B, với cache cũ chưa refetch, thêm ANH-6. Client gửi [TOAN-6, ANH-6], và server ghi đè thành công.
  4. VAN-6 biến mất mà không ai được báo.
- Khoá `FOR UPDATE` chỉ tuần tự hoá hai request, không phát hiện được là dữ liệu đã cũ. Đây là cùng khuôn với gói học phí của khóa học (ghi đè cả danh sách), nên là lựa chọn thiết kế chứ không phải hồi quy. Chỉ ghi lại vì mất dữ liệu diễn ra im lặng.
- Fix (chọn một):
  - Rẻ nhất: gửi kèm `updated_at` của lộ trình (hoặc một `version`), service so sánh sau khi `Lock` và trả 409 khi lệch. Web bắt 409, refetch rồi toast "Lộ trình vừa được người khác sửa".
  - Hoặc thêm hai endpoint cộng/trừ một khóa (`POST`/`DELETE …/courses/:cid`) cho thao tác chip. Endpoint ghi đè giữ lại cho sắp xếp.

**L3 — Mutation giai đoạn bị lỗi không làm mới chi tiết, nên người dùng mắc kẹt trong một vòng lỗi lặp lại.** *(CONFIRMED qua đọc code)*
`apps/web/src/features/courses/hooks/use-paths.ts:89-103` (`onSettled` chỉ invalidate list và by-course, không invalidate `detail(pathId)`).
- Kịch bản:
  1. B thêm giai đoạn 4 trong khi A đang xem lộ trình 3 giai đoạn.
  2. A bấm "Xuống" ở giai đoạn 2. Client gửi `stage_ids` gồm 3 id, và server trả 422 "Thứ tự phải liệt kê đúng mỗi giai đoạn".
  3. Toast hiện ra, nhưng cache vẫn là 3 giai đoạn, nên mọi lần bấm lại đều 422 cho tới khi đổi focus cửa sổ hoặc tải lại trang.
  4. Tương tự khi giai đoạn đã bị người khác xoá: sửa, gán khóa hay xoá đều trả 404 lặp lại.
- Fix: trong `useStageMutation`, thêm `onError: () => invalidateQueries({ queryKey: pathsKeys.detail(pathId) })`. Có thể làm luôn trong `onSettled`, vì `setQueryData` ở `onSuccess` đã có dữ liệu mới nên refetch thêm là thừa nhưng vô hại.

**L4 — API cho gắn khóa ở trạng thái `draft` vào giai đoạn. Web chỉ đưa ra khóa `active`.** *(CONFIRMED)*
`apps/api/internal/features/paths/repository.go:259` (`LockCourses` chỉ lọc `deleted_at IS NULL`); `service.go:200` (comment chỉ nói tới archived).
- Nhận khóa `archived` là cần thiết: nếu không, khi thêm khóa mới, client gửi lại cả danh sách đang có một khóa archived và sẽ bị 422 (`path-detail-page.tsx:83-84` ghi đúng ý này). Nhận khóa `draft` thì không có lý do trong comment hay trong phase file.
- Kịch bản: một client khác web, hoặc gọi API thẳng, gắn khóa nháp "TOAN-7 (đang soạn)" vào lộ trình `active` dùng cho tư vấn tuyển sinh. Tư vấn viên thấy và giới thiệu một khóa chưa mở.
- Fix (cần quyết định sản phẩm): hoặc chỉ nhận `active` cộng các id `archived` hay `draft` **đã có sẵn** trong giai đoạn (so với danh sách hiện tại sau khi `Lock`), hoặc ghi rõ trong comment và swagger rằng mọi trạng thái đều được nhận. Nếu nhận thì hiện badge "Đang soạn" trên chip, như `path-detail-page.tsx:338` đang làm với mọi trạng thái khác `active`.

**L5 — Nhãn a11y của nút điều khiển trùng nhau giữa các giai đoạn.** *(CONFIRMED)*
`apps/web/src/features/courses/pages/path-detail-page.tsx:299` (`aria-label="Lên"`), `:308` (`"Xuống"`), `:312-317` ("Sửa", "Xoá"), `:371` (`aria-label="Thêm khóa học"`).
- Mỗi giai đoạn nằm trong một `section aria-label="Giai đoạn n: …"`, nên người dùng screen reader đi theo landmark vẫn có ngữ cảnh. Nhưng danh sách nút hoặc form-control của trình đọc (rotor, Elements list) sẽ hiện N nút "Lên" và N ô "Thêm khóa học" giống hệt nhau, không lọc nhận được (WCAG 2.4.6).
- Chip "Gỡ {tên khóa}" đã làm đúng.
- Fix: dùng `aria-label={\`Đưa ${stage.name} lên\`}`, `\`Sửa giai đoạn ${stage.name}\``, `\`Thêm khóa học vào ${stage.name}\``, … Test vitest hiện tìm nút theo tên trong `within(section)`, nên cần cập nhật theo.

**L6 — Biên API rộng hơn web ở vài chỗ.** *(CONFIRMED)*
- Mã lộ trình:
  - `classcode.Valid` dùng `^[A-Z0-9-]{2,20}$`, nhận cả `--` hay `-A`, trong khi web dùng `^[A-Z0-9][A-Z0-9-]*$` và cấm gạch đầu (`paths-schemas.ts:82`, `courses-schemas.ts:73`).
  - Đây là thừa hưởng từ `courses`, không phải lỗi riêng của phase này. Một client ngoài web tạo `-LT` thì web mở form sửa sẽ báo lỗi ngay trên mã đang lưu.
- Số giai đoạn không có trần:
  - `CreateStage` và `ReorderRequest` (`dto.go:27`, chỉ có `min=1`) không giới hạn. `SetStagePositions` (`repository.go:225-235`) chạy N câu `UPDATE` tuần tự trong khi giữ khoá lộ trình.
  - Rủi ro thấp vì chỉ người có `paths.edit` gọi được. Vẫn nên đặt trần (ví dụ 50) cho đối xứng với trần 20 khóa mỗi giai đoạn.
- Trạng thái lúc tạo: API nhận `archived` khi tạo, còn web chỉ đưa `draft`/`active` (`path-dialog.tsx:82`). Vô hại, chỉ ghi nhận.

**L7 — Picker khóa chỉ nạp 100 khóa `active` đầu tiên và im lặng khi không nạp được.** *(CONFIRMED, cùng khuôn với dialog tạo lớp)*
`apps/web/src/features/courses/pages/path-detail-page.tsx:85-86`.
- Trung tâm có hơn 100 khóa đang tuyển: những khóa xếp sau theo tên không bao giờ hiện trong picker, và ô tìm của `HvSelect` chỉ lọc trên danh sách đã nạp. `classes-api.ts:54` (dialog tạo lớp) có cùng giới hạn, nên đây là giới hạn đã chấp nhận ở phase 5.
- Người có `paths.edit` nhưng bị deny `courses.read` thì query catalog trả 403. Picker khi đó rỗng, không có thông báo nào, và chip khóa vẫn link tới `/courses/:id`, nơi họ không mở được. `class-detail-header.tsx:21` đã xử lý đúng trường hợp này bằng `has("courses.read")`.
- Fix: khi `catalog.isError`, hiện dòng "Không tải được danh mục khóa học". Gate link chip bằng `has("courses.read")`, như header lớp. Giới hạn 100 để lại cho khi có endpoint options có tìm kiếm phía server.

**L8 — Deep link `/paths` khi thiếu `paths.read` hiện lỗi chung chung.** *(CONFIRMED, cùng khuôn `/courses`)*
`apps/web/src/features/courses/pages/learning-paths-page.tsx:120-129`.
- Nav ẩn mục này đúng cách. Nhưng mở thẳng URL thì nhận 403, và trang hiện "Không tải được lộ trình học" kèm nút "Thử lại", mà bấm lại vẫn 403.
- `courses-page.tsx` cũng vậy, nên đây là parity, không phải hồi quy. Nếu sửa thì sửa chung: phân nhánh `ApiError.status === 403` sang "Bạn chưa có quyền xem lộ trình học".

### Nit
- `apiMessage()` được định nghĩa lại ở `path-detail-page.tsx:35` giống hệt `course-detail-page.tsx`. Nếu `lib/api/errors` chưa có helper tương đương thì chấp nhận được. Nếu đã có thì dùng lại.
- Phase file ghi snapshot "+9" và audit "+7", còn thực tế là +11 và +8. Con số thực tế đúng (10 route `paths`, 1 route `courses`; 8 route ghi), nên phase file đếm thiếu. Nên sửa khi đóng phase để khỏi làm nhiễu các lần đối chiếu sau.

## Khoảng trống test đáng đóng

1. **Đồng thời, integration.** Chưa có test chạy song song cho các cặp sau:
   - Hai `CreateStage` cùng lộ trình: cả hai phải thành công với vị trí 1 và 2, không có 500 ở COMMIT.
   - `SetStageCourses` với `courses.Delete` trên cùng khóa: kết quả phải là (200, 409) hoặc (422, 200), không bao giờ có link tới khóa đã xoá.
   Đây là lý do tồn tại của `Lock` và `LockCourses`. Không có test thì một refactor sau có thể lặng lẽ bỏ khoá mà CI vẫn xanh.
2. **HTTP binding.** Chưa có case `course_ids` 21 phần tử qua HTTP (binding `max=20`), và chưa có case `stage_ids` chứa uuid sai định dạng. Service test phủ cap 20 nhưng bỏ qua binding, đúng bài học M1 của phase 5.
3. **Web, luồng lỗi:**
   - Toast khi reorder trả 422, khi gán khóa trả 422, và khi xoá giai đoạn trả 404.
   - Case xoá khóa trả `COURSE_IN_PATH` trên `course-detail-page` (hiện chỉ có `COURSE_IN_USE`).
   - Trạng thái lỗi và loading của `learning-paths-page`. Phase file yêu cầu "quartet", mà mới có data và empty.
4. **Web, chip archived được giữ lại.** Chưa có test "thêm khóa mới vào giai đoạn đang có một khóa archived thì request vẫn gửi id archived". Đây chính là hợp đồng mà comment ở `path-detail-page.tsx:83-84` mô tả.

## Pattern parity với `courses`

- Khớp:
  - Service kiểm quyền ở đầu mỗi method, repository có scope `live` với `deleted_at` tường minh, `affected()` biến 0 dòng thành not-found.
  - `mapCodeClash` trả 409 `CODE_TAKEN`, và `useApiFormErrors({ conflictField: "code" })` gắn lỗi này vào ô mã.
  - Sort whitelist có tie-breaker `id`. Mọi response dùng `make(…, 0, n)` nên serialise ra `[]` chứ không ra `null`.
  - Migration cùng khuôn 000029: composite FK, unique một phần trên `(center_id, code)`, backfill hai nhánh với step label, và down theo sổ.
- Cải thiện so với phase 5:
  - `courses.Delete` giờ chạy trong tx có `Lock` FOR UPDATE, nên phần kiểm `COURSE_IN_PATH` được tuần tự hoá đúng với `LockCourses` FOR SHARE.
  - `sameIDSet` được tách thành `shared/idset` và dùng chung với `library`, có test riêng.
- Lệch nhỏ: xem L1 (cache chéo giữa hai bộ key trong cùng một feature) và L7 (picker không gate theo `courses.read`, trong khi header lớp có gate).

## Verdict

**SHIP.** Không có finding nào chặn merge. Tenancy, khoá đồng thời, RBAC, guard `COURSE_IN_PATH`, migration up/down và swagger đều đúng, và đã kiểm bằng đọc code cộng unit test, typecheck, lint, vitest. Integration suite chưa chạy vì cần Docker.

Nên làm trước phase 7, rẻ và có lợi cho người dùng: L1 (invalidate cache chéo) và L3 (refetch chi tiết khi mutation lỗi). L4 cần quyết định sản phẩm về khóa `draft`. L2, L5–L8 và các khoảng trống test có thể gom vào phase 9 (E2E, docs, ship).

## Disposition

Áp dụng ngay trong commit `13f92b3` (`fix(web): refresh learning path caches and name stage controls per stage`), có test đi trước ở `paths-cache.test.tsx` và `path-detail-page.test.tsx`:

- **L1 (cache chéo):** `useUpdatePath`/`useDeletePath` invalidate thêm `by-course`; `useUpdateCourse`/`useArchiveCourse`/`useDeleteCourse` invalidate `pathsKeys.all`.
- **L3 (mutation lỗi):** `useStageMutation` có `onError` invalidate `detail(pathId)`.
- **L5 (a11y):** nhãn nút và picker mang tên giai đoạn (`Đưa … lên/xuống`, `Sửa giai đoạn …`, `Xoá giai đoạn …`, `Thêm khóa học vào …`).
- **L7 (picker):** khi catalog lỗi, hiện một dòng `role="alert"` và khoá picker; chip khóa học chỉ là link khi có `courses.read`. Giới hạn 100 khóa giữ nguyên như phase 5.

Để lại, có lý do:

- **L2 (lost update):** cùng khuôn ghi đè danh sách với gói học phí (phase 5), cần thêm `version`/`updated_at` vào contract API. Ghi vào phase 9 làm điểm cân nhắc, không sửa trong phase này.
- **L4 (khóa `draft` trong lộ trình):** quyết định sản phẩm. Hiện giữ hành vi API nhận mọi trạng thái còn sống; web đã hiện badge trạng thái với mọi khóa khác `active`. Cần người dùng quyết định trước phase 9.
- **L6 (biên API rộng hơn web):** thừa hưởng từ `courses` (`classcode.Valid`) và không có rủi ro bảo mật; trần số giai đoạn để phase 9 xem cùng L2.
- **L8 (deep link 403):** parity với `/courses`; sửa chung ở phase 9 nếu còn thời gian.
- **Khoảng trống test** (đồng thời, binding HTTP, luồng lỗi web, chip archived): ghi vào phase 9. Riêng chip archived: MSW handler `PUT …/courses` hiện từ chối khóa khác `active`, lệch với API thật, cần chỉnh handler trước khi viết test đó.
- **Nit:** `apiMessage()` giữ nguyên vì `lib/api/errors` chưa có helper tương đương. Số đếm trong phase file sửa khi đóng phase.
