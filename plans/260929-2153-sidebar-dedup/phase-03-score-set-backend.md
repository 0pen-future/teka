---
phase: 3
title: "Bộ điểm: backend chép khi áp dụng mẫu"
status: completed
priority: P2
effort: "6h"
dependencies: []
---

# Phase 3: Bộ điểm — backend chép khi áp dụng mẫu

## Goal

Sau phase này, `program_template_versions.score_set` là nguồn duy nhất cho bộ điểm. Cụ thể:

- Khi chủ trung tâm áp dụng một phiên bản mẫu cho lớp (PUT `/classes/:id/program`), các thành phần điểm của phiên bản đó được làm phẳng và chép vào `class_score_components`, trong cùng transaction với việc áp dụng.
- Sáu route `/score-sets` và `/classes/:id/score-set` bị gỡ cùng mã CRUD của chúng.
- Snapshot và điểm đang có của các lớp được giữ nguyên.

## Context

- `apps/api/internal/features/classprogram/service.go`:
  - `Apply` ở :109-168 làm trong một `WithinTx` các bước: khoá template, đọc `PublishedVersion`, khoá curriculum, `Upsert` rồi `PutCurriculum`.
  - `Remove` ở :171-187 chỉ gỡ liên kết.
  - Interface `LibrarySource` ở :46-53 và `NewService` ở :65.
- `apps/api/internal/features/grading/service.go`:
  - `AssignScoreSet` ở :198-248 theo trình tự `LockClassForScoring` → `guardNoScores` → `ReplaceClassComponents`, tất cả trong một tx;
  - `ClearScoreSet` ở :253-276;
  - `guardNoScores` ở :470, `buildSetComponents` ở :512, `ownerOnly` ở :584, `classHasScores` ở :608;
  - CRUD bộ điểm ở :79-196.
- `apps/api/internal/features/grading/repository.go`: `ReplaceClassComponents` ở :160 (xoá hết rồi chèn lại), `ClassHasScores` ở :171, `LockClassForScoring` ở :179 (advisory xact lock theo lớp).
- `apps/api/internal/database/tx.go:22`: "Nested calls join the ambient transaction". Vì vậy tx của grading sẽ nhập vào tx của `Apply`.
- `apps/api/internal/features/library/model.go:322-341` định nghĩa `ScoreComponent{Key, Label, Max, Weight}` và `ScoreSetGroup{Key, Title, Components}`.
  - `SetScoreSet` (service.go:1450) giới hạn tối đa 10 nhóm và 20 thành phần mỗi nhóm.
  - Key là duy nhất trong mỗi nhóm. Label không được rỗng nhưng không giới hạn độ dài và có thể trùng.
  - `VersionResponse` (dto.go:90-103) không mang `score_set`, còn `repo.GetVersion` trả về `VersionRow` có `ScoreSet`.
- `apps/api/migrations/000014_grading.up.sql` định nghĩa `class_score_components(name VARCHAR(50), position SMALLINT, source_set_id → score_sets ON DELETE SET NULL, UNIQUE(class_id,name), UNIQUE(class_id,position))`. `student_scores` trỏ FK `(component_id, class_id)` với ON DELETE CASCADE.
- Nơi khởi tạo `classprogram.NewService`:
  - `apps/api/internal/server/router.go:253`, nằm trước `grading.NewService` ở :266;
  - `apps/api/internal/features/classprogram/integration_test.go:56`;
  - `apps/api/seeds/teaching_menu.go:92`;
  - `apps/api/internal/features/classprogram/handler_test.go:94`, là bản stub.
- Các test và manifest đang tham chiếu route bộ điểm:
  - `apps/api/internal/shared/routespec/routespec.go:171`, :222-227;
  - `apps/api/internal/server/route_policy_snapshot_test.go:47-52`;
  - `apps/api/internal/server/route_policy_test.go:69`, :84-89;
  - `apps/api/internal/server/policy_integration_test.go:148-153`;
  - `apps/api/internal/features/audit/action_test.go:17-21`, :46, :64, :94, :112-113.
- Seed không bị ảnh hưởng: `seeds/teaching_menu.go:308-325` đặt `score_set` cho bản nháp. Phiên bản đã phát hành mà seed áp dụng không có bộ điểm, nên bước chép là no-op.

## Thiết kế đề xuất: chép khi áp dụng

Luồng dữ liệu như sau. PUT `/classes/:id/program` đi vào `classprogram.Apply`, rồi chạy trong tx:

1. `library.LockTemplateForVersion`
2. `PublishedVersion` và `PublishedScoreSet`
3. khoá và đọc curriculum
4. `Upsert` class_programs
5. `PutCurriculum`
6. `grading.SyncTemplateComponents`
7. commit

**Quy tắc làm phẳng (a).**

- Duyệt các nhóm theo thứ tự, trong mỗi nhóm duyệt thành phần theo thứ tự. Position gán lần lượt 0..N-1, giống `buildSetComponents` hiện nay.
- Tên thành phần:
  - Nếu chỉ có đúng một nhóm có thành phần, tên là `label` đã trim.
  - Nếu có từ hai nhóm trở lên, tên là `"{title} · {label}"`.
- Cắt tên còn tối đa 50 rune, vì `VARCHAR(50)` đếm theo ký tự.
- Nếu hai tên trùng nhau (so không phân biệt hoa thường, giống `normalizeComponentNames`), tên sau được thêm hậu tố `" (2)"`, `" (3)"`, … Phần gốc bị cắt ngắn để tổng độ dài vẫn không quá 50 rune.
- `max` và `weight` không được chép, vì snapshot không có cột cho chúng và thang điểm cố định 0–10 (`validateScoreEntries`). Đây là giới hạn đã biết.

**Áp dụng lại khi lớp đã có điểm (b).** Quy tắc đi theo tinh thần "chưa có điểm mới được đổi" của `AssignScoreSet`:

- Nếu danh sách phẳng rỗng (mẫu không có bộ điểm), giữ nguyên snapshot hiện tại. Kết quả là `kept_empty_template`.
- Nếu lớp đã có ít nhất một điểm, giữ nguyên snapshot và điểm. Việc áp dụng chương trình vẫn thành công. Kết quả là `kept_scored`.
- Nếu tên và vị trí trùng với snapshot hiện tại, không ghi gì. Kết quả là `unchanged`.
- Các trường hợp còn lại: thay toàn bộ snapshot. Kết quả là `replaced`.
- Gỡ chương trình (`Remove`) không đụng snapshot, cũng như không đụng curriculum.

**Các phương án khác cho (b).**

- Từ chối bằng 409 `CLASS_HAS_SCORES` khi lớp có điểm và bộ điểm khác. Cách này làm lớp đang học không đổi được chương trình (kể cả curriculum), nên kém hơn.
- Thêm cờ `confirm` để xoá điểm rồi thay. Cách này nguy hiểm vì CASCADE xoá điểm đã chấm, và không ai yêu cầu.

**Các phương án khác cho toàn bộ thiết kế.**

- Đọc trực tiếp JSONB của mẫu khi chấm điểm. Cách này cần đổi schema `student_scores` sang key và mất tính ổn định của snapshot. Bị loại.
- Giữ trang cấu hình nhưng đọc từ mẫu. Cách này vẫn còn hai bề mặt, trái với quyết định. Bị loại.

## Requirements

- `library.Service` có thêm `PublishedScoreSet(ctx, sc, versionID) ([]ScoreSetGroup, error)`. Hàm đọc `repo.GetVersion` và chỉ chấp nhận trạng thái `published`; lỗi dùng chung `notFound` và `errVersionNotPublishedForClass` như `versionForClass`.
- `grading.Service` có thêm `SyncTemplateComponents(ctx, sc, classID, groups []TemplateScoreGroup) (SnapshotOutcome, error)`.
  - Hàm chỉ cho chủ trung tâm và dùng `resolveClass`.
  - Trong tx thực hiện: `LockClassForScoring`, rồi `ClassHasScores`, rồi `GetClassComponents`, rồi so sánh, rồi `ReplaceClassComponents`.
  - `TemplateScoreGroup{Title string; Labels []string}` là kiểu grading tự định nghĩa, để grading không phải import library.
- `classprogram` có interface `ScoreSnapshotStore` và gọi nó ở cuối closure tx của `Apply`.
- Gỡ các route, handler, service, repository, DTO, model và lỗi chỉ phục vụ CRUD, assign và clear bộ điểm. Riêng route GET `/classes/:id/score-components` và hai route điểm theo buổi được giữ.
- `ClassComponent.SourceSetID` bị gỡ khỏi model. Cột vẫn nằm trong DB và nullable cho tới phase 7.

## Related files

**Modify**

- `apps/api/internal/features/library/service.go`: thêm `PublishedScoreSet` cạnh `PublishedVersion` ở :194
- `apps/api/internal/features/grading/service.go`, `repository.go`, `handler.go`, `routes.go`, `dto.go`, `model.go`, `errors.go`
- `apps/api/internal/features/grading/service_test.go`, `grading_integration_test.go`, `staff_read_integration_test.go`
- `apps/api/internal/features/classprogram/service.go`, `handler.go` (swagger mô tả `apply`), `handler_test.go`, `integration_test.go`
- `apps/api/internal/server/router.go`: đưa `gradingSvc` lên trước `classprogramSvc` và truyền vào
- `apps/api/seeds/teaching_menu.go:92`: dựng `grading.NewService` và truyền vào
- `apps/api/internal/shared/routespec/routespec.go`
- `apps/api/internal/server/route_policy_snapshot_test.go`, `route_policy_test.go`, `policy_integration_test.go`
- `apps/api/internal/features/audit/action_test.go`
- `apps/api/docs/docs.go`, `swagger.json`, `swagger.yaml`: sinh lại bằng `make api-docs`

**Create**: không có. Test mới nằm trong các file test đã có.

**Delete**: không xoá file nào. Code bị gỡ nằm trong các file trên.

## Implementation steps

1. **library.** Thêm `PublishedScoreSet` với doc comment: bộ điểm của một phiên bản đã phát hành, dùng khi áp dụng cho lớp, không qua gate `library.read`, giống `PublishedVersion`. Trả `scoreSet(row.ScoreSet)` (service.go:1573) để không bao giờ trả nil.
2. **grading, phần thêm mới.**
   - Thêm các kiểu `TemplateScoreGroup`, `SnapshotOutcome` và bốn hằng outcome.
   - Viết hàm thuần `flattenTemplateComponents(groups) []string` theo quy tắc (a), dùng `unicode/utf8` để cắt theo rune.
   - Viết `SyncTemplateComponents`:
     - nếu `!sc.IsOwner` thì trả `ownerOnly()`, và đổi message thành "only the center owner can change class score components";
     - gọi `resolveClass`;
     - làm phẳng; nếu rỗng thì trả `kept_empty_template`;
     - trong `s.tx.WithinTx`: `LockClassForScoring`; nếu `ClassHasScores` thì trả `kept_scored`; so tên và vị trí với `GetClassComponents`, nếu bằng nhau thì trả `unchanged`; nếu không thì `ReplaceClassComponents` với `ID: id.New()`, `ClassID`, `CenterID: class.CenterID` và `Position: int16(i)`;
     - lỗi đi qua `txError`.
   - Doc comment nói rõ vì sao giữ snapshot khi đã có điểm: thay snapshot sẽ CASCADE xoá điểm.
3. **grading, phần gỡ.**
   - Trong service: gỡ `ListSets`, `CreateSet`, `UpdateSet`, `DeleteSet`, `AssignScoreSet`, `ClearScoreSet`, `guardNoScores`, `buildSetComponents`, `normalizeComponentNames`, `scoreSetNotFound`, `classHasScores`, `duplicateSetName`, và `componentInvalid` nếu không còn dùng.
   - Trong repository: gỡ `ListSets`, `GetSet`, `ListComponentsForSets`, `CreateSet`, `UpdateSet`, `SoftDeleteSet` và `ReplaceSetComponents`, cả trong interface lẫn trong phần cài đặt.
   - Trong handler: gỡ `listSets`, `createSet`, `updateSet`, `deleteSet`, `assignScoreSet` và `clearScoreSet`, kèm annotation swag của chúng.
   - Trong routes: gỡ nhóm `/score-sets` cùng POST và DELETE `/:id/score-set`, rồi sửa doc comment của `RegisterRoutes`.
   - Trong dto: gỡ `ScoreSetResponse`, `ScoreSetRequest` và `AssignScoreSetRequest`.
   - Trong model: gỡ `ScoreSet`, `SetComponent` và `ClassComponent.SourceSetID`, rồi sửa comment ở :20 và :48.
   - Trong errors: gỡ `ErrScoreSetNotFound` và `ErrClassHasScores`, sửa comment của `ErrOwnerOnly`.
   - Sửa doc comment của `Service` (:59-61) và các comment ở :173 và :278-282 để không còn nhắc "owner config page".
4. **classprogram.**
   - Thêm interface:
     ```go
     type ScoreSnapshotStore interface {
         SyncTemplateComponents(ctx context.Context, sc authctx.Scope, classID uuid.UUID, groups []grading.TemplateScoreGroup) (grading.SnapshotOutcome, error)
     }
     ```
   - Thêm `PublishedScoreSet` vào `LibrarySource`. Thêm trường `scores` và tham số mới cho `NewService`.
   - Trong closure của `Apply`, sau `PutCurriculum`: gọi `s.library.PublishedScoreSet`, đổi sang `[]grading.TemplateScoreGroup`, rồi gọi `s.scores.SyncTemplateComponents`. Nếu lỗi thì trả lỗi để tx rollback.
   - Sửa package doc (:1-6) và doc của `Apply`: áp dụng chương trình cũng đặt đầu điểm của lớp khi lớp chưa có điểm.
   - Sửa swagger `@Description` của `apply` trong handler.go (:108).
5. **Wiring.**
   - Trong `router.go`, đưa khối `gradingSvc` (:266-267) lên trước `classprogramSvc` (:253) và truyền `gradingSvc`. Sửa hai comment khối cho đúng thứ tự mới.
   - Làm tương tự trong `seeds/teaching_menu.go`, dùng `sessionsSvc` và `enrollmentsSvc` có sẵn ở :81-82.
   - Trong `handler_test.go`, thêm `stubScoreSnapshotStore` và `PublishedScoreSet` vào `stubLibrarySource`.
6. **routespec và policy.**
   - Gỡ :222-227 và sửa comment :170-171 thành "... sensitive review writes, and class program changes".
   - `route_policy_snapshot_test.go`: gỡ :47-52.
   - `route_policy_test.go`: thay sáu dòng score-set trong `frozen` bằng `"PUT /api/v1/classes/:id/program"` và `"DELETE /api/v1/classes/:id/program"`, rồi sửa comment :66-70.
   - `policy_integration_test.go:148-153`: đổi sang `GET /api/v1/centers/me/permissions`, kỳ vọng chủ trung tâm nhận 200 và thành viên nhận 403, kèm thông điệp "permission administration is owner-only".
   - `audit/action_test.go`: gỡ các dòng `score_set.*` và `class.score_set.*`.
7. **Test grading.**
   - `service_test.go`:
     - thay `TestNormalizeComponentNames` bằng `TestFlattenTemplateComponents`, bảng ca gồm: một nhóm; nhiều nhóm có tiền tố; nhãn dài 60 rune bị cắt còn 50; trùng khác hoa thường thành " (2)"; trùng khi đã cắt; rỗng;
     - `TestOwnerGatesShortCircuit` chỉ còn kiểm `SyncTemplateComponents`.
   - `grading_integration_test.go`:
     - xoá `TestScoreSetCRUDRoundTrip`, `TestScoreSetDuplicateName`, `TestScoreSetsAreOwnerOnly`, `TestAssignSnapshotIsIndependentOfSource`, `TestAssignAndClearRefusedWhenClassHasScores` và `TestAssignSoftDeletedSetIsNotFound`;
     - thêm `TestSyncTemplateComponentsReplacesUntilScored`: lớp chưa có điểm thì `replaced`; lặp lại thì `unchanged`; ghi một điểm rồi sync bộ khác thì `kept_scored`, snapshot và điểm vẫn còn; mẫu rỗng thì `kept_empty_template`;
     - fixture của `TestSessionScore*` chuyển từ `AssignScoreSet` sang `SyncTemplateComponents`.
   - `staff_read_integration_test.go`: fixture cũng chuyển sang `SyncTemplateComponents`.
8. **Test classprogram (`integration_test.go`).**
   - Fixture dựng thêm `grading.NewService` và truyền vào.
   - Thêm `TestApplyCopiesTemplateScoreSetIntoClassComponents`: mẫu hai nhóm được áp dụng thì `GET` score-components trả tên có tiền tố đúng thứ tự.
   - Thêm `TestReapplyKeepsComponentsOnceTheClassHasScores`.
   - Thêm `TestRemoveKeepsClassComponents`.
   - Thêm ca: khi ghi snapshot lỗi, tx rollback nên `class_programs` không đổi. Có thể mô phỏng bằng stub trả lỗi trong `handler_test.go` nếu integration khó dựng.
9. Chạy `make api-docs` để sinh lại swagger, rồi kiểm tra diff chỉ gồm các route bị gỡ và mô tả `apply`.

## Todo

- [x] `library.PublishedScoreSet`
- [x] `grading.SyncTemplateComponents` và `flattenTemplateComponents`
- [x] Gỡ CRUD, assign và clear bộ điểm khỏi grading
- [x] `classprogram.Apply` gọi snapshot store trong tx
- [x] Wiring ở router, seeds và stub test
- [x] routespec, policy tests và audit action test
- [x] Test grading và classprogram mới
- [x] `make api-docs`

## Verification

Chạy tuần tự, không song song với việc khác:

- `cd apps/api && go build ./... && go vet ./...`
- `make lint-api`
- `make test-api-unit` (gồm `go test -short ./...` và scopelint)
- `make test-api`, chạy riêng theo trình tự (`-p 1`) và cần Docker. Ngưỡng coverage 60.
- `make api-docs`, sau đó `git diff --stat apps/api/docs` chỉ thay đổi ở ba file sinh.
- `rg -n "score-sets|score_set\.|ScoreSetRequest|AssignScoreSet|SourceSetID" apps/api --glob '!migrations/**'` chỉ còn khớp ở `library` (`/library/versions/:vid/score-set`, `template_version.set_score_set`).

## Risk assessment

| Rủi ro | Khả năng × Tác động | Giảm thiểu |
|---|---|---|
| CASCADE xoá điểm khi thay snapshot | Thấp × Cao | Khoá theo lớp, kiểm `ClassHasScores` và thay snapshot trong cùng tx, như `AssignScoreSet`. Có test integration cho `kept_scored` |
| Tên vượt 50 rune hoặc trùng làm hỏng UNIQUE và rollback cả việc áp dụng | Trung bình × Trung bình | Cắt theo rune và thêm hậu tố dedup. Có bảng test đơn vị |
| Deadlock giữa `Apply` và `PutSessionScores` | Thấp × Trung bình | `PutSessionScores` chỉ lấy advisory lock theo lớp. `Apply` lấy khoá template và curriculum trước, rồi mới lấy advisory lock. Không có đường nào lấy các khoá theo thứ tự ngược lại |
| Phiên bản mẫu có tới 200 thành phần làm lưới điểm rất rộng | Thấp × Thấp | Giới hạn của library vẫn giữ. Đợt lưu điểm chỉ gửi các ô đã sửa, nên giới hạn 500 mục không bị chạm |
| Web cũ gọi `/score-sets` trong lúc deploy | Thấp × Thấp | Phase 4 lên cùng bản build. Nếu lỡ thì chỉ nhận 404 trên trang sắp bị gỡ |

## Security

- Chép snapshot chỉ xảy ra trong `Apply`, vốn là route `KindOwnerOnly` (`class_program.apply`). `SyncTemplateComponents` còn tự kiểm `IsOwner` lần nữa.
- Không thêm key quyền nào. Route đọc score-components vẫn giữ gate `scores.read`.
- Audit của việc áp dụng giữ action `class_program.apply`. Các dòng audit cũ mang action `score_set.*` vẫn còn và vẫn đọc được.

## Rollback

`git revert` commit của phase rồi deploy lại. Bảng `score_sets` và `score_set_components` vẫn còn cho tới phase 7, nên code cũ chạy lại bình thường. Các snapshot đã chép trong thời gian code mới chạy là dữ liệu hợp lệ theo schema cũ (`source_set_id` NULL).

## Open decisions

- Quy tắc làm phẳng: dấu phân cách `" · "`, tiền tố tên nhóm chỉ khi có từ hai nhóm, cắt còn 50 rune, hậu tố `" (n)"`.
- Áp dụng lại khi lớp đã có điểm: đề xuất giữ im lặng và API không báo gì thêm. Phương án khác là trả thêm trường `score_components_kept` để web hiện thông báo.
- `max` và `weight` của mẫu chưa được dùng khi chấm (thang 0–10 cố định).
