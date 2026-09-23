---
phase: 5
title: "Danh mục khóa học & Chi tiết khóa học"
status: pending
priority: P1
effort: "1.5d"
dependencies: [1, 3]
---

# Phase 5: Danh mục khóa học & Chi tiết khóa học

> Outline (deep mode). **Scout lại**: `classes` sau Phase 1 (`ListFilter`, `readScoped`), `billing` cho gói học phí (tránh trùng),
> `library` public surface, `permission-schemas.ts:70`.

## Goal
Khóa học là "sản phẩm" của trung tâm: mã, tên, môn, cấp độ, mô tả, chương trình mẫu mặc định, đơn giá mặc định,
gói học phí. Lớp thuộc một khóa; danh sách lớp hiện chip khóa; chi tiết khóa liệt kê lớp đang mở.

## Key decisions
- `courses.default_template_version_id` tham chiếu **phiên bản published** (kiểm tra ở service), FK composite
  `(default_template_version_id, center_id) → program_template_versions (id, center_id)` (Phase 3 đã có UNIQUE). <!-- Red Team S1 F5 -->
- `classes.course_id` nullable, composite FK `(course_id, center_id)` → `courses (id, center_id)`; khi tạo lớp có
  `course_id` mà không truyền `default_unit_price` → copy từ khóa.
- `course_tuition_packs` là bảng thuần dữ liệu (tên, số buổi, giá); **không** nối vào billing (non-goal, ghi rõ).
- Quyền `courses.read` = `def()` + backfill; `courses.edit` = `optIn()` — sửa khóa học kéo theo giá/gói học phí,
  không mặc định cho mọi giáo viên. <!-- Red Team S1 F6 --> Không bump `CatalogVersion` ở đây (D5).
- Số lớp đang mở của khóa được **embed** trong `GET /courses` và `GET /courses/:id` (`classes_running`, `classes_upcoming`
  qua một subquery theo predicate phase của Phase 1) — không tạo endpoint `/courses/:id/stats` riêng. <!-- Red Team S1 F15 -->

## Migration sketch — `000029_courses` <!-- Red Team S1 F5, F8 -->
```sql
courses (id PK, center_id NOT NULL, code VARCHAR(20), name, subject, level, description,
  status CHECK (status IN ('draft','active','archived')) DEFAULT 'draft',
  default_template_version_id UUID NULL, default_unit_price BIGINT NOT NULL DEFAULT 0, total_sessions INT, duration_min INT,
  created_at, updated_at, deleted_at)
  UNIQUE (center_id, code) WHERE deleted_at IS NULL;  UNIQUE (id, center_id)
  FOREIGN KEY (default_template_version_id, center_id) REFERENCES program_template_versions (id, center_id)
course_tuition_packs (id PK, course_id NOT NULL, center_id NOT NULL, name, sessions INT, price BIGINT, position INT,
  FOREIGN KEY (course_id, center_id) REFERENCES courses (id, center_id))
ALTER TABLE classes ADD COLUMN course_id UUID NULL,
  ADD CONSTRAINT fk_classes_course FOREIGN KEY (course_id, center_id) REFERENCES courses (id, center_id);
CREATE INDEX idx_classes_course ON classes (course_id) WHERE deleted_at IS NULL;
-- backfill quyền courses.read (step label riêng)
-- down: xoá quyền theo step label; DROP INDEX; ALTER TABLE classes DROP CONSTRAINT, DROP COLUMN course_id; DROP 2 bảng
```

## API — feature `internal/features/courses/`
- `GET/POST /courses`, `GET/PUT/DELETE /courses/:id`, `POST /courses/:id/archive`
- `PUT /courses/:id/tuition-packs` (ghi đè toàn bộ)
- `classes`: `ListFilter.CourseID`, `ClassResponse.course {id, code, name}` (JOIN), `Create/Update` nhận `course_id`
  (service kiểm khóa cùng trung tâm → 422 nếu không).
- Mọi route mutating khai `req(action, "course", "id")`; `route_policy_snapshot_test.go` (+7), `audit/action_test.go` (+5). <!-- Red Team S1 F7 -->

## Web — feature mới `apps/web/src/features/courses/`
- `pages/courses-page.tsx` (`/courses`): chip trạng thái, bảng MÃ/TÊN/MÔN/CẤP/CT MẪU/LỚP ĐANG MỞ (từ trường embed), dialog tạo (gate `has("courses.edit")`).
- `pages/course-detail-page.tsx` (`/courses/:id`): tabs Thông tin / Chương trình mẫu (chọn version published từ
  `@/features/library`) / Thiết lập (gói học phí) / Vận hành (`useClassesList({course_id})` từ `@/features/roster`).
- `roster`: `class-table.tsx` hiện chip khóa; `ClassDialog` thêm `HvSelect` khóa học (tuỳ chọn); `class-detail-header`
  chip khóa link `/courses/:id`.
- Nav "Danh mục khóa học" `/courses` perm `courses.read`; `OVERFLOW_LABELS` + `OVERFLOW_PATH_PREFIXES`; mount router;
  `RESOURCE_LABELS` thêm `courses`. <!-- Red Team S1 F7, F14 -->

## Verification
- Integration: tạo lớp với `course_id` khác trung tâm → 422; archive khóa không ảnh hưởng lớp; member không được cấp `courses.edit` → 403.
- Migration 000029 up/down/up sạch (kể cả khi `classes` đã có `course_id` dữ liệu → down phải drop constraint trước cột).
- Vitest quartet 2 trang; `class-dialog.test.tsx` cập nhật; e2e `courses.spec.ts`.

## Risks
- Vòng import feature: `courses` → `roster` và `roster` → `courses`? Tránh: roster chỉ nhận `course` embed từ API,
  không import `courses`; `courses` import `useClassesList` qua `@/features/roster/index.ts`.
