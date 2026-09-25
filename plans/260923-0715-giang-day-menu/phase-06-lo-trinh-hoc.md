---
phase: 6
title: "Lộ trình học"
status: completed
priority: P2
effort: "1d"
dependencies: [5]
---

# Phase 6: Lộ trình học

> Outline (deep mode). **Scout lại**: `courses/` sau Phase 5, khuôn reorder DEFERRABLE của Phase 3.

## Goal
Lộ trình là chuỗi **giai đoạn**, mỗi giai đoạn gồm một hoặc nhiều khóa học; dùng để tư vấn tuyển sinh và
hiển thị "khóa tiếp theo" trong chi tiết khóa học.

## Key decisions
- Ba bảng `learning_paths` → `path_stages` (position) → `path_stage_courses` (position). Một khóa có thể nằm trong
  nhiều lộ trình.
- Quyền `paths.read` = `def()` + backfill; `paths.edit` = `optIn()` (`paths.edit` ⇒ `paths.read`). <!-- Red Team S1 F6 -->
  Không bump `CatalogVersion` ở đây (D5).
- Không gắn học viên vào lộ trình (non-goal — không có trong menu Giảng dạy).

## Migration sketch — `000030_learning_paths` <!-- Red Team S1 F5, F8 -->
```sql
learning_paths (id PK, center_id NOT NULL, code VARCHAR(20), name, description, status CHECK (status IN ('draft','active','archived')),
  created_at, updated_at, deleted_at)  UNIQUE (center_id, code) WHERE deleted_at IS NULL;  UNIQUE (id, center_id)
path_stages (id PK, path_id NOT NULL, center_id NOT NULL, position INT, name, goal TEXT,
  FOREIGN KEY (path_id, center_id) REFERENCES learning_paths (id, center_id),
  UNIQUE (path_id, position) DEFERRABLE INITIALLY DEFERRED;  UNIQUE (id, center_id))
path_stage_courses (stage_id, course_id, center_id NOT NULL, position INT, PRIMARY KEY (stage_id, course_id),
  FOREIGN KEY (stage_id, center_id)  REFERENCES path_stages (id, center_id),
  FOREIGN KEY (course_id, center_id) REFERENCES courses (id, center_id))
-- backfill quyền paths.read (step label riêng)
-- down: xoá quyền theo step label; DROP 3 bảng theo thứ tự ngược
```

## API — feature `internal/features/paths/`
- `GET/POST /paths`, `GET/PUT/DELETE /paths/:id`
- `POST /paths/:id/stages`, `PUT/DELETE /paths/:id/stages/:sid`, `PUT /paths/:id/stages/order`
- `PUT /paths/:id/stages/:sid/courses` (ghi đè `[course_id...]`, kiểm cùng trung tâm)
- `GET /courses/:id/paths` (trong `courses`): lộ trình chứa khóa này (cho tab Thông tin khóa học).
- Mọi route mutating khai `req(action, "learning_path"|"path_stage", id)`; `route_policy_snapshot_test.go` (+11),
  `audit/action_test.go` (+8). <!-- Red Team S1 F7 --> <!-- Updated: Phase 6 close - đếm thực tế 10 route paths + 1 route courses, 8 route ghi -->

## Web (feature `courses`)
- `pages/learning-paths-page.tsx` (`/paths`): danh sách card lộ trình, số giai đoạn, số khóa.
- `pages/path-detail-page.tsx` (`/paths/:id`): timeline giai đoạn dọc như prototype, mỗi giai đoạn chip khóa,
  nút thêm/sắp xếp (gate `has("paths.edit")`); chọn khóa qua `HvSelect` có `searchThreshold`.
- Nav "Lộ trình học" `/paths` perm `paths.read`; `OVERFLOW_LABELS` + `OVERFLOW_PATH_PREFIXES`; `RESOURCE_LABELS` thêm `paths`. <!-- Red Team S1 F7, F14 -->

## Verification
- Integration: reorder stages giữ unique position trong một tx (DEFERRABLE); xoá khóa đang trong lộ trình → 409 `COURSE_IN_PATH`;
  member không được cấp `paths.edit` → 403.
- Migration 000030 up/down/up sạch.
- Vitest quartet 2 trang.

## Risks
- Nếu Phase 3 không dùng DEFERRABLE (scout thấy lý do), quay về chiến lược 2 bước với vị trí tạm âm — dùng chung helper.

## Completion notes (2026-09-24)
- Backend (`4ae6166`): migration 000030 theo sketch với ba bảng composite FK, unique một phần `(center_id, code)`,
  `uq_path_stages_position` DEFERRABLE INITIALLY DEFERRED và backfill hai nhánh `paths.read` theo sổ
  `rbac_backfill_rows`. `paths.read` là `def()`, `paths.edit` là `optIn()` và kéo theo `paths.read`;
  `CatalogVersion` không đổi. 11 route mới (10 `paths` + `GET /courses/:id/paths`), snapshot +11, audit +8.
- Thao tác giai đoạn (thêm, sửa, xoá, sắp xếp, gán khóa) đều khoá lộ trình `FOR UPDATE`; gán khóa khoá khóa học
  `FOR SHARE` còn `courses.Delete` khoá `FOR UPDATE` trong cùng tx với phần đếm, nên xoá khóa đang trong lộ trình
  sống trả 409 `COURSE_IN_PATH` nhất quán. Gán khóa ghi đè cả danh sách, trần 20 khóa mỗi giai đoạn, id trùng 422;
  nhận mọi khóa còn sống (kể cả `archived`) để client gửi lại danh sách đang có khóa lưu trữ vẫn lưu được.
  `sameIDSet` tách thành `shared/idset` dùng chung với `library`.
- Web (`2f6151f`, fix `13f92b3`): `/paths` danh sách card và `/paths/:id` timeline dọc; nút "Sửa lộ trình" và
  "Xoá lộ trình" trên header; picker `HvSelect` chỉ đưa khóa `active`, chip khóa hiện badge với trạng thái khác.
  Chi tiết khóa học có mục "Lộ trình học" đọc `GET /courses/:id/paths` khi có `paths.read`. Sau review: mọi
  mutation lộ trình/khóa học làm mới cache chéo (`by-course`, `pathsKeys.all`), mutation giai đoạn lỗi refetch chi
  tiết, nhãn a11y mang tên giai đoạn, chip chỉ là link khi có `courses.read`, catalog lỗi báo một dòng.
- Kiểm chứng: integration paths + courses + migrations xanh (`-p 1`); `make test-api-unit`/`scopelint`/`lint`
  xanh; vitest courses 50 pass sau fix, typecheck xanh. Không có e2e riêng cho phase này (theo plan, gom vào Phase 9).
- Để lại cho Phase 9 (review L2, L4, L6, L8): lost update khi hai người cùng gán khóa (cần `version` trong contract),
  quyết định sản phẩm về khóa `draft` trong lộ trình, trần số giai đoạn, deep link 403 chung với `/courses`,
  và các test đồng thời/binding HTTP/luồng lỗi web còn thiếu.
