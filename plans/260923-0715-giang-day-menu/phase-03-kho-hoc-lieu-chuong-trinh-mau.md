---
phase: 3
title: "Kho học liệu — Chương trình mẫu & Buổi học mẫu"
status: pending
priority: P1
effort: "2d"
dependencies: [1]
---

# Phase 3: Kho học liệu — Chương trình mẫu & Buổi học mẫu

> Outline (deep mode). **Scout lại**: `teaching/` (`class_curricula`, `StringList`), `docs/adding-permissions.md`,
> migration backfill 2 bảng `000022_task_board` (step label), `classes/repository.go:124` (`readScoped`),
> `teaching` web feature làm mẫu trang có tab, `permission-schemas.ts:70` (`RESOURCE_LABELS`).

## Context Links
- [plan.md](./plan.md) · D3, D4, D5, D8, D9, D10 · Prototype `Kho học liệu` (tabs), `Chi tiết chương trình mẫu`, `Chi tiết buổi học mẫu`.

## Goal
Trung tâm tạo chương trình mẫu có phiên bản; mỗi phiên bản có danh sách buổi học mẫu (thứ tự, mục tiêu, thời lượng,
ghi chú BTVN). Phiên bản **published** là bất biến để lớp áp dụng ổn định (Phase 7).

## Key decisions
- Ba bảng: `program_templates` → `program_template_versions` (status `draft|published|archived`) → `template_lessons`.
- Chỉ **một draft** mỗi template; "Tạo phiên bản mới" copy toàn bộ lessons của bản published mới nhất sang draft.
- Publish: khóa sửa lessons (service từ chối 409 `VERSION_LOCKED`); archive không xoá.
- Xoá template: soft-delete chỉ khi không có `class_programs` tham chiếu (Phase 7 thêm ràng buộc).
- Quyền: `library.read` = `def()` + **backfill 2 bảng** (mọi vai hiện có đọc được kho); `library.edit` và
  `library.publish` = `optIn()` — owner cấp tay, vì sửa/publish chương trình là quyền quản trị nội dung.
  `implied`: `library.edit` ⇒ `library.read`, `library.publish` ⇒ `library.read`. <!-- Red Team S1 F6 -->
  **Không bump `CatalogVersion` ở phase này** — bump một lần theo D5. <!-- Red Team S1 F7 -->
- Reorder buổi: `UNIQUE (version_id, position) DEFERRABLE INITIALLY DEFERRED` để `PUT .../lessons/order` cập nhật
  trong một tx không cần vị trí tạm; Phase 6 dùng cùng khuôn.

## Migration sketch — `000027_program_templates` <!-- Red Team S1 F5, F8 -->
```sql
program_templates (id PK, center_id NOT NULL, code VARCHAR(20), name, subject, level, description, created_by, created_at, updated_at, deleted_at)
  UNIQUE (center_id, code) WHERE deleted_at IS NULL;  UNIQUE (id, center_id)
program_template_versions (id PK, template_id NOT NULL, center_id NOT NULL, version_no INT, status, changelog TEXT,
  published_at, created_by, created_at, updated_at)
  FOREIGN KEY (template_id, center_id) REFERENCES program_templates (id, center_id)
  UNIQUE (template_id, version_no);  UNIQUE (template_id) WHERE status = 'draft';  UNIQUE (id, center_id)
template_lessons (id PK, version_id NOT NULL, center_id NOT NULL, position INT, title, objectives TEXT, duration_min INT,
  homework_note TEXT, created_at, updated_at)
  FOREIGN KEY (version_id, center_id) REFERENCES program_template_versions (id, center_id)
  UNIQUE (version_id, position) DEFERRABLE INITIALLY DEFERRED;  UNIQUE (id, center_id)
-- backfill quyền library.read theo khuôn 000022, với step label riêng (vd 'library_read_backfill')
-- down: xoá đúng các dòng quyền theo step label, rồi DROP 3 bảng theo thứ tự ngược
```

## API — feature `internal/features/library/`
- `GET/POST /library/templates`, `GET/PUT/DELETE /library/templates/:id`
- `POST /library/templates/:id/versions` (draft mới), `GET /library/templates/:id/versions`
- `POST /library/versions/:vid/publish` (`library.publish`), `POST /library/versions/:vid/archive` (`library.edit`)
- `GET/POST /library/versions/:vid/lessons`, `PUT/DELETE /library/lessons/:lid`, `PUT /library/versions/:vid/lessons/order`
- Kind: đọc `perm(library.read)`, ghi `perm(library.edit|publish)`; **mọi route mutating** khai `req(action, entity, id)`
  với entity `program_template` / `template_version` / `template_lesson`; cập nhật `route_policy_snapshot_test.go` (+12)
  và `audit/action_test.go` (+9). <!-- Red Team S1 F7 -->
- Scope: kho thuộc trung tâm (không theo lớp) → read port center-wide qua helper kiểu `readScoped(ctx, sc)`
  (`classes/repository.go:124`) — scopelint cho phép `CenterWideFor` tại helper đó, không phải theo tên hàm. <!-- Red Team S1 F14 -->
- Pagination `page/per_page/sort` whitelist `name|created_at`.

## Web — feature mới `apps/web/src/features/library/`
- `pages/library-page.tsx` (`/library`): tabs Chương trình mẫu / Buổi học mẫu / Học liệu / Bài tập (2 tab sau: Phase 4).
- `pages/template-detail-page.tsx` (`/library/templates/:id`): header version selector + Publish (gate `has("library.publish")`);
  bảng buổi học với nút lên/xuống (`HvButton`) — không dnd ở đây.
- `pages/template-lesson-page.tsx` (`/library/templates/:id/lessons/:lessonId`): form mục tiêu, thời lượng, BTVN (gate `has("library.edit")`).
- `routes.tsx`, `index.ts`, `api/library-api.ts`, `hooks/`, `schemas/`, `__tests__/`.
- Nav "Kho học liệu" `/library` perm `library.read`; `OVERFLOW_LABELS` + `OVERFLOW_PATH_PREFIXES`; mount trong `app/router.tsx`;
  thêm `library` vào `RESOURCE_LABELS` (`permission-schemas.ts:70`) để trang phân quyền hiển thị nhóm key mới. <!-- Red Team S1 F7, F14 -->
- MSW: mirror key `library.*` trong catalog fixture (không đổi `CATALOG_VERSION` — D5).

## Verification
- `make test-api-unit`, `make test-api` package `library`; test publish lock; test copy lessons sang draft; test reorder giữ unique.
- Quyền: bảng 8 dòng `docs/adding-permissions.md` cho `library.read` (def) và `library.edit` (optIn: member không được cấp → 403).
- Migration 000027 up/down/up sạch; `backfill_parity_test` xanh.
- Vitest quartet 3 trang; e2e `library.spec.ts` (tạo template → thêm 2 buổi → publish).

## Risks
- Trùng khái niệm với `class_curricula.lessons` (chuỗi tên buổi) — Phase 7 đồng bộ; Phase 3 **không** đụng teaching.
- Owner phải cấp `library.edit` tay cho người soạn → Phase 9 seeds cấp sẵn cho tài khoản demo GV.
