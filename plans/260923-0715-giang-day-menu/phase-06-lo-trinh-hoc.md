---
phase: 6
title: "Lộ trình học"
status: pending
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
- Mọi route mutating khai `req(action, "learning_path"|"path_stage", id)`; `route_policy_snapshot_test.go` (+9),
  `audit/action_test.go` (+7). <!-- Red Team S1 F7 -->

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
