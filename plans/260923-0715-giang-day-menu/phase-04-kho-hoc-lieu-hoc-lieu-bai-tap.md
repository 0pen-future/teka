---
phase: 4
title: "Kho học liệu — Học liệu, Bài tập, Trường nhật ký, Bộ điểm"
status: completed
priority: P2
effort: "2d"
dependencies: [3]
---

# Phase 4: Kho học liệu — Học liệu, Bài tập, Trường nhật ký, Bộ điểm

> Outline (deep mode). **Scout lại**: Phase 3 `library/` sau khi merge; `teaching` marks/score để không trùng bộ điểm.

## Goal
Bổ sung vào phiên bản chương trình mẫu: học liệu (link/tên), bài tập, trường nhật ký buổi (log fields) và bộ điểm;
gắn học liệu/bài tập vào từng buổi mẫu với cờ chia sẻ cho học viên.

## Key decisions
- Học liệu là **link + mô tả**, không upload file (không có object storage trong repo — non-goal ghi rõ).
- Gắn buổi↔học liệu/bài tập là bảng nối, ghi đè toàn bộ bằng `PUT` (idempotent, đơn giản cho UI checkbox).
- `log_fields` và `score_set` gắn ở **phiên bản** (cùng lock publish).
- Không key mới: đọc `library.read`, ghi `library.edit` (optIn — kể cả `score-set`). <!-- Red Team S1 F6 -->

## Migration sketch — `000028_library_items` <!-- Red Team S1 F5, F8 -->
```sql
library_materials (id PK, center_id NOT NULL, title, kind CHECK (kind IN ('link','doc','video','other')), url TEXT, description, tags JSONB,
  created_at, updated_at, deleted_at)  UNIQUE (id, center_id)
library_exercises (id PK, center_id NOT NULL, title, description, difficulty SMALLINT, tags JSONB, created_at, updated_at, deleted_at)  UNIQUE (id, center_id)
template_lesson_materials (lesson_id, material_id, center_id NOT NULL, shared_with_students BOOL DEFAULT false, position INT,
  PRIMARY KEY (lesson_id, material_id),
  FOREIGN KEY (lesson_id, center_id)   REFERENCES template_lessons (id, center_id),
  FOREIGN KEY (material_id, center_id) REFERENCES library_materials (id, center_id))
template_lesson_exercises (lesson_id, exercise_id, center_id NOT NULL, position INT, PRIMARY KEY (lesson_id, exercise_id),
  FOREIGN KEY (lesson_id, center_id)   REFERENCES template_lessons (id, center_id),
  FOREIGN KEY (exercise_id, center_id) REFERENCES library_exercises (id, center_id))
template_log_fields (id PK, version_id NOT NULL, center_id NOT NULL, position INT, label, kind CHECK (kind IN ('text','number','select','checkbox')),
  options JSONB, required BOOL, FOREIGN KEY (version_id, center_id) REFERENCES program_template_versions (id, center_id),
  UNIQUE (version_id, position) DEFERRABLE INITIALLY DEFERRED)
ALTER TABLE program_template_versions ADD COLUMN score_set JSONB NOT NULL DEFAULT '[]'; -- [{key,label,max,weight}]
-- down: DROP COLUMN score_set; DROP 5 bảng theo thứ tự ngược
```
Không backfill quyền (không key mới).

## API (thêm vào `library`)
- `GET/POST /library/materials`, `GET/PUT/DELETE /library/materials/:id`; tương tự `/library/exercises`.
- `PUT /library/lessons/:lid/materials` body `[{material_id, shared_with_students}]`; `PUT /library/lessons/:lid/exercises`.
- `PUT /library/versions/:vid/log-fields`, `PUT /library/versions/:vid/score-set` (`library.edit`).
- `GET /library/versions/:vid` trả đầy đủ lessons + materials + exercises (một payload cho Phase 7 read-through).
- Mọi route mutating khai `req(action, entity, id)` (entity `library_material`, `library_exercise`, `template_lesson`,
  `template_version`); `route_policy_snapshot_test.go` (+12), `audit/action_test.go` (+10). <!-- Red Team S1 F7 -->
- Service kiểm `material_id`/`exercise_id` cùng `center_id` với lesson trước khi ghi bảng nối (FK composite là hàng rào thứ hai).

## Web (feature `library`)
- Tab Học liệu / Bài tập trong `library-page.tsx`: bảng + dialog tạo/sửa (gate `has("library.edit")`).
- `template-lesson-page.tsx`: hai khối chọn học liệu/bài tập (`HvSelect` multi hoặc checklist), cờ chia sẻ.
- `template-detail-page.tsx`: tab phụ "Nhật ký & Điểm" (log fields builder, score set editor).

## Verification
- Integration: PUT nối idempotent; material khác trung tâm → 422; xoá material đang gắn → 409 `MATERIAL_IN_USE`.
- Migration 000028 up/down/up sạch.
- Vitest cho dialog + tab; cập nhật e2e `library.spec.ts`.

## Risks
- Score set JSONB vs bảng — chọn JSONB vì chỉ đọc nguyên khối; nếu Phase 7 cần chấm theo key thì vẫn đủ.

## Completion notes (2026-09-24)
- Backend: migration 000028 đúng sketch (2 bảng catalog xoá mềm, 2 bảng nối ghi đè toàn bộ, `template_log_fields`
  với `UNIQUE (version_id, position) DEFERRABLE`, cột `score_set` JSONB); `migrations_test.go` bước MigrateDown 23 → 24.
  15 route mới trong routespec (catalog CRUD, `PUT lessons/:lid/materials|exercises`, `GET versions/:vid`,
  `PUT versions/:vid/log-fields|score-set`) + snapshot/audit tests; mã lỗi 409 `MATERIAL_IN_USE` / `EXERCISE_IN_USE`;
  swagger sinh lại bằng `make api-docs`.
- Mọi ghi gắn/log-fields/score-set đi qua `WithinTx` + `LockVersion` như Phase 3, nên cùng chịu khoá publish
  (`VERSION_LOCKED`); id học liệu/bài tập khác trung tâm → 422 trước khi chạm FK composite.
- Helper dùng chung mới `validation.Elements[T]`: body mảng trả 422 với key `"<i>.<field>"` (gin bỏ chỉ số trong
  `SliceValidationError`); web đọc lại bằng `lib/row-errors.ts` để tô lỗi đúng dòng.
- Quyết định khi thực thi (ngoài sketch): `CreateVersion` sao chép cả gắn học liệu/bài tập, trường nhật ký và bộ điểm
  của phiên bản nguồn (không chỉ buổi học) — bản nháp mới phải là bản sao đầy đủ để sửa tiếp, nếu không người dùng
  phải gắn lại từ đầu. Xoá mềm chương trình mẫu **không** gỡ liên kết học liệu (giữ đúng plan), nhưng link của
  template đã xoá **không còn tính là đang dùng**: xoá học liệu/bài tập chỉ bị 409 khi còn link trong template sống —
  link nháp → "gỡ khỏi các buổi", link trong phiên bản đã phát hành/lưu trữ → thông báo riêng, không gỡ được.
  Sửa nội dung học liệu/bài tập áp dụng cho mọi phiên bản đang tham chiếu (danh mục dùng chung, không snapshot).
- Xoá và gắn tuần tự trên dòng mục (`FOR UPDATE` khi xoá, `FOR SHARE` khi gắn); `url` học liệu chỉ nhận
  `http(s)://` ở cả API lẫn zod; trần 100 mục cho một lần gắn.
- Web: tab Học liệu / Bài tập (`items-tabs.tsx`, dialog tạo/sửa, xác nhận xoá, tìm kiếm debounce), khối chọn học
  liệu/bài tập với cờ chia sẻ trên trang buổi (`lesson-attachments.tsx`, lưu riêng từng khối), tab phụ
  "Nhật ký & Điểm" trên chi tiết chương trình (`log-fields-editor.tsx`, `score-set-editor.tsx`, chỉ đọc khi không
  phải nháp). Ghi catalog invalidate cả chi tiết buổi/phiên bản vì payload nhúng dòng catalog.
- Kiểm chứng: integration library + migrations xanh (`-p 1`), `make test-api-unit`/`scopelint`/`lint` xanh; vitest
  library 54 test (975 toàn bộ), typecheck xanh; e2e `library.spec.ts` 2/2 trên stack cô lập.
- Review: SHIP WITH FIXES → đã sửa M1–M3, L1–L4, L6, L7 và 2 nit; L5 giữ tham chiếu theo quyết định trên.
  Chi tiết và phương án thay thế cho M2 ở [reports/review-phase-04-260923.md](./reports/review-phase-04-260923.md)
  mục Disposition.
