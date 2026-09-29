---
phase: 7
title: "Xoá bảng bộ điểm cũ"
status: completed
priority: P3
effort: "2h"
dependencies: [3, 4]
---

# Phase 7: Xoá bảng bộ điểm cũ

## Goal

Phase này thêm migration `000037` để xoá hai bảng `score_sets` và `score_set_components`, cùng cột truy vết `class_score_components.source_set_id`. Sau phase 3 và 4, không còn code nào đọc hay ghi các đối tượng này. Snapshot `class_score_components` và điểm `student_scores` không bị ảnh hưởng.

## Context

- `apps/api/migrations/000014_grading.up.sql` tạo:
  - `score_sets` (soft delete, unique index `score_sets_center_name_live`);
  - `score_set_components` (FK `set_id` ON DELETE CASCADE);
  - cột `class_score_components.source_set_id` (FK tới `score_sets` ON DELETE SET NULL, chỉ dùng để truy vết).
- Không migration nào sau 000014 tham chiếu hai bảng này. `score_set` ở 000028 và 000034 là cột JSONB của `program_template_versions`, là một thứ khác và được giữ.
- Migration mới nhất là `000036_drop_template_lesson_prep`. Đây là tiền lệ:
  - up xoá cấu trúc;
  - down dựng lại cấu trúc nhưng không khôi phục dữ liệu;
  - comment viết bằng tiếng Việt;
  - test ở `migrations_test.go:3216` (`TestDropTemplateLessonPrepSchemaAndPermission`).
- `apps/api/migrations/migrations_test.go:27-47` (`domainTables`) liệt kê `score_sets` và `score_set_components` ở :36. `TestMigrationRoundTrip` (:118) kiểm mọi bảng trong danh sách đều tồn tại sau khi migrate up.
- Sau phase 3, grep Go ngoài thư mục migrations không còn `score_sets`, `score_set_components` hay `source_set_id`. Trước phase 3, ba file `grading/model.go`, `service.go` và `repository.go` vẫn còn tham chiếu.
- `docs/deployment.md:104-110` yêu cầu backup DB trước `migrate up` trên prod. Khi prod đã có dữ liệu ghi sau migration thì ưu tiên sửa tiến (forward-fix) thay vì `migrate down`.

## Key insights

- Phase này là tuỳ chọn. Có thể giữ hai bảng như schema chết mà không ảnh hưởng hành vi. Lợi ích của việc xoá là schema khớp với mô hình "mẫu là nguồn duy nhất" và không ai vô tình dùng lại bảng cũ.
- Phải chạy sau khi phase 3 và 4 đã ổn trên prod. Nếu cần lùi phase 3 thì code cũ vẫn còn bảng để dùng. Vì vậy phase này nên là một PR và một lần deploy riêng.
- Dữ liệu trong `score_sets` sẽ mất vĩnh viễn nếu không có backup. Snapshot của các lớp đã gán vẫn nằm ở `class_score_components`, nên chỉ mất danh mục bộ điểm gốc và liên kết truy vết.

## Requirements

- Up xoá cột và hai bảng theo thứ tự phụ thuộc, dùng `IF EXISTS` như 000036.
- Down dựng lại đúng cấu trúc của 000014: hai bảng rỗng, unique index và cột `source_set_id` nullable có FK SET NULL. Down không khôi phục dữ liệu và phải nói rõ điều đó trong comment.
- Test migration chứng minh:
  - up chạy được khi có dữ liệu cũ;
  - snapshot và điểm còn nguyên sau up;
  - down rồi up lại vẫn sạch.

## Related files

**Create**

- `apps/api/migrations/000037_drop_score_sets.up.sql`
- `apps/api/migrations/000037_drop_score_sets.down.sql`

**Modify**

- `apps/api/migrations/migrations_test.go`: sửa `domainTables` ở :36 và thêm một test mới đặt sau `TestDropTemplateLessonPrepSchemaAndPermission`.

**Delete**: không có.

## Implementation steps

1. **Up.** Comment đầu file giải thích bằng tiếng Việt: bộ điểm giờ lấy từ mẫu chương trình và được chép vào lớp khi áp dụng, nên danh mục bộ điểm cấp trung tâm và cột truy vết không còn được dùng.
   ```sql
   ALTER TABLE class_score_components DROP COLUMN IF EXISTS source_set_id;
   DROP TABLE IF EXISTS score_set_components;
   DROP TABLE IF EXISTS score_sets;
   ```
   Xoá cột trước để gỡ FK, rồi xoá bảng con, cuối cùng xoá bảng cha. `DROP TABLE score_sets` tự xoá index `score_sets_center_name_live`.
2. **Down.** Chép nguyên văn DDL của `score_sets`, `score_sets_center_name_live` và `score_set_components` từ 000014. Sau đó chạy `ALTER TABLE class_score_components ADD COLUMN source_set_id UUID REFERENCES score_sets(id) ON DELETE SET NULL;`. Comment ghi rõ: dựng lại cấu trúc để binary cũ chạy được, còn danh mục bộ điểm và liên kết truy vết đã xoá ở up thì không được khôi phục.
3. **`domainTables`.** Gỡ `"score_sets"` và `"score_set_components"` khỏi :36, giữ `"class_score_components"`.
4. **Test mới `TestDropLegacyScoreSetTables`.** Làm theo khuôn `TestDropTemplateLessonPrepSchemaAndPermission`:
   - `startBarePostgres`, `MigrateUp`, rồi `m.Migrate(36)`;
   - seed một trung tâm, một lớp, một `score_sets` với một thành phần, một `class_score_components` có `source_set_id` trỏ tới bộ đó, một buổi và một `student_scores`;
   - `MigrateUp`;
   - kiểm `to_regclass('score_sets')` và `to_regclass('score_set_components')` là NULL;
   - kiểm cột `source_set_id` không còn trong `information_schema.columns`;
   - kiểm snapshot và điểm vẫn còn đúng một dòng mỗi loại;
   - `m.Migrate(36)` để chạy down, kiểm hai bảng tồn tại và rỗng, cột `source_set_id` tồn tại với giá trị NULL;
   - `MigrateUp` lần nữa không lỗi.
   Dùng helper seed sẵn có (`seedTeachingParents` và các helper tương tự) thay vì viết mới.
5. **Kiểm tay trên stack e2e cô lập.** Chạy `make migrate-up`, rồi `make migrate-down`, rồi `make migrate-up` với biến môi trường trỏ vào DB của compose `-p teka-e2e`, tuyệt đối không trỏ vào container `teka-*` của prod.
6. **Trình tự prod** (người vận hành làm sau khi được duyệt):
   1. Backup DB theo `docs/deployment.md:104-110`.
   2. Deploy bản build có 000037.
   3. Chạy `migrate up`.
   4. Kiểm bằng `\d class_score_components`.

## Todo

- [x] `000037_drop_score_sets.up.sql`
- [x] `000037_drop_score_sets.down.sql`
- [x] `domainTables` và `TestDropLegacyScoreSetTables`
- [x] Round-trip up → down → up: thay bằng `TestDropLegacyScoreSetTables` và `TestMigrationRoundTrip` trên Postgres 16 thật (testcontainers), không dựng lại stack e2e
- [x] Backup và migrate trên prod (sau khi được duyệt)

## Verification

- `rg -n "score_sets|score_set_components|source_set_id" apps/api --glob '!migrations/**'` không trả kết quả nào. Đây là điều kiện tiên quyết, đạt được sau phase 3.
- `cd apps/api && go test ./migrations/ -run 'TestDropLegacyScoreSetTables|TestMigrationRoundTrip' -p 1`, cần Docker.
- `make test-api`, chạy riêng.
- `make lint-api`

## Risk assessment

| Rủi ro | Khả năng × Tác động | Giảm thiểu |
|---|---|---|
| Mất danh mục bộ điểm gốc | Chắc chắn × Thấp | Đây là chủ đích. Snapshot lớp vẫn còn. Backup trước khi migrate |
| Deploy 000037 khi phase 3 chưa lên, code cũ truy vấn bảng đã mất | Thấp × Cao | Phase 7 là PR riêng và chỉ merge sau khi phase 3 và 4 đã chạy ổn trên prod. Kiểm bằng lệnh `rg` ở Verification |
| Down không khôi phục dữ liệu | Chắc chắn × Thấp | Ghi rõ trong comment. Khi cần dữ liệu thì khôi phục từ backup |
| Khoá bảng lúc DROP | Thấp × Thấp | Các bảng nhỏ. `DROP COLUMN` trên `class_score_components` chỉ đổi catalog |

## Security

Phase này không đổi quyền hay route. Việc xoá dữ liệu có chủ đích và được bảo vệ bằng backup bắt buộc. Migration chỉ chạy trên prod khi người dùng đã duyệt.

## Rollback

- Trước khi có dữ liệu mới phụ thuộc: chạy `migrate down` tới 36 để dựng lại bảng rỗng, rồi khôi phục dữ liệu từ backup nếu cần.
- Sau khi prod đã chạy lâu: sửa tiến theo `docs/deployment.md`.
- Revert commit chỉ gỡ migration khỏi binary. Nếu DB đã ở version 37 thì phải chạy `migrate down` trước khi deploy binary cũ.

## Open decisions

Xoá bảng (phase này) hay giữ làm schema chết. Plan đề xuất xoá, nhưng chỉ khi phase 3 và 4 đã ổn trên prod và đã có backup.

Trạng thái 2026-09-29 23:59: phase 3 và 4 đã lên prod lúc 23:53 (ảnh `teka-{api,web}:71e0f01-260929-2353`, schema vẫn là 36, backup `~/teka-backups/teka-prod-260929-2353.dump`). Migration 000037 đã nằm trên nhánh và test xanh; `TestDownFoldsPersonalChannelIntoManual` phải lùi 33 bước thay vì 32 vì có thêm 000037. Người dùng duyệt ngày 2026-09-30. Trước đó prod có 5 `score_sets`, 23 `score_set_components` và 19 dòng `class_score_components` mang `source_set_id`. Đã backup `~/teka-backups/teka-prod-260930-0344.dump` (có dữ liệu hai bảng), deploy ảnh `teka-{api,web}:294f055-260930-0344` và migrate lên schema 37 (không dirty). Sau migrate: hai bảng không còn, cột `source_set_id` không còn, 19 đầu điểm và 7 điểm giữ nguyên, `/readyz` trả 200 cả trong mạng nội bộ lẫn qua URL public.
