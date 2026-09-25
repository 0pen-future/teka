---
phase: 2
title: "API — chương trình mẫu, phiên bản, buổi học & web contract"
status: completed
priority: P1
effort: "2d"
dependencies: [1]
---

# Phase 2: API — chương trình mẫu, phiên bản, buổi học & web contract

## Context Links
- [plan.md](./plan.md) · D2, D6, D7, D8, D9, D10, D11 · v5 màn `td` (7 tab) và `ts` (Hình thức, Nhóm bài tập).
- Bất biến kế thừa: một draft/template, published bất biến (`lessonWrite` ở `service.go:710`), copy nội dung khi tạo
  draft (`copyVersionContent` ở `service.go:247`).
- `class_programs` (`000031`) và `classprogram.Apply` (copy tiêu đề buổi vào `class_curricula`).

## Goal
Phiên bản mang đủ dữ liệu cho 7 tab v5: buổi học có `mode`/`unit`/đếm đính kèm, nhóm bài tập theo phiên bản,
nhiều bộ điểm, 4 loại trường nhật ký, lịch sử phiên bản kèm lớp đang gắn; thẻ chương trình mẫu có đếm lớp/buổi/phiên bản.
Kết thúc phase bằng việc cập nhật **web contract** (Zod schema, API client, hooks, MSW) để Phase 3/4/5 chạy song song.

## Migration `000034_template_version_v5`

```sql
-- up
ALTER TABLE template_lessons
    ADD COLUMN mode VARCHAR(12) NOT NULL DEFAULT 'scheduled'
        CHECK (mode IN ('scheduled', 'self_study')),
    ADD COLUMN unit VARCHAR(100);

CREATE TABLE template_exercise_groups (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    version_id UUID NOT NULL,
    center_id  UUID NOT NULL,
    name       VARCHAR(100) NOT NULL,
    position   INT NOT NULL CHECK (position > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (id, center_id),
    UNIQUE (version_id, position) DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT fk_template_exercise_groups_version_center
        FOREIGN KEY (version_id, center_id) REFERENCES program_template_versions (id, center_id) ON DELETE CASCADE
);
CREATE INDEX idx_template_exercise_groups_version ON template_exercise_groups (version_id);

ALTER TABLE template_lesson_exercises
    ADD COLUMN group_id UUID,
    ADD CONSTRAINT fk_template_lesson_exercises_group_center
        FOREIGN KEY (group_id, center_id) REFERENCES template_exercise_groups (id, center_id) ON DELETE SET NULL;

-- Nhật ký: thêm 2 loại v5, giữ 2 loại cũ
ALTER TABLE template_log_fields DROP CONSTRAINT template_log_fields_kind_check;
ALTER TABLE template_log_fields ADD CONSTRAINT template_log_fields_kind_check
    CHECK (kind IN ('text', 'long_text', 'checkbox', 'student', 'number', 'select'));

-- Bộ điểm: mảng phẳng -> mảng bộ. Chỉ bọc khi phần tử đầu không có "components".
UPDATE program_template_versions
SET score_set = jsonb_build_array(jsonb_build_object(
        'key', 'main', 'title', 'Bộ điểm', 'components', score_set))
WHERE jsonb_typeof(score_set) = 'array'
  AND jsonb_array_length(score_set) > 0
  AND NOT (score_set -> 0 ? 'components');
UPDATE program_template_versions SET score_set = '[]'::jsonb
WHERE score_set IS NULL OR jsonb_typeof(score_set) <> 'array';

-- down
UPDATE program_template_versions
SET score_set = COALESCE(score_set -> 0 -> 'components', '[]'::jsonb)
WHERE jsonb_typeof(score_set) = 'array' AND jsonb_array_length(score_set) > 0
  AND (score_set -> 0 ? 'components');
ALTER TABLE template_log_fields DROP CONSTRAINT template_log_fields_kind_check;
UPDATE template_log_fields SET kind = 'text' WHERE kind IN ('long_text', 'student');
ALTER TABLE template_log_fields ADD CONSTRAINT template_log_fields_kind_check
    CHECK (kind IN ('text', 'number', 'select', 'checkbox'));
ALTER TABLE template_lesson_exercises DROP CONSTRAINT fk_template_lesson_exercises_group_center, DROP COLUMN group_id;
DROP TABLE template_exercise_groups;
ALTER TABLE template_lessons DROP COLUMN unit, DROP COLUMN mode;
```

Xác nhận tên constraint CHECK của `kind` bằng `\d template_log_fields` trước khi viết; nếu Postgres đặt tên khác thì
sửa cả up/down. Down mất các bộ điểm ngoài bộ đầu — ghi rõ trong comment đầu file.

## Model / DTO
- `Lesson`: `Mode string`, `Unit *string`. `LessonResponse` + `mode`, `unit`, `material_count`, `exercise_count`
  (subselect trong query `ListLessons`/`GetLesson` ở `repository.go:447`, khuôn `lesson_count` của `versionSelect` ở `repository.go:344`).
- `LessonRequest` + `mode` (`oneof=scheduled self_study`, default `scheduled` khi trống), `unit` (`max=100`).
  `duration_min` giữ validate hiện có; UI bước 15 chỉ là gợi ý phía client.
- `ExerciseGroup{ID, VersionID, CenterID, Name, Position}` + `ExerciseGroupResponse{id, version_id, name, position, exercise_count}`;
  `ExerciseGroupRequest{Name string \`binding:"required,max=100"\`}`.
- `LessonExerciseInput` + `GroupID *uuid.UUID` (phải thuộc cùng `version_id`, nếu không → 422).
  `LessonExerciseResponse` + `group_id`.
- `ScoreSet` JSONB: đổi kiểu thành `[]ScoreSetGroup{Key, Title, Components []ScoreComponent}`;
  `ScoreSetInput = []ScoreSetGroupInput{key, title, components []ScoreComponentInput}`.
  Validate: key nhóm khớp `scoreKey`, unique; tối đa 10 bộ; mỗi bộ ≤ 20 thành phần (giữ `maxScoreComponents`);
  thành phần key unique **trong bộ**. `VersionDetailResponse.score_set` là mảng bộ.
- `LogFieldInput.Kind` binding `oneof=text long_text checkbox student number select`.
- `VersionResponse` + `class_count int`, `classes []VersionClassRef{id, name}` (raw SQL join
  `class_programs p JOIN classes c ON c.id = p.class_id WHERE p.template_version_id = ? AND p.center_id = ?`,
  order by `c.name`; cap 50, `class_count` là count thật).
- `TemplateResponse` + `class_count` (mọi phiên bản của template), `lesson_count` (của released version nếu có, else
  draft), `versions []VersionRef{id, version_no, status}` (order `version_no DESC`).
  Hub tính pill trạng thái client-side: published → "Đang hoạt động", chỉ draft → "Bản nháp", chỉ archived → "Ngừng".
- `DuplicateLessonResponse = LessonResponse`.

## Service
- `DuplicateLesson(ctx, sc, lessonID)`: trong `lessonWrite` — load lesson + materials + exercises, insert bản sao với
  `position = src.position + 1`, dịch các buổi sau lên 1 (tận dụng unique deferred), copy `mode`, `unit`, `objectives`,
  `duration_min`, `homework_note`, đính kèm (kể cả `group_id`, `shared_with_students`); **không copy** prep fields
  (`prep_status`, `assignee_id`, `due_date`, `checklist`) — cùng cách `copyVersionContent` chỉ copy nội dung (`service.go:247`). Tiêu đề: `"{title} (bản sao)"`.
- `ClearLessons(ctx, sc, versionID)`: draft only; xoá mọi buổi của phiên bản (cascade bảng nối). Audit action
  `template_version.clear_lessons`.
- `CreateExerciseGroup` / `DeleteExerciseGroup`: `lessonWrite` (draft only); position = max+1; xoá → FK SET NULL tự gỡ nhóm khỏi bài tập.
- `copyVersionContent`: copy `template_exercise_groups` trước, xây `map[oldGroupID]newGroupID`, rồi copy
  `template_lesson_exercises` với `group_id` ánh xạ; copy `mode`/`unit`.
- `SetScoreSet` đổi chữ ký nhận `[]ScoreSetGroupInput`. `copyVersionContent` copy nguyên JSONB.
- `SetLessonExercises`: validate `group_id` thuộc phiên bản của lesson (một query `SELECT id FROM template_exercise_groups WHERE version_id = ? AND id IN (?)`).
- `classprogram.Apply` **không đổi** (D6). `classprogram` chỉ đọc `LessonResponse` nên field mới không ảnh hưởng.

## Routes
| Method | Path | Perm | `req()` |
|---|---|---|---|
| POST | `/api/v1/library/lessons/:lid/duplicate` | `library.edit` | `req("template_lesson.duplicate", "template_lesson", "lid")` |
| DELETE | `/api/v1/library/versions/:vid/lessons` | `library.edit` | `req("template_version.clear_lessons", "template_version", "vid")` |
| GET | `/api/v1/library/versions/:vid/exercise-groups` | `library.read` | — |
| POST | `/api/v1/library/versions/:vid/exercise-groups` | `library.edit` | `req("template_exercise_group.create", "template_exercise_group", "")` |
| DELETE | `/api/v1/library/versions/:vid/exercise-groups/:gid` | `library.edit` | `req("template_exercise_group.delete", "template_exercise_group", "gid")` |

Cập nhật `route_policy_snapshot_test.go` (+5) và `audit/action_test.go` (+4). Body `PUT …/score-set` đổi hình dạng —
route cũ giữ nguyên, không versioning API (chỉ web nội bộ dùng, đã grep).

## Web contract (kết thúc phase — để 3 phase web không đụng chung file)
- `apps/web/src/features/library/schemas/library-schemas.ts`:
  - `materialKindSchema` = 8 giá trị (`other` chỉ đọc), `materialSchema` + `active`, `lesson_count`, `template_count`;
    `materialFormSchema.kind` 7 giá trị.
  - `exerciseSchema` + `code`, `skill`, `level`, `active`, `lesson_count`, `template_count`; `exerciseFormSchema` + `code?`, `skill?`, `level?`.
  - `lessonModeSchema = z.enum(["scheduled","self_study"])`; `templateLessonSchema` + `mode`, `unit`, `material_count`, `exercise_count`;
    `lessonFormSchema` + `mode`, `unit`.
  - `exerciseGroupSchema`, `lessonExerciseSchema` + `group_id`.
  - `scoreSetGroupSchema{key,title,components}`; `versionDetailSchema.score_set: z.array(scoreSetGroupSchema)`.
  - `logFieldKindSchema` = 6 giá trị.
  - `templateVersionSchema` + `class_count`, `classes`; `programTemplateSchema` + `class_count`, `lesson_count`, `versions`.
- `api/library-api.ts`: `setMaterialStatus`, `setExerciseStatus`, `duplicateLesson`, `clearLessons`,
  `listExerciseGroups`, `createExerciseGroup`, `deleteExerciseGroup`; `listMaterials/listExercises` nhận `active?`.
- `hooks/library-keys.ts` + `hooks/use-library.ts`: `useSetMaterialStatus`, `useSetExerciseStatus`, `useDuplicateLesson`,
  `useClearLessons`, `useExerciseGroups`, `useCreateExerciseGroup`, `useDeleteExerciseGroup`; invalidate
  `lessons(versionId)`, `versionDetail(versionId)`, `template(id)` như các mutation hiện có.
- `lib/library-labels.ts`: `materialKindLabel` 8 loại (video Video, audio Âm thanh, image Hình ảnh, doc Tài liệu,
  note Ghi chú, live Buổi học trực tuyến, link Liên kết ngoài, other Khác), `materialKindIcon` (lucide),
  `materialFormatLabel(url)` (PDF/DOCX/PPTX/MP4/MP3/YouTube/Drive/Link theo đuôi hoặc host), `lessonModeLabel`
  (scheduled "Buổi học có lịch", self_study "Không lịch"), `logFieldKindLabel` 6 loại, `templateStatusLabel(template)`.
- `__tests__/library-handlers.ts`: thêm handler cho 7 route mới, field mới trong fixture, `?active=` filter.

## Tests
- `migrations_test.go`: `TestTemplateVersionV5Migration` — seed version với `score_set` phẳng + log field `text`, up:
  bọc thành 1 bộ `main`; insert group + gán `group_id`, xoá group → `group_id` NULL; down: score_set phẳng trở lại, `long_text` → `text`.
  Cập nhật số bước roll-back trong test cũ (+2 tổng cộng sau 000033/000034).
- Unit `library/service_test.go` (khuôn hiện có): duplicate giữ thứ tự và reset prep; clear lessons trên published → 409;
  group_id khác phiên bản → 422; score set 11 bộ → 422; copy draft ánh xạ group.
- Integration: `POST versions` sau khi published có group → draft mới có group mới với id khác, bài tập trỏ đúng.
- Web: `vitest` cho schema parse fixture mới (MSW handlers tự kiểm qua các test trang ở Phase 3–5).

## Files
- Create: `apps/api/migrations/000034_template_version_v5.up.sql`, `.down.sql`.
- Modify (API): `library/model.go`, `dto.go`, `repository.go`, `service.go`, `handler.go`, `routes.go`,
  `shared/routespec/routespec.go`, `server/route_policy_snapshot_test.go`, `features/audit/action_test.go`,
  `migrations/migrations_test.go`, `library/service_test.go`, integration tests.
- Modify (web contract): `features/library/schemas/library-schemas.ts`, `api/library-api.ts`, `hooks/library-keys.ts`,
  `hooks/use-library.ts`, `lib/library-labels.ts`, `__tests__/library-handlers.ts`.

## Verification
```bash
make migrate-up && make migrate-down && make migrate-up
make test-api-unit && make scopelint
go test ./apps/api/migrations/... -run 'TemplateVersionV5|TemplateLessonPrepSchema|ClassProgramsSchema' -p 1
cd apps/web && npx tsc --noEmit && npx vitest run src/features/library
```

## Risks / rollback
- Reshape `score_set` chạm dữ liệu thật → backup trước; down chỉ giữ bộ đầu.
- Thay đổi chữ ký `SetScoreSet` phá `score-set-editor.tsx` cho tới khi Phase 4 viết lại → Phase 2 phải cập nhật
  editor hiện có ở mức tối thiểu (đọc/ghi bộ đầu) để `tsc` xanh trước khi Phase 4 bắt đầu.

## Success Criteria
- [x] `GET /library/versions/:vid` trả `score_set` dạng mảng bộ, lessons có `mode/unit/material_count/exercise_count`, exercises có `group_id`.
- [x] `GET /library/templates` trả `class_count`, `lesson_count`, `versions[]`; `GET …/versions` trả `classes[]`.
- [x] Duplicate/clear/groups tôn trọng `VERSION_LOCKED`; xoá nhóm không xoá bài tập.
- [x] Draft mới copy nhóm với id mới và ánh xạ `group_id`.
- [x] `tsc`, `vitest` xanh với contract mới; MSW handlers phủ 7 route mới.
