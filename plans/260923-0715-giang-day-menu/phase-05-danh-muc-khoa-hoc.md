---
phase: 5
title: "Danh mục khóa học & Chi tiết khóa học"
status: completed
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

## Completion notes (2026-09-24)
- Backend: migration 000029 theo sketch, nhưng FK `classes.course_id` và `courses.default_template_version_id`
  dùng `ON DELETE SET NULL` thay vì CASCADE: xoá khóa hay phiên bản không bao giờ kéo theo lớp hay khóa.
  Xoá khóa còn lớp gắn → 409 `COURSE_IN_USE`; lưu trữ không chạm lớp. `courses.read` là quyền mặc định có
  backfill (`rbac_backfill_rows`), `courses.edit` opt-in; baseline 60 key và `CatalogVersion` không đổi.
  7 route mới trong routespec + snapshot/audit test; swagger sinh lại bằng `make api-docs`.
- Gắn lớp ↔ khóa: `course_id` là con trỏ với quy tắc patch (nil giữ, `""` gỡ, uuid gắn) và không có tag `uuid`
  ở binding vì validator v10 coi con trỏ tới `""` là có giá trị; service tự parse. `default_unit_price` chỉ được
  bỏ trống khi có `course_id` (chép giá khóa). Khóa `archived` không nhận lớp mới, nhưng lớp đã gắn gửi lại đúng
  id vẫn lưu được.
- Ghi đồng thời: xoá khóa / ghi gói học phí khoá dòng `FOR UPDATE` trong tx; gắn lớp tra khóa `FOR SHARE` trong
  cùng tx ghi lớp, nên không lớp sống nào trỏ vào khóa đã xoá (integration chạy race 8 vòng).
- Template mặc định: chỉ kiểm tra "đã phát hành trong trung tâm" khi id đổi; gửi lại id đang lưu không bị chặn
  dù phiên bản đã lưu trữ. Embed `default_template` trả `status` thật; template đã xoá mềm → embed null, id giữ
  nguyên và UI gợi ý chọn lại (không tự ghi đè dữ liệu người dùng).
- Web: `features/courses` (danh sách, chi tiết 4 tab, dialog, editor gói học phí); roster nhận `course` embed và
  lookup khóa qua `listCourseOptions` của chính roster (không import `courses`, tránh vòng import). Helper
  `useApiFormErrors` đưa 409 bất kỳ có `field` về đúng trường; `textareaClassName` chuyển sang `src/lib/forms/`.
  Bộ đếm lớp trên tab Thông tin ghi rõ "toàn trung tâm" (quyết định giữ đếm toàn trung tâm, xem review L4).
  Picker chương trình mẫu cần `library.read`; chip khóa trên header lớp cần `courses.read` mới thành link.
- Kiểm chứng: integration courses + classes + migrations xanh (`-p 1`); `make test-api-unit`/`scopelint`/`lint`
  xanh; vitest 1006 pass (courses 2 trang + dialog lớp + header lớp), typecheck xanh; e2e `courses.spec.ts` 1/1
  trên stack cô lập, dọn dữ liệu qua API trong `afterEach`.
- Review: SHIP WITH FIXES → đã sửa M1–M3, L1–L3, L5, L6 và 2 nit; L4 giữ đếm toàn trung tâm kèm nhãn UI.
  Chi tiết ở [reports/review-phase-05-260924.md](./reports/review-phase-05-260924.md) mục Disposition.
