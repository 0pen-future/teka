---
phase: 8
title: "Chuẩn bị tài liệu (bảng chuẩn bị trên bản nháp chương trình)"
status: completed
priority: P2
effort: "1.5d"
dependencies: [3]
---

# Phase 8: Chuẩn bị tài liệu

> Outline (deep mode). **Xác nhận trong scope** (Validation Session 1, Q2). **Scout lại**:
> `tasks/` (headless kanban `apps/web/src/lib/kanban`, `use-board-dnd.ts`), `library/` sau Phase 3/4.

## Goal
Quy trình soạn chương trình mẫu theo nhóm: tạo "dự án chuẩn bị" (= một chương trình mẫu với bản nháp có N buổi trống),
bảng việc theo buổi (todo/doing/review/done), phân công thành viên + hạn, checklist từng buổi, rồi **publish** bản nháp
(Phase 3) khi xong. Không import file (U1) — màn "Nguồn dữ liệu" bị bỏ.

## Key decisions
- **Không có bảng `prep_*` riêng.** <!-- Red Team S1 F11 --> Một "dự án chuẩn bị" **là** `program_template` + phiên bản `draft`;
  mỗi "mục chuẩn bị" **là** một `template_lessons` của bản nháp. Trước đây plan sao chép cột của `template_lessons` sang
  `prep_items` rồi cần bước "generate" để copy ngược — bỏ cả hai. Bước "Tạo chương trình" của prototype trở thành
  **wizard tạo template + draft với `lesson_count` buổi trống**; kết thúc dự án = publish (route Phase 3).
- Thêm 4 cột chuẩn bị vào `template_lessons`: `prep_status` (`todo|doing|review|done`, default `todo`), `assignee_id`,
  `due_date`, `checklist JSONB` (`[{label, done}]`). Khi version publish, các cột này giữ nguyên làm lịch sử.
- **Tái dùng headless kanban** `src/lib/kanban` cho board (move menu + phím `[`/`]`, không dnd-kit ở phase này);
  backend không cần `pkg/kanban` (4 cột cố định, thứ tự = `position` sẵn có).
- Quyền: đọc `library.read`; đổi `prep_status`/checklist/nội dung buổi `library.edit` (optIn); đổi `assignee_id`/`due_date`
  cần `prep.assign` (`optIn`, `implied` ⇒ `library.read`). <!-- Red Team S1 F6 --> Đây là phase thêm key cuối →
  **bump `CatalogVersion` 4→5 một lần ở đây** + `catalog_test.go:316-318` + `handlers.ts:17` (D5). <!-- Red Team S1 F7 -->

## Migration sketch — `000032_template_lesson_prep` <!-- Red Team S1 F5, F8, F11 -->
```sql
ALTER TABLE template_lessons
  ADD COLUMN prep_status VARCHAR(10) NOT NULL DEFAULT 'todo' CHECK (prep_status IN ('todo','doing','review','done')),
  ADD COLUMN assignee_id UUID NULL,
  ADD COLUMN due_date DATE NULL,
  ADD COLUMN checklist JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD CONSTRAINT fk_template_lessons_assignee FOREIGN KEY (assignee_id, center_id) REFERENCES center_members (teacher_id, center_id) ON DELETE SET NULL (assignee_id); -- PK thật (000007:72); chỉ đặt NULL cột assignee_id, không xoá buổi mẫu
CREATE INDEX idx_template_lessons_assignee ON template_lessons (assignee_id) WHERE assignee_id IS NOT NULL;
-- down: DROP INDEX; ALTER TABLE template_lessons DROP CONSTRAINT, DROP 4 cột
```
Không backfill quyền (`prep.assign` optIn). <!-- Updated: Validation Session 1 - khóa center_members xác minh từ 000007 -->

## API (thêm vào `library`)
- `GET /library/versions/:vid/board` → lessons nhóm theo `prep_status` (+ assignee tên, due, checklist tiến độ).
- `PATCH /library/lessons/:lid/prep {prep_status?, checklist?}` (`library.edit`);
  `PATCH /library/lessons/:lid/assignment {assignee_id?, due_date?}` (`prep.assign`; assignee phải là thành viên đang hoạt động).
  Thay cả khối, không phải tri-state: field bị bỏ hoặc gửi `null` đều xoá giá trị đang lưu (không giữ nguyên như tasks'
  cách phân biệt "bỏ qua" vs "xoá"); client phải luôn gửi lại field muốn giữ. `GET /library/assignees` (`prep.assign`,
  không cần `members.list`) trả về thành viên đang hoạt động của trung tâm cho picker phân công.
- `POST /library/templates` nhận thêm `lesson_count` (tuỳ chọn) → tạo draft với N buổi trống `Buổi 1..N` (wizard).
- `GET /library/templates?has_draft=true` để trang "Chuẩn bị tài liệu" liệt kê dự án đang mở.
- Chỉ sửa được khi version `draft` (409 `VERSION_LOCKED` như Phase 3).
- Mọi route mutating khai `req(action, "template_lesson", id)`; `route_policy_snapshot_test.go` (+3), `audit/action_test.go` (+2). <!-- Red Team S1 F7 -->

## Web (feature `library`)
- `pages/prep-page.tsx` (`/prep`): danh sách template có draft (tên, môn, tiến độ done/N, người tham gia) + nút "Tạo chương trình".
- `pages/prep-board-page.tsx` (`/prep/:vid/board`): 4 cột từ `src/lib/kanban`, card = buổi mẫu (tiêu đề, assignee, due, checklist x/y).
- `pages/prep-assign-page.tsx` (`/prep/:vid/assign`): bảng phân công & tiến độ (gate `has("prep.assign")`).
- Chi tiết buổi chuẩn bị = `template-lesson-page.tsx` (Phase 3) mở rộng khối checklist + trạng thái.
- `pages/template-create-wizard-page.tsx` (`/library/templates/new`): 3 bước thông tin → số buổi → xem trước → tạo.
- Nav "Chuẩn bị tài liệu" `/prep` perm `library.read`; `OVERFLOW_LABELS` + `OVERFLOW_PATH_PREFIXES`; `RESOURCE_LABELS`
  thêm `prep`. <!-- Red Team S1 F14 -->

## Verification
- Integration: PATCH prep trên version published → 409; assignee khác trung tâm → 422; member không có `prep.assign` → 403;
  wizard `lesson_count=8` → 8 lessons position 1..8; `CatalogVersion` = 5 và `catalog_test` + MSW mirror khớp.
- Migration 000032 up/down/up sạch.
- Vitest board (dùng kanban lib test helpers), wizard; e2e `prep.spec.ts`.

## Risks
- Phase 8 đã xác nhận trong scope (Validation S1 Q2) → đây là phase bump `CatalogVersion` duy nhất (D5). <!-- Updated: Validation Session 1 - Q2 -->
- Cột chuẩn bị nằm trên bảng nội dung (`template_lessons`) — chấp nhận: đây là metadata của cùng thực thể, tránh bảng song song.

## Completion notes (2026-09-24)
- API (`d0c27ce`): migration `000032` thêm `prep_status`, `assignee_id`, `due_date`, `checklist` vào `template_lessons`;
  FK `(assignee_id, center_id)` → `center_members` là `ON DELETE SET NULL (assignee_id)` (lệch sketch CASCADE có chủ đích:
  CASCADE sẽ xoá nội dung buổi mẫu). Board, PATCH prep, PATCH assignment, `lesson_count` khi tạo template,
  `has_draft`; `prep.assign` optIn ⇒ `library.read`; `CatalogVersion` 4→5 một lần.
- Web (`d578cb9`): `/prep`, `/prep/:vid/board` (headless kanban), `/prep/:vid/assign`, khối chuẩn bị trên trang buổi mẫu,
  wizard `/library/templates/new`; e2e `prep.spec.ts`.
- Sau review (`c1781f3`, `c90e4f9`): `GET /library/assignees` gate `prep.assign` để người không có `members.list` vẫn
  phân công được; PATCH prep chỉ ghi field có trong request; nhãn checklist trim + 422 khi rỗng; panel chuẩn bị key theo
  `lesson.id` và chỉ gửi field đổi; ô hạn commit khi blur; wizard giữ bước 3 cho lỗi không thuộc form thông tin;
  `/prep` cảnh báo khi vượt 100 bản nháp. Hợp đồng: PATCH assignment **thay cả hai field**.
- Gỡ thành viên là soft-leave (`left_at`) nên FK không chạy; người đã rời vẫn là assignee cũ cho đến khi đổi. Chấp nhận,
  phân công mới vẫn bị chặn 422.
- Kiểm chứng: integration `library` + `migrations` (`-p 1`) xanh; `make test-api-unit`, `lint-api`, `test-web`
  (1087 pass), `lint-web` xanh. e2e `prep.spec.ts` chạy lại cùng toàn bộ suite ở Phase 9.
