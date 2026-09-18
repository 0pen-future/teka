# Code review — Phase 1: API move có vị trí (`after_task_id`) + renormalize

- Ngày: 2026-09-17 16:26 (Asia/Saigon)
- Phạm vi: diff chưa commit của `apps/api` (14 file sửa + 1 file test mới), spec `plans/260917-1515-task-dnd-rich-text/phase-01-api-move-position.md`
- Chỉ review, không sửa mã nguồn.

## Tóm tắt

Cài đặt bám sát spec: core `pkg/kanban` giữ nguyên ranh giới (không thêm
dependency), hàm thuần `positionAfter` tách bạch khỏi I/O, repository scope
`center_id` đầy đủ, mapping lỗi và swagger đúng khuôn hiện có, hợp đồng
`MoveTaskRequest` tương thích ngược. Cả 4 AC đều có bằng chứng test.

Một vấn đề thật cần xử lý trước khi Phase 2 (kéo-thả) lên: khoá advisory chỉ
được lấy ở nhánh `after != nil`. Nhánh `after == nil` (thả lên đầu cột — một
thao tác kéo-thả rất phổ biến) chạy trong transaction nhưng **không khoá**, nên
vừa không đạt yêu cầu "hai request move đồng thời vào cùng cột phải tuần tự
hoá" trong mục Requirements của spec, vừa mở ra một lost-update mới do
`RenormalizeColumn` sinh ra.

## Bảng AC ↔ bằng chứng

| AC | Kết quả | Bằng chứng |
|----|---------|-----------|
| AC1 — body `{column_id}` (vắng hoặc `after_task_id: null`) → min−1 như v1 | Đạt | `pkg/kanban/service.go:419` trả về `topPositionInColumn`; `internal/features/tasks/service.go:275-279` chỉ set `after` khi `Set && Value != nil`; test `TestMoveTaskWithoutAfterKeepsTopPlacement` (cả hai body absent/null, service_test.go), `TestMoveTaskWithoutAfterLandsAtTop` (core), `TestMoveTaskWithoutAfterStillLandsAtTop` (integration, position 4.0) |
| AC2 — đặt ngay sau anchor, cùng cột lẫn khác cột; `CompletedAt` theo `IsDone` | Đạt | `positionAfter` `pkg/kanban/service.go:460-479` (bỏ qua chính `moving` khi tìm successor; anchor cuối → `+1`); `MoveTask` giữ nguyên `completedAtIf(clock, col.IsDone)` tại `service.go:326`; test `TestMoveTaskAfterReordersWithinTheSameColumn`, `TestMoveTaskAfterPlacesBetweenNeighboursAcrossColumns` (core), `TestMoveTaskAfterReordersWithinAColumn` + `TestMoveTaskAfterAcrossColumns` (integration, assert `CompletedAt != nil`) |
| AC3 — anchor không hợp lệ → 422 `fields.after_task_id`; chuỗi không UUID → 400 | Đạt | `pkg/kanban/errors.go` `ErrInvalidAfterTask` wrap `ErrInvalidInput`; `internal/features/tasks/errors.go:74-75` đặt **trước** case `ErrInvalidInput` chung → `apperror.Invalid` = HTTP 422 (`apperror.go:58`); chuỗi hỏng → `json.Unmarshal` lỗi (không phải `validator.ValidationErrors`) → `validation.BindError` → `apperror.BadRequest` 400 (`validation.go:82`); test `TestMoveTaskAfterRejectsAnchorsOutsideTheDestination` (core + integration, đủ 5 biến thể gồm tenant khác và soft-deleted), `TestMoveTaskAfterOutsideTheDestinationIs422`, `TestMoveTaskRequestRejectsNonUUIDAfterTask` |
| AC4 — chèn lặp → renormalize, gap ≥ 1e-6; 2 move đồng thời cùng anchor không trùng position | Đạt (trong phạm vi nhánh `after`) | `TestMoveTaskRenormalizesAfterRepeatedInsertsBetweenTheSamePair` (45 lần chèn cùng một cặp, assert thứ tự chính xác + mọi cặp kề `>= 1e-6`); `TestConcurrentMovesAfterTheSameAnchorNeverCollide` (2 goroutine, assert position tăng nghiêm ngặt — test này thật sự chứng minh khoá vì pool test là 5 kết nối, `internal/testutil/postgres.go:64`). Xem H-1: nhánh `after == nil` không nằm trong bảo đảm này. |

## Checklist a–e

| # | Hạng mục | Kết quả |
|---|----------|---------|
| a | Mọi AC được đáp ứng | Đạt (bảng trên); riêng yêu cầu "mọi move đồng thời vào cùng cột tuần tự hoá" trong Requirements chỉ đạt một nửa — H-1 |
| b | Không hồi quy touchpoint | Đạt. `grep '\.MoveTask('` chỉ có 1 caller sản phẩm (`internal/features/tasks/service.go:280`), phần còn lại là test. `DeleteColumn`, `HandoverOnDeparture`, `ReorderColumns` không đổi một dòng (diff sạch); `Board` chỉ đổi `sort.Slice` → `sort.SliceStable`, an toàn vì `ListBoard` đã order `column_id, position, created_at` nên tie-break `created_at` được giữ. Test `TestHandoverOnDepartureUnassignsAndReassignsWithoutTxOrEvent` (uow.calls == 0) vẫn xanh |
| c | Hợp đồng công khai | Đạt. `MoveTaskRequest` chỉ thêm field optional, `column_id` vẫn `binding:"required"` → client cũ không đổi (test "absent" chứng minh). Swagger `docs.go`/`swagger.json`/`swagger.yaml` đồng bộ cùng nội dung (description mới + 422 + `after_task_id: string`). Không migration, không schema DB mới, không env/config mới |
| d | Theo pattern hiện có | Đạt. Advisory lock cùng khuôn `grading/repository.go:187` và `tasks/column_repository.go:78`; hai query mới đều `center_id = ?` (scope tay, `make scopelint` sạch); `Optional[uuid.UUID] swaggertype:"string"` trùng khuôn `UpdateTaskRequest.AssigneeID` (`dto.go:166`); doc comment port và README "Position strategy" được viết lại đúng hành vi |
| e | Không lỗi lint/biên dịch mới | Đạt. `go build ./...` = 0; `go vet -tags=integration ./...` = 0; `make lint-api` (kèm `scopelint`) → `0 issues`; `go test -short -count=1 ./pkg/kanban/... ./internal/features/tasks/...` → ok/ok. Log `make test-api` chạy đơn lẻ (16:24, sau khi file test mới tạo lúc 16:11) không có dòng FAIL nào, `features/tasks` ok 8.93s |

## Phát hiện

### High

**H-1 — SIDE EFFECT: nhánh `after == nil` không lấy advisory lock, `RenormalizeColumn` có thể ghi đè một move-lên-đầu đồng thời**

- `pkg/kanban/service.go:418-421` — `positionInColumn` return sớm sang
  `topPositionInColumn` (→ `MinPositionInColumn`) trước khi chạm tới
  `ListColumnPositions`, nên transaction của một move "lên đầu cột" **không giữ
  khoá nào** trên cột đích.
- Hệ quả 1 (yêu cầu spec): mục Requirements viết "Hai request move đồng thời vào
  cùng cột phải tuần tự hoá". Hiện chỉ move có `after` mới tuần tự hoá. Hai
  move-lên-đầu đồng thời cùng đọc `MIN(position)` dưới READ COMMITTED và cùng
  ghi `min−1` → hai task trùng position. Đây là hành vi đã có từ v1, nhưng
  Phase 2 biến "thả lên đầu cột" thành thao tác thường xuyên (drop ở index 0
  gửi `after_task_id: null`), nên xác suất chạm không còn là lý thuyết.
- Hệ quả 2 (mới do phase này sinh ra): tx A (`after` set) giữ khoá, liệt kê cột,
  rồi gọi `RenormalizeColumn` ghi `0..n-1` theo `order` đã chụp. Tx B
  (`after == nil`, cùng cột, task đang nằm sẵn trong cột đó) không bị khoá, ghi
  `position = min−1` cho task X và commit. Vì X nằm trong `order` của A, lệnh
  `UPDATE ... WHERE id = X` của A ghi đè position vừa ghi bằng chỉ số cũ →
  **thao tác kéo lên đầu của người dùng bị âm thầm hoàn tác** (chỉ thứ tự, không
  mất dữ liệu; không tự phục hồi cho tới khi người dùng kéo lại).
- Đề xuất: lấy cùng một advisory lock cho cả nhánh `nil`. Rẻ nhất là bỏ
  return sớm và luôn gọi `ListColumnPositions`, tính top từ `rows[0].Position − 1`
  (rỗng → 0) — bỏ luôn được `MinPositionInColumn` khỏi đường move, giữ
  `CreateTask` nguyên trạng. Nếu muốn giữ `MinPositionInColumn`, cần thêm một
  port khoá riêng và gọi trước, nhưng cách đó thêm bề mặt port mà không lợi hơn.
- Ghi chú phụ: README mục "Position strategy" và
  `pkg/kanban/ports.go:96-101` đang khẳng định tuyệt đối "No two neighbours in a
  column are therefore ever closer than `minPositionGap`". Với H-1 chưa sửa,
  câu này sai khi có hai move-lên-đầu đồng thời (gap = 0). Sửa H-1 thì câu văn
  đúng; nếu quyết định không sửa, cần hạ giọng câu khẳng định.

### Medium

**M-1 — `RenormalizeColumn` trả `ErrTaskNotFound` → move hợp lệ nhận 404 gây hiểu nhầm**

- `internal/features/tasks/task_repository.go:142-144`. `DeleteTask` (soft
  delete) **không** lấy advisory lock, nên một task nằm trong `order` có thể
  biến mất giữa `ListColumnPositions` và vòng update → `RowsAffected == 0` →
  `ErrTaskNotFound` → handler trả 404 "task not found" cho một request move mà
  task nguồn vẫn còn sống. Transaction rollback nên không có hỏng dữ liệu, nhưng
  client (Phase 2) rất dễ diễn giải 404 thành "thẻ này đã bị xoá" và gỡ thẻ
  đang kéo khỏi board.
- Đề xuất: renormalize chỉ là đánh số lại; bỏ qua dòng không khớp
  (`continue` khi `RowsAffected == 0`) là an toàn và đúng ngữ nghĩa hơn — dòng
  biến mất đúng là dòng không cần đánh số. Nếu muốn giữ tính nghiêm ngặt, trả
  một lỗi riêng ánh xạ 409/retry thay vì 404.

### Low

**L-1 — doc comment cũ ở feature service**: `internal/features/tasks/service.go:273`
vẫn ghi "MoveTask changes a task's column, landing it at the top of the target",
trái với hành vi mới. Sửa một dòng.

**L-2 — ngưỡng `minPositionGap` là tuyệt đối, không tương đối**:
`pkg/kanban/service.go:472-473` so `gap/2 < minPositionGap`. Với `|position|`
cỡ 1e10 trở lên, `r.Position + gap/2` làm tròn về đúng `r.Position` trong khi
`gap/2` vẫn ≥ 1e-6 → hai task trùng position mà không renormalize. Không đạt
được trong thực tế (cần ~1e10 lần chèn đầu/đuôi cột), nên chỉ là ghi chú: nếu
muốn chắc tuyệt đối, kiểm `pos <= r.Position || pos >= next.Position` thay cho
so sánh gap.

**L-3 — `RenormalizeColumn` bump `updated_at` của mọi task trong cột**:
`taskModel.UpdatedAt` (`model.go:52`) là field GORM tự cập nhật theo tên, nên
`Update("position", ...)` cũng ghi `updated_at = now()`. Task không ai sửa vẫn
đổi `updated_at` trong response/cache. Hiếm (chỉ khi renormalize) và nhất quán
với các method khác trong file, nhưng đáng biết khi Phase 2 dùng `updated_at`
cho optimistic UI hay polling.

**L-4 — deadlock lý thuyết với `DeleteColumn`**: renormalize update nhiều dòng
theo thứ tự position, còn `MoveAllToColumn` (`task_repository.go:245`) update cả
cột theo thứ tự do planner chọn, và `DeleteColumn` không lấy advisory lock theo
cột. Hai tx giao nhau trên cùng cột có thể deadlock ở row lock → Postgres huỷ
một tx (40P01) → 500. Rất hiếm (xoá cột là thao tác owner, hiếm) và không hỏng
dữ liệu; ghi nhận để quyết định sau, không nên mở rộng phạm vi phase này.

**L-5 — `hashtext` 32-bit dùng chung không gian advisory**: key có tenant
(`tenant:column`) nên không lẫn giữa các trung tâm về mặt ngữ nghĩa; va chạm
hash chỉ gây tuần tự hoá thừa, không gây sai. Không có đường nào giữ hai advisory
lock cùng lúc (đã kiểm `grading/repository.go`, `tasks/column_repository.go`,
`imports/lock.go`) nên không có chu trình khoá. Chấp nhận được, đúng khuôn repo.

**L-6 — không có test ở tầng HTTP thật**: `TestMoveTaskRequestRejectsNonUUIDAfterTask`
gọi thẳng `json.Unmarshal` + `validation.BindError`, đúng những gì handler làm
(`handler.go:340-343`), nhưng không đi qua gin. Package `tasks` xưa nay không có
`handler_test.go` nên đây không phải hồi quy; chỉ là khoảng trống còn lại nếu
sau này ai đó đổi cách bind.

**L-7 — test đồng thời và pool 5 kết nối**: `TestConcurrentMovesAfterTheSameAnchorNeverCollide`
giữ một kết nối suốt tx trong khi chờ advisory lock. Với 2 goroutine và pool 5
thì an toàn; nếu sau này thêm test đồng thời song song hơn trong cùng package,
cần để ý cạn pool. (`make test-api` đã chạy `-p 1` theo thông lệ dự án.)

## Điểm đáng ghi nhận (hiệu chỉnh rủi ro)

- `positionAfter` là hàm thuần, bảng test phủ đủ: anchor cuối, anchor giữa,
  anchor = moving, anchor vắng, rows rỗng, reorder cùng cột (bỏ qua chính nó,
  kể cả khi nó là successor duy nhất), gap dưới sàn, và **gap đúng bằng sàn**
  (`2 * minPositionGap` → không renormalize). Đây là loại test chứng minh bất
  biến chứ không chỉ chạy qua mã.
- Hai port mới đều scope `center_id` trong `WHERE`, và bài test tenant khác ở
  tầng integration (`another tenant` → 422) là lưới bảo vệ đúng chỗ mà spec đã
  chỉ ra là `scopelint` không soi được.
- Transaction boundary đúng như spec: `ListColumnPositions` → (renormalize) →
  `Update` nằm trong cùng một `uow.Within`; `GormTxManager.WithinTx` gộp tx lồng
  nhau nên không có rủi ro tx lồng, và `database.FromContext` bảo đảm cả khoá
  advisory lẫn các UPDATE dùng đúng handle tx (khoá `pg_advisory_xact_lock` mới
  có ý nghĩa).

## Hành động đề xuất (theo thứ tự)

1. H-1: lấy advisory lock cho cả nhánh `after == nil` (đề xuất: bỏ return sớm,
   tính top từ `ListColumnPositions`), rồi bổ sung một integration test "một
   move-lên-đầu chạy song song với một move có `after` gây renormalize" —
   không có test này thì lỗi âm thầm hoàn tác sẽ không ai thấy.
2. M-1: đổi `RowsAffected == 0` trong `RenormalizeColumn` thành bỏ qua dòng, hoặc
   ánh xạ sang mã lỗi không phải 404.
3. L-1: sửa doc comment `internal/features/tasks/service.go:273`.
4. L-2/L-3/L-4/L-5: ghi nhận, không cần hành động trong phase này.

## Câu hỏi chưa giải quyết

1. H-1 có nằm trong phạm vi Phase 1 không, hay đẩy sang Phase 2 khi web bắt đầu
   gửi drop-at-top? Yêu cầu trong spec Phase 1 viết là "mọi move đồng thời vào
   cùng cột", nên mặc định tôi coi nó thuộc Phase 1.
2. Với M-1, dự án muốn move kiểu "best effort" (bỏ qua dòng vừa bị xoá) hay
   "strict" (huỷ move, báo client thử lại)? Quyết định này thuộc về sản phẩm.
3. Khi cột đích đã chạm cap hiển thị 50 việc, client chỉ thấy 50 thẻ đầu; thả
   xuống dưới thẻ thứ 50 sẽ đặt task lên **trên** phần bị ẩn. Hành vi này đúng
   theo cài đặt nhưng chưa thấy spec nói tới — cần chốt ở Phase 2.

Status: DONE_WITH_CONCERNS
Summary: Cả 4 AC đều đạt và có bằng chứng test; biên dịch, vet (kèm tag integration), lint/scopelint và unit test đều sạch. Còn một vấn đề mức High: nhánh `after == nil` không lấy advisory lock nên không đạt yêu cầu tuần tự hoá trong spec và để `RenormalizeColumn` âm thầm ghi đè một move-lên-đầu đồng thời.
Concerns/Blockers: H-1 (khoá thiếu ở nhánh top + lost update do renormalize) nên sửa trước khi Phase 2 phát sinh thao tác thả lên đầu cột; M-1 (`RenormalizeColumn` trả 404 khi có DeleteTask đồng thời) nên sửa cùng lượt.
