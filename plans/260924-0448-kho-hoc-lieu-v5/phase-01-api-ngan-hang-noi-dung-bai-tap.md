---
phase: 1
title: "API — ngân hàng nội dung & ngân hàng bài tập"
status: completed
priority: P1
effort: "1.5d"
dependencies: []
---

# Phase 1: API — ngân hàng nội dung & ngân hàng bài tập

## Context Links
- [plan.md](./plan.md) · D1, D3, D4, D5, D10 · v5 màn `lib` tab "Ngân hàng nội dung" / "Ngân hàng bài tập".
- Hiện trạng: [`../reports/scout-260924-1140-library-current-state.md`](../reports/scout-260924-1140-library-current-state.md) §Materials/Exercises.
- Migration mẫu: `apps/api/migrations/000032_template_lesson_prep.{up,down}.sql` (ALTER TABLE + comment tiếng Việt).

## Goal
Hai ngân hàng đủ cột cho bảng v5 (loại 7 giá trị, mã bài tập, kỹ năng, cấp độ, đếm "dùng trong", trạng thái
hoạt động) và có route bật/tắt hoạt động. Không đổi hợp đồng cũ: mọi field hiện có giữ nguyên tên và kiểu.

## Migration `000033_library_bank_fields`

```sql
-- up
ALTER TABLE library_materials DROP CONSTRAINT library_materials_kind_check;
ALTER TABLE library_materials
    ADD CONSTRAINT library_materials_kind_check
        CHECK (kind IN ('video', 'audio', 'image', 'doc', 'note', 'live', 'link', 'other')),
    ADD COLUMN active BOOLEAN NOT NULL DEFAULT true;

ALTER TABLE library_exercises
    ADD COLUMN code   VARCHAR(20),
    ADD COLUMN skill  VARCHAR(50),
    ADD COLUMN level  VARCHAR(50),
    ADD COLUMN active BOOLEAN NOT NULL DEFAULT true;

-- backfill mã theo thứ tự tạo trong từng trung tâm: BT-0001, BT-0002, …
WITH numbered AS (
    SELECT id, ROW_NUMBER() OVER (PARTITION BY center_id ORDER BY created_at, id) AS n
    FROM library_exercises
)
UPDATE library_exercises e SET code = 'BT-' || LPAD(n.n::text, 4, '0')
FROM numbered n WHERE n.id = e.id;

ALTER TABLE library_exercises ALTER COLUMN code SET NOT NULL;
CREATE UNIQUE INDEX uq_library_exercises_center_code
    ON library_exercises (center_id, code) WHERE deleted_at IS NULL;
CREATE INDEX idx_library_materials_center_active ON library_materials (center_id, active);
CREATE INDEX idx_library_exercises_center_active ON library_exercises (center_id, active);

-- down
DROP INDEX uq_library_exercises_center_code;
DROP INDEX idx_library_exercises_center_active;
DROP INDEX idx_library_materials_center_active;
ALTER TABLE library_exercises DROP COLUMN active, DROP COLUMN level, DROP COLUMN skill, DROP COLUMN code;
UPDATE library_materials SET kind = 'other' WHERE kind IN ('audio', 'image', 'note', 'live');
ALTER TABLE library_materials DROP COLUMN active;
ALTER TABLE library_materials DROP CONSTRAINT library_materials_kind_check;
ALTER TABLE library_materials ADD CONSTRAINT library_materials_kind_check
    CHECK (kind IN ('link', 'doc', 'video', 'other'));
```

Kiểm tra tên constraint thực tế bằng `\d library_materials` (Postgres tự đặt `library_materials_kind_check`
cho CHECK inline ở `000028`). Xoá mềm: index unique là partial nên dòng `deleted_at IS NOT NULL` không giữ mã.

## Model / DTO (`apps/api/internal/features/library/`)
- `model.go`: thêm hằng `MaterialKindVideo/Audio/Image/Doc/Note/Live/Link/Other` (giữ 4 hằng cũ, bổ sung 4 mới);
  `Material.Active bool`; `Exercise.Code string`, `Skill *string`, `Level *string`, `Active bool`.
  `MaterialRow`/`ExerciseRow` (nếu đã có) hoặc struct scan mới thêm `LessonCount int`, `TemplateCount int`.
- `dto.go`:
  - `MaterialRequest.Kind` binding `oneof=video audio image doc note live link` (line ~249; **bỏ `other`** khỏi input mới,
    dữ liệu cũ vẫn đọc được).
  - `MaterialResponse` + `active`, `lesson_count`, `template_count`.
  - `ExerciseRequest` + `code` (optional, `max=20`, regex `^[A-Za-z0-9-]+$` sau upper-case), `skill` (`max=50`), `level` (`max=50`).
  - `ExerciseResponse` + `code`, `skill`, `level`, `active`, `lesson_count`, `template_count`.
  - Mới: `ItemStatusRequest{ Active *bool \`binding:"required"\` }`.
- `errors.go`: 409 `EXERCISE_CODE_TAKEN` ("Mã bài tập đã tồn tại").

## Repository
- Đếm "dùng trong" bằng subselect (khuôn `versionSelect` thêm `lesson_count` ở `repository.go`):
  ```sql
  (SELECT count(*) FROM template_lesson_materials lm
     JOIN template_lessons l ON l.id = lm.lesson_id
     JOIN program_template_versions v ON v.id = l.version_id
     JOIN program_templates t ON t.id = v.template_id AND t.deleted_at IS NULL
    WHERE lm.material_id = library_materials.id) AS lesson_count,
  (SELECT count(DISTINCT t.id) FROM … cùng join …) AS template_count
  ```
  Tương tự cho `template_lesson_exercises`. Đếm qua mọi phiên bản (draft/published/archived) để khớp
  `MATERIAL_IN_USE`.
- `ListMaterials`/`ListExercises` nhận filter `active *bool` (query `?active=true`), `q` tìm thêm theo `code` (bài tập).
- `NextExerciseCode(ctx, sc) (string, error)`: `SELECT max(code)` theo pattern `BT-%` trong center (kể cả dòng đã xoá mềm,
  để không tái cấp mã), parse số, +1, format `BT-%04d`.
- `SetMaterialActive` / `SetExerciseActive`: `UPDATE … SET active = ?, updated_at = now() WHERE id = ? AND center_id = ? AND deleted_at IS NULL`.
- Unique violation trên `uq_library_exercises_center_code` → map sang `ErrExerciseCodeTaken` (cùng khuôn `CODE_TAKEN` của template).

## Service
- `CreateExercise`: `code` trống → gọi `NextExerciseCode` trong cùng tx với insert (retry 1 lần khi va unique).
  Không trống → `strings.ToUpper(strings.TrimSpace(code))`.
- `UpdateExercise`: cho đổi `code` (validate + unique).
- `SetMaterialStatus(ctx, sc, id, active)` / `SetExerciseStatus(...)` yêu cầu `library.edit`; trả response mới nhất.
- Picker cho buổi (`SetLessonMaterials` / `SetLessonExercises`): **từ chối** id đang `active = false` với 422
  `Invalid("Nội dung đã ngừng hoạt động")` — buổi đã gắn trước đó giữ nguyên.
- `DeleteMaterial`/`DeleteExercise` giữ 409 in-use.

## Routes (`routes.go` + `routespec.go` ~423–470)
| Method | Path | Perm | `req()` |
|---|---|---|---|
| PATCH | `/api/v1/library/materials/:id/status` | `library.edit` | `req("library_material.set_status", "library_material", "id")` |
| PATCH | `/api/v1/library/exercises/:id/status` | `library.edit` | `req("library_exercise.set_status", "library_exercise", "id")` |

Cập nhật `apps/api/internal/server/route_policy_snapshot_test.go` (+2) và
`apps/api/internal/features/audit/action_test.go` (+2).

## Tests
- `apps/api/migrations/migrations_test.go`: `TestLibraryBankFieldsBackfill` — insert 2 exercise (1 xoá mềm) trước 000033,
  chạy up, assert mã `BT-0001/BT-0002`, unique partial, `kind = 'audio'` chèn được; down → `kind` về `other`,
  cột biến mất. Cập nhật comment/đếm bước ở test roll-back (`MigrateDown(m, 28)` → 29 sau khi 000033, 30 sau 000034).
- `apps/api/internal/features/library/items_test.go` (unit, sqlmock/khuôn hiện có): tự sinh mã, mã trùng 409,
  status toggle, picker từ chối item inactive, list filter `active`.
- Integration (`make test-api`, serial): đếm `lesson_count`/`template_count` sau khi gắn vào 2 buổi của 1 template.

## Files
- Create: `apps/api/migrations/000033_library_bank_fields.up.sql`, `.down.sql`
- Modify: `library/model.go`, `dto.go`, `errors.go`, `repository.go`, `service.go`, `handler.go`, `routes.go`,
  `shared/routespec/routespec.go`, `server/route_policy_snapshot_test.go`, `features/audit/action_test.go`,
  `migrations/migrations_test.go`, `library/items_test.go`, `library/*_integration_test.go`.

## Verification
```bash
make migrate-up && make migrate-down && make migrate-up   # round-trip cục bộ (backup trước)
make test-api-unit
make scopelint
go test ./apps/api/migrations/... -run 'LibraryBankFields|LibraryItemsSchema' -p 1
```

## Risks / rollback
- Backfill mã chạy trên bảng nhỏ (ngân hàng theo trung tâm) → không cần batch.
- Down migration ép các `kind` mới về `other` (mất thông tin loại) — chấp nhận, prod dùng forward migration.

## Success Criteria
- [x] `POST /library/exercises` không có `code` → trả `BT-000N` kế tiếp; có `code` trùng → 409 `EXERCISE_CODE_TAKEN`.
- [x] `PATCH …/status {active:false}` ẩn item khỏi `?active=true`, bảng ngân hàng vẫn thấy với `active=false`.
- [x] `MaterialResponse`/`ExerciseResponse` có `lesson_count`, `template_count` đúng sau khi gắn/gỡ.
- [x] Snapshot route policy + audit action test xanh với 2 route mới; `CatalogVersion` vẫn 5.
